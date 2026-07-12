//go:build http

package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
	toolfs "github.com/hijera/foxxycode-agent/internal/tools/fs"
)

// Sender implements acp.UpdateSender for HTTP (streaming SSE or silent non-stream).
type Sender struct {
	cfg *config.Config

	mu         sync.Mutex
	stream     bool
	w          io.Writer
	flusher    http.Flusher
	chatID     string
	created    int64
	model      string
	sessionDir string
	cwd        string
}

// NewSender creates a bridge. Pass w=nil when stream is false.
func NewSender(cfg *config.Config, w http.ResponseWriter, stream bool, model string) *Sender {
	s := &Sender{
		cfg:     cfg,
		stream:  stream,
		w:       w,
		chatID:  newChatID(),
		created: time.Now().Unix(),
		model:   model,
	}
	if w != nil {
		if f, ok := w.(http.Flusher); ok {
			s.flusher = f
		}
	}
	return s
}

// SetSessionDir sets the persisted session directory for permission persistence across restarts.
func (s *Sender) SetSessionDir(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionDir = strings.TrimSpace(dir)
}

// SetCWD records the session working directory so edit previews can resolve relative paths.
func (s *Sender) SetCWD(cwd string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cwd = strings.TrimSpace(cwd)
}

func wireBridgeSession(bridge *Sender, st *session.State) {
	if bridge != nil && st != nil {
		bridge.SetSessionDir(st.GetPersistedSessionDir())
		bridge.SetCWD(st.GetCWD())
	}
}

// SendSessionUpdate forwards agent chunks to SSE when streaming.
func (s *Sender) SendSessionUpdate(sessionID string, update interface{}) error {
	// file_edit events target native editor clients on a side channel, independent of the
	// OpenAI-shaped composer stream, so they fire even when this bridge is not streaming.
	if u, ok := update.(acp.FileEditUpdate); ok {
		s.broadcastEditApplied(sessionID, u)
		return nil
	}
	if !s.stream || s.w == nil {
		return nil
	}
	switch u := update.(type) {
	case acp.MessageChunkUpdate:
		return s.forwardTextChunk(u)
	case acp.ToolCallUpdate:
		return s.writeNamedEventJSON("tool_call", u)
	case acp.ToolCallStatusUpdate:
		return s.writeNamedEventJSON("tool_call_update", u)
	case acp.PlanUpdate:
		return s.writeNamedEventJSON("plan", u)
	case acp.TokenUsageUpdate:
		return s.writeNamedEventJSON("token_usage", u)
	case acp.MemoryPhaseUpdate:
		return s.writeNamedEventJSON("memory_phase", u)
	case acp.MemoryMessageChunkUpdate:
		return s.writeNamedEventJSON("memory_chunk", u)
	case acp.CompactionUpdate:
		return s.writeNamedEventJSON("compaction", u)
	case acp.AvailableCommandsUpdate:
		return s.writeNamedEventJSON("available_commands", u)
	default:
		return nil
	}
}

// broadcastEditApplied fans a filesystem write out to connected native editor clients.
func (s *Sender) broadcastEditApplied(sessionID string, u acp.FileEditUpdate) {
	ideEvents.broadcast(ideEvent{
		Type:       "edit_applied",
		ToolCallID: u.ToolCallID,
		SessionID:  sessionID,
		Path:       u.Path,
		Before:     u.Before,
		After:      u.After,
	})
}

func (s *Sender) forwardTextChunk(u acp.MessageChunkUpdate) error {
	if u.SessionUpdate != acp.UpdateTypeAgentMessageChunk {
		return nil
	}
	text := ""
	if u.Content.Type == acp.ContentTypeText || u.Content.Type == acp.ContentTypeReasoning {
		text = u.Content.Text
	}
	if text == "" {
		return nil
	}
	choiceDelta := map[string]interface{}{}
	if u.Content.Type == acp.ContentTypeReasoning {
		choiceDelta["reasoning_content"] = text
	} else {
		choiceDelta["content"] = text
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delta := map[string]interface{}{
		"id":      s.chatID,
		"object":  "chat.completion.chunk",
		"created": s.created,
		"model":   s.model,
		"choices": []map[string]interface{}{{
			"index": 0,
			"delta": choiceDelta,
		}},
	}
	line, err := json.Marshal(delta)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(s.w, "data: %s\n\n", line); err != nil {
		return err
	}
	if s.flusher != nil {
		s.flusher.Flush()
	}
	return nil
}

func (s *Sender) writeNamedEventJSON(event string, payload interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	line, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, line); err != nil {
		return err
	}
	if s.flusher != nil {
		s.flusher.Flush()
	}
	return nil
}

// RequestPermission auto-approves when permission_mode is bypass; otherwise emits SSE and waits for POST /foxxycode/sessions/{id}/permission.
func (s *Sender) RequestPermission(ctx context.Context, params acp.PermissionRequestParams) (*acp.PermissionResult, error) {
	if s.cfg != nil && s.cfg.Tools.ResolvedPermMode() == config.PermModeBypass {
		return &acp.PermissionResult{Outcome: "allow", OptionID: "allow"}, nil
	}
	if !s.stream || s.w == nil {
		return &acp.PermissionResult{Outcome: "cancelled", OptionID: "reject"}, nil
	}
	sid := strings.TrimSpace(params.SessionID)
	tcid := strings.TrimSpace(params.ToolCall.ToolCallID)
	if sid == "" || tcid == "" {
		return nil, fmt.Errorf("sessionId and toolCall.toolCallId are required")
	}
	s.mu.Lock()
	sd := s.sessionDir
	s.mu.Unlock()
	toolName := ""
	argsJSON := ""
	if len(params.ToolCall.Content) > 0 {
		argsJSON = strings.TrimSpace(params.ToolCall.Content[0].Content.Text)
	}
	if t := strings.TrimSpace(params.ToolCall.Title); t != "" {
		if after, ok := strings.CutPrefix(t, "Run:"); ok {
			toolName = strings.TrimSpace(after)
		} else if after, ok := strings.CutPrefix(t, "run:"); ok {
			toolName = strings.TrimSpace(after)
		}
	}
	if sd != "" {
		_ = session.WritePendingPermission(sd, params, toolName, argsJSON)
	}
	s.broadcastEditProposed(sid, tcid, toolName, argsJSON)
	ch := registerPermissionWait(sid, tcid, sd)
	defer unregisterPermissionWait(sid, tcid, sd)
	if err := s.writeNamedEventJSON("permission", params); err != nil {
		return nil, err
	}
	select {
	case res := <-ch:
		if res == nil {
			return &acp.PermissionResult{Outcome: "cancelled", OptionID: "reject"}, nil
		}
		if sd != "" {
			_ = session.ClearPendingPermission(sd)
		}
		return res, nil
	case <-ctx.Done():
		return &acp.PermissionResult{Outcome: "cancelled", OptionID: "reject"}, nil
	}
}

// broadcastEditProposed computes the diff a pending filesystem write would produce and
// pushes it to native editor clients so they can render an inline Accept/Reject preview.
// No-op for non-write tools or when no IDE client is connected.
func (s *Sender) broadcastEditProposed(sessionID, toolCallID, toolName, argsJSON string) {
	if !ideEvents.hasSubscribers() {
		return
	}
	s.mu.Lock()
	cwd := s.cwd
	s.mu.Unlock()
	absPath, before, after, ok, err := toolfs.EditPreview(toolName, argsJSON, cwd)
	if !ok || err != nil {
		return
	}
	ideEvents.broadcast(ideEvent{
		Type:       "edit_proposed",
		ToolCallID: toolCallID,
		SessionID:  sessionID,
		Path:       absPath,
		Before:     string(before),
		After:      string(after),
	})
}

// RequestQuestion emits a composer SSE question event and waits for POST /foxxycode/sessions/{id}/question.
func (s *Sender) RequestQuestion(ctx context.Context, params acp.QuestionRequestParams) (*acp.QuestionResult, error) {
	if !s.stream || s.w == nil {
		return nil, fmt.Errorf("question tool requires streaming responses")
	}
	sid := strings.TrimSpace(params.SessionID)
	rid := strings.TrimSpace(params.RequestID)
	if sid == "" || rid == "" {
		return nil, fmt.Errorf("sessionId and requestId are required")
	}
	ch := registerQuestionWait(sid, rid)
	defer unregisterQuestionWait(sid, rid)
	if err := s.writeNamedEventJSON("question", params); err != nil {
		return nil, err
	}
	select {
	case res := <-ch:
		if res == nil {
			return &acp.QuestionResult{}, nil
		}
		return res, nil
	case <-ctx.Done():
		return &acp.QuestionResult{}, ctx.Err()
	}
}

// WriteFoxxyCodeMetaSSE emits a named event with FoxxyCode response metadata (effective model). No-op when not streaming.
func (s *Sender) WriteFoxxyCodeMetaSSE(metadata map[string]string) error {
	if !s.stream || s.w == nil || len(metadata) == 0 {
		return nil
	}
	payload := map[string]interface{}{"metadata": metadata}
	return s.writeNamedEventJSON("foxxycode_meta", payload)
}

// FinishStream writes foxxycode_meta (when metadata non-nil), then [DONE] for SSE.
func (s *Sender) FinishStreamWithMetadata(meta map[string]string) error {
	if s.stream && s.w != nil && len(meta) > 0 {
		_ = s.WriteFoxxyCodeMetaSSE(meta)
	}
	return s.FinishStream()
}

// FinishStream writes [DONE] for SSE.
func (s *Sender) FinishStream() error {
	if !s.stream || s.w == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := io.WriteString(s.w, "data: [DONE]\n\n")
	if s.flusher != nil {
		s.flusher.Flush()
	}
	return err
}

// ChatID returns the OpenAI-style completion id for this request.
func (s *Sender) ChatID() string { return s.chatID }

// SetModel updates the model name in subsequent chunks.
func (s *Sender) SetModel(m string) { s.model = m }

func newChatID() string {
	return fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
}
