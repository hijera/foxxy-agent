//go:build http

package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/bgtask"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/prompts"
	"github.com/hijera/foxxycode-agent/internal/session"
	"github.com/hijera/foxxycode-agent/internal/tools/todo"
)

func describeFallbackTitle(words []string) string {
	if len(words) == 0 {
		return ""
	}
	n := min(8, len(words))
	return strings.Join(words[:n], " ")
}

func describeClampWords(s string, maxWords int) string {
	w := strings.Fields(s)
	if len(w) <= maxWords {
		return strings.Join(w, " ")
	}
	return strings.Join(w[:maxWords], " ")
}

func describeStripLineNoise(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "- ")
	s = strings.TrimPrefix(s, "* ")
	s = strings.Trim(s, `"'“”„`)
	for strings.HasPrefix(s, "**") {
		s = strings.TrimPrefix(s, "**")
		if i := strings.Index(s, "**"); i >= 0 {
			s = strings.TrimSpace(s[:i] + s[i+2:])
		} else {
			break
		}
	}
	return strings.TrimSpace(s)
}

// describePickPhraseFromLLM picks a usable title from model output. Some models emit a junk first line (e.g. "Po") then the real phrase.
func describePickPhraseFromLLM(llmRaw string, userWords []string) string {
	trimmed := strings.TrimSpace(llmRaw)
	if trimmed == "" {
		return describeFallbackTitle(userWords)
	}
	type scored struct {
		text  string
		words int
		chars int
	}
	var cands []scored
	for _, line := range strings.Split(trimmed, "\n") {
		part := describeStripLineNoise(line)
		if part == "" {
			continue
		}
		fw := strings.Fields(part)
		if len(fw) == 0 {
			continue
		}
		joined := strings.Join(fw, " ")
		cands = append(cands, scored{
			text:  joined,
			words: len(fw),
			chars: utf8.RuneCountInString(joined),
		})
	}
	bestText := ""
	bestScore := 0
	substantial := func(c scored) bool {
		if c.words >= 3 {
			return true
		}
		return c.words >= 2 && c.chars >= 12
	}
	for _, c := range cands {
		if !substantial(c) {
			continue
		}
		score := c.words*120 + min(c.chars, 140)
		if score > bestScore {
			bestScore = score
			bestText = c.text
		}
	}
	if bestText == "" && len(cands) > 0 {
		longest := ""
		for _, c := range cands {
			if c.chars > utf8.RuneCountInString(longest) {
				longest = c.text
			}
		}
		if utf8.RuneCountInString(longest) >= 8 {
			bestText = longest
		}
	}
	if bestText == "" || utf8.RuneCountInString(bestText) < 4 {
		return describeFallbackTitle(userWords)
	}
	return describeClampWords(bestText, 12)
}

func (s *Server) registerFoxxyCodeRoutes() {
	s.mux.HandleFunc("GET /foxxycode/workspace/files", s.foxxycodeWorkspaceFilesGet)
	s.mux.HandleFunc("GET /foxxycode/workspace/context", s.foxxycodeWorkspaceContextGet)
	s.mux.HandleFunc("GET /foxxycode/workspace/folders", s.foxxycodeWorkspaceFoldersGet)
	s.mux.HandleFunc("POST /foxxycode/workspace/folders", s.foxxycodeWorkspaceFoldersPost)
	s.mux.HandleFunc("GET /foxxycode/workspace/file", s.foxxycodeWorkspaceFileGet)
	s.mux.HandleFunc("POST /foxxycode/workspace/relativize", s.foxxycodeWorkspaceRelativizePost)
	s.mux.HandleFunc("GET /foxxycode/slash-commands", s.foxxycodeSlashCommandsGet)
	s.mux.HandleFunc("GET /foxxycode/commands", s.foxxycodeCommandsGet)
	s.mux.HandleFunc("GET /foxxycode/sessions", s.foxxycodeSessionsList)
	s.mux.HandleFunc("POST /foxxycode/sessions/bulk-delete", s.foxxycodeSessionsBulkDelete)
	s.mux.HandleFunc("POST /foxxycode/describe", s.foxxycodeDescribePost)
	s.mux.HandleFunc("POST /foxxycode/enhance-prompt", s.foxxycodeEnhancePromptPost)
	s.mux.HandleFunc("POST /foxxycode/completion", s.foxxycodeCompletionPost)
	s.mux.HandleFunc("GET /foxxycode/completion/config", s.foxxycodeCompletionConfigGet)
	s.mux.HandleFunc("GET /foxxycode/completion/stats", s.foxxycodeCompletionStatsGet)
	s.mux.HandleFunc("POST /foxxycode/completion/feedback", s.foxxycodeCompletionFeedbackPost)
	s.mux.HandleFunc("GET /foxxycode/sessions/{id}/activity", s.foxxycodeSessionActivityGet)
	s.mux.HandleFunc("GET /foxxycode/sessions/{id}/messages", s.foxxycodeSessionMessagesGet)
	s.mux.HandleFunc("GET /foxxycode/sessions/{id}/export", s.foxxycodeSessionExportGet)
	s.mux.HandleFunc("POST /foxxycode/sessions/{id}/export/file", s.foxxycodeSessionExportFilePost)
	s.mux.HandleFunc("GET /foxxycode/sessions/{id}/assets/{name}/thumbnail", s.foxxycodeSessionAssetThumbnailGet)
	s.mux.HandleFunc("GET /foxxycode/sessions/{id}/composer-stream", s.foxxycodeSessionComposerStream)
	s.mux.HandleFunc("GET /foxxycode/events", s.foxxycodeEventsStream)
	s.mux.HandleFunc("GET /foxxycode/sessions/{id}/tool-calls", s.foxxycodeToolCallsList)
	s.mux.HandleFunc("GET /foxxycode/sessions/{id}/tool-calls/{toolCallId}", s.foxxycodeToolCallGet)
	s.mux.HandleFunc("GET /foxxycode/sessions/{id}/assets/{name}", s.foxxycodeSessionAssetGet)
	s.mux.HandleFunc("GET /foxxycode/sessions/{id}/stats", s.foxxycodeSessionStatsGet)
	s.mux.HandleFunc("GET /foxxycode/sessions/{id}/debug", s.foxxycodeSessionDebugGet)
	s.mux.HandleFunc("POST /foxxycode/stream-tickets", s.foxxycodeStreamTicketPost)
	s.mux.HandleFunc("PATCH /foxxycode/sessions/{id}", s.foxxycodeSessionPatch)
	s.mux.HandleFunc("POST /foxxycode/sessions/{id}/workspace", s.foxxycodeSessionWorkspacePost)
	s.mux.HandleFunc("POST /foxxycode/sessions/{id}/cancel", s.foxxycodeSessionCancelGeneration)
	s.mux.HandleFunc("POST /foxxycode/sessions/{id}/compact", s.foxxycodeSessionCompactPost)
	s.mux.HandleFunc("POST /foxxycode/sessions/{id}/question", s.foxxycodeSessionQuestionPost)
	s.mux.HandleFunc("POST /foxxycode/sessions/{id}/permission", s.foxxycodeSessionPermissionPost)
	s.mux.HandleFunc("GET /foxxycode/ide/events", s.foxxycodeIdeEvents)
	s.mux.HandleFunc("POST /foxxycode/ide/editor-state", s.foxxycodeIdeEditorState)
	s.mux.HandleFunc("POST /foxxycode/ide/terminal-state", s.foxxycodeIdeTerminalStatePost)
	s.mux.HandleFunc("GET /foxxycode/ide/terminal-state", s.foxxycodeIdeTerminalStateGet)
	s.mux.HandleFunc("POST /foxxycode/ide/copy-buffer", s.foxxycodeIdeCopyBufferPost)
	s.mux.HandleFunc("POST /foxxycode/ide/paste-classify", s.foxxycodeIdePasteClassifyPost)
	s.mux.HandleFunc("DELETE /foxxycode/sessions/{id}", s.foxxycodeSessionDelete)
	s.mux.HandleFunc("GET /foxxycode/sessions/{id}/plan", s.foxxycodePlanGet)
	s.mux.HandleFunc("PUT /foxxycode/sessions/{id}/plan", s.foxxycodePlanPut)
	s.mux.HandleFunc("POST /foxxycode/sessions/{id}/plan/archive", s.foxxycodePlanArchivePost)
	s.registerDesignPlanRoutes()
	s.registerMemoryRoutes()
	s.registerBackgroundRoutes()
	s.registerQueueRoutes()
	s.registerSubagentRoutes()
	s.registerHookRoutes()
	s.registerSchedulerRoutes()
	s.registerBranchRoutes()
	s.registerSkillsManagementRoutes()
	s.registerMCPManagementRoutes()
}

func (s *Server) foxxycodeSessionCancelGeneration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if err := session.ValidateFolderSessionID(id); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusBadRequest)
		return
	}
	hdr := strings.TrimSpace(r.Header.Get("X-FoxxyCode-Session-ID"))
	if hdr != "" && hdr != id {
		http.Error(w, `{"error":{"message":"X-FoxxyCode-Session-ID does not match path id"}}`, http.StatusBadRequest)
		return
	}
	_ = s.mgr.WriteCrossProcessCancelRequest(id)
	if s.mgr.SessionByID(id) == nil {
		fs := s.mgr.FileStore()
		if fs == nil || !fs.HasPersistedSnapshot(id) {
			http.Error(w, `{"error":{"message":"session not found"}}`, http.StatusNotFound)
			return
		}
		if _, err := s.mgr.HandleSessionLoad(r.Context(), acp.SessionLoadParams{
			SessionID: id,
			CWD:       s.sessionDefaultCWD(),
		}); err != nil {
			http.Error(w, `{"error":{"message":"session not found"}}`, http.StatusNotFound)
			return
		}
		if s.mgr.SessionByID(id) == nil {
			http.Error(w, `{"error":{"message":"session not found"}}`, http.StatusNotFound)
			return
		}
	}
	s.mgr.HandleSessionCancel(acp.SessionCancelParams{SessionID: id})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"object": "foxxycode.session_cancelled", "id": id})
}

func (s *Server) foxxycodeSessionPermissionPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if err := session.ValidateFolderSessionID(id); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusBadRequest)
		return
	}
	hdr := strings.TrimSpace(r.Header.Get("X-FoxxyCode-Session-ID"))
	if hdr != "" && hdr != id {
		http.Error(w, `{"error":{"message":"X-FoxxyCode-Session-ID does not match path id"}}`, http.StatusBadRequest)
		return
	}
	var body struct {
		ToolCallID string `json:"toolCallId"`
		OptionID   string `json:"optionId"`
		Outcome    string `json:"outcome"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":{"message":"invalid JSON"}}`, http.StatusBadRequest)
		return
	}
	tcid := strings.TrimSpace(body.ToolCallID)
	if tcid == "" {
		http.Error(w, `{"error":{"message":"toolCallId is required"}}`, http.StatusBadRequest)
		return
	}
	opt := strings.TrimSpace(body.OptionID)
	out := strings.TrimSpace(body.Outcome)
	if opt == "" && out == "" {
		http.Error(w, `{"error":{"message":"optionId or outcome is required"}}`, http.StatusBadRequest)
		return
	}
	if out == "" {
		switch opt {
		case "reject":
			out = "cancelled"
		default:
			out = "allow"
		}
	}
	if opt == "" {
		if out == "cancelled" {
			opt = "reject"
		} else {
			opt = "allow"
		}
	}
	res := &acp.PermissionResult{
		Outcome:  out,
		OptionID: opt,
	}
	ok := CompletePermissionAnswer(id, tcid, res)
	if !ok {
		// A child session never owns a prompt of its own (its requests are
		// relayed to the parent chat), and a resume would build an agent on
		// it; a read-only transcript answers 409 instead of 404.
		if rejectSubagentTurn(w, s.persistedSessionState(r.Context(), id)) {
			return
		}
		if s.tryResumePendingPermission(r.Context(), id, tcid, res) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, `{"error":{"message":"no pending permission for this toolCallId"}}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) foxxycodeSessionQuestionPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if err := session.ValidateFolderSessionID(id); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusBadRequest)
		return
	}
	hdr := strings.TrimSpace(r.Header.Get("X-FoxxyCode-Session-ID"))
	if hdr != "" && hdr != id {
		http.Error(w, `{"error":{"message":"X-FoxxyCode-Session-ID does not match path id"}}`, http.StatusBadRequest)
		return
	}
	var body struct {
		RequestID string     `json:"requestId"`
		Answers   [][]string `json:"answers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":{"message":"invalid JSON"}}`, http.StatusBadRequest)
		return
	}
	rid := strings.TrimSpace(body.RequestID)
	if rid == "" {
		http.Error(w, `{"error":{"message":"requestId is required"}}`, http.StatusBadRequest)
		return
	}
	if body.Answers == nil {
		http.Error(w, `{"error":{"message":"answers is required"}}`, http.StatusBadRequest)
		return
	}
	ok := CompleteQuestionAnswer(id, rid, &acp.QuestionResult{Answers: body.Answers})
	if !ok {
		http.Error(w, `{"error":{"message":"no pending question for this requestId"}}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) foxxycodeDescribePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}

	var body struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":{"message":"invalid JSON"}}`, http.StatusBadRequest)
		return
	}

	raw := strings.TrimSpace(body.Text)
	if raw == "" {
		http.Error(w, `{"error":{"message":"text is required"}}`, http.StatusBadRequest)
		return
	}

	words := strings.Fields(raw)
	if len(words) <= 3 {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"object": "foxxycode.describe",
			"short":  strings.Join(words, " "),
		})
		return
	}

	provider, err := s.providerFactory(s.activeCfg())
	if err != nil {
		s.log.Error("describe provider", "error", err)
		http.Error(w, `{"error":{"message":"LLM unavailable"}}`, http.StatusServiceUnavailable)
		return
	}

	ctx := r.Context()
	resp, err := provider.Complete(ctx, []llm.Message{
		{
			Role: llm.RoleSystem,
			Content: prompts.WithIdentity("You generate short descriptions for chat titles and command labels. " +
				"Return exactly one short phrase (3 to 8 words) describing what the user's text is about. " +
				"Match the user's language when possible. " +
				"No quotes, no preamble, no headings, no line breaks, no numbering. Output only the phrase."),
		},
		{Role: llm.RoleUser, Content: raw},
	}, nil)
	if err != nil {
		s.log.Error("describe llm", "error", err)
		http.Error(w, `{"error":{"message":"LLM error"}}`, http.StatusBadGateway)
		return
	}

	short := describePickPhraseFromLLM(resp.Content, words)
	if short == "" {
		short = strings.Join(words[:min(3, len(words))], " ")
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object": "foxxycode.describe",
		"short":  short,
	})
}

type foxxycodeToolCallRow struct {
	ToolCallID             string          `json:"toolCallId"`
	Name                   string          `json:"name,omitempty"`
	Kind                   string          `json:"kind,omitempty"`
	Status                 string          `json:"status,omitempty"`
	StartedAt              string          `json:"startedAt,omitempty"`
	FinishedAt             string          `json:"finishedAt,omitempty"`
	ArgsPreview            string          `json:"argsPreview,omitempty"`
	ResultPreview          string          `json:"resultPreview,omitempty"`
	ResultPreviewTruncated bool            `json:"resultPreviewTruncated,omitempty"`
	ResultTotalLines       int             `json:"resultTotalLines,omitempty"`
	PlanSnapshot           []acp.PlanEntry `json:"planSnapshot,omitempty"`
}

func previewText(s string, max int) string {
	txt := strings.TrimSpace(s)
	if txt == "" {
		return ""
	}
	if max <= 0 || len(txt) <= max {
		return txt
	}
	return txt[:max] + "..."
}

func toolKind(name string) string {
	n := strings.TrimSpace(strings.ToLower(name))
	if n == "" {
		return "tool"
	}
	if strings.HasPrefix(n, "foxxycode_todo_") {
		return "todo"
	}
	switch n {
	case "run_command":
		return "shell"
	case "write", "edit", "apply_patch", "mkdir", "touch", "mv":
		return "fs"
	}
	return "tool"
}

func foxxycodeApplyResultPreview(row *foxxycodeToolCallRow, full string) {
	snip, trunc, tl := session.PreviewToolResultSnippet(strings.TrimSpace(row.Name), full)
	row.ResultPreview = snip
	row.ResultPreviewTruncated = trunc
	row.ResultTotalLines = tl
}

// foxxycodeLoadToolCallBundle resolves meta, args, full tool output from disk and in-memory transcript.
func (s *Server) foxxycodeLoadToolCallBundle(st *session.State, sd, toolCallID string) (meta *session.ToolCallMeta, primaryName string, args string, fullResult string) {
	toolCallID = strings.TrimSpace(toolCallID)
	if sd != "" {
		if m, err := session.ReadToolCallMeta(sd, toolCallID); err == nil {
			meta = m
			if m != nil && strings.TrimSpace(m.Name) != "" {
				primaryName = strings.TrimSpace(m.Name)
			}
		}
		if a, err := session.ReadToolCallArgs(sd, toolCallID); err == nil {
			args = a
		}
		if res, err := session.ReadToolCallResult(sd, toolCallID); err == nil {
			fullResult = res
		}
	}
	if meta == nil || (args == "" && fullResult == "") {
		for _, m := range st.GetMessages() {
			if m.Role == llm.RoleAssistant {
				for _, tc := range m.ToolCalls {
					if tc.ID != toolCallID {
						continue
					}
					if primaryName == "" && strings.TrimSpace(tc.Name) != "" {
						primaryName = strings.TrimSpace(tc.Name)
					}
					if meta == nil {
						tmp := session.ToolCallMeta{
							ToolCallID: toolCallID,
							Name:       tc.Name,
							Kind:       toolKind(tc.Name),
							Status:     "pending",
						}
						meta = &tmp
					}
					if args == "" {
						args = tc.InputJSON
					}
				}
			}
			if m.Role == llm.RoleTool && m.ToolCallID == toolCallID {
				if fullResult == "" {
					fullResult = m.Content
				}
				if meta != nil && meta.Status == "pending" {
					meta.Status = "completed"
				}
			}
		}
	}
	if meta != nil && primaryName == "" && strings.TrimSpace(meta.Name) != "" {
		primaryName = strings.TrimSpace(meta.Name)
	}
	return meta, primaryName, args, fullResult
}

func (s *Server) foxxycodeToolCallsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	st := s.foxxycodeEnsureLoaded(w, r, id)
	if st == nil {
		return
	}
	sd := strings.TrimSpace(st.GetPersistedSessionDir())
	msgs := st.GetMessages()

	type ent struct {
		row foxxycodeToolCallRow
	}
	ordered := make([]ent, 0)
	idx := map[string]int{}

	for _, m := range msgs {
		if m.Role == llm.RoleAssistant && len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				if strings.TrimSpace(tc.ID) == "" {
					continue
				}
				if _, ok := idx[tc.ID]; ok {
					continue
				}
				idx[tc.ID] = len(ordered)
				ordered = append(ordered, ent{
					row: foxxycodeToolCallRow{
						ToolCallID:  tc.ID,
						Name:        tc.Name,
						Kind:        toolKind(tc.Name),
						Status:      "pending",
						ArgsPreview: previewText(tc.InputJSON, 200),
					},
				})
			}
		}
		if m.Role == llm.RoleTool && strings.TrimSpace(m.ToolCallID) != "" {
			i, ok := idx[m.ToolCallID]
			if !ok {
				idx[m.ToolCallID] = len(ordered)
				ordered = append(ordered, ent{row: foxxycodeToolCallRow{ToolCallID: m.ToolCallID}})
				i = idx[m.ToolCallID]
			}
			ordered[i].row.Status = "completed"
			foxxycodeApplyResultPreview(&ordered[i].row, m.Content)
		}
	}

	if sd != "" {
		for i := range ordered {
			id := ordered[i].row.ToolCallID
			if meta, err := session.ReadToolCallMeta(sd, id); err == nil && meta != nil {
				if strings.TrimSpace(meta.Name) != "" {
					ordered[i].row.Name = meta.Name
				}
				if strings.TrimSpace(meta.Kind) != "" {
					ordered[i].row.Kind = meta.Kind
				}
				if strings.TrimSpace(meta.Status) != "" {
					ordered[i].row.Status = meta.Status
				}
				ordered[i].row.StartedAt = meta.StartedAt
				ordered[i].row.FinishedAt = meta.FinishedAt
				ordered[i].row.PlanSnapshot = append([]acp.PlanEntry(nil), meta.PlanSnapshot...)
			}
			if args, err := session.ReadToolCallArgs(sd, id); err == nil {
				ordered[i].row.ArgsPreview = previewText(args, 200)
			}
			if res, err := session.ReadToolCallResult(sd, id); err == nil {
				foxxycodeApplyResultPreview(&ordered[i].row, res)
			}
		}
	}

	outRows := make([]foxxycodeToolCallRow, 0, len(ordered))
	for _, e := range ordered {
		outRows = append(outRows, e.row)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object":    "foxxycode.tool_calls",
		"sessionId": id,
		"toolCalls": outRows,
	})
}

func (s *Server) foxxycodeToolCallGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	toolCallID := strings.TrimSpace(r.PathValue("toolCallId"))
	st := s.foxxycodeEnsureLoaded(w, r, id)
	if st == nil {
		return
	}
	sd := strings.TrimSpace(st.GetPersistedSessionDir())

	meta, _, args, full := s.foxxycodeLoadToolCallBundle(st, sd, toolCallID)

	payload := map[string]interface{}{
		"object":     "foxxycode.tool_call",
		"sessionId":  id,
		"toolCallId": toolCallID,
		"meta":       meta,
		"args":       args,
		"result":     full,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) foxxycodeSessionStatsGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	st := s.foxxycodeEnsureLoaded(w, r, id)
	if st == nil {
		return
	}
	sd := strings.TrimSpace(st.GetPersistedSessionDir())
	if sd == "" {
		http.Error(w, `{"error":{"message":"stats unavailable"}}`, http.StatusServiceUnavailable)
		return
	}
	stats, err := session.ReadSessionStats(sd)
	if err != nil {
		if os.IsNotExist(err) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"object":    "foxxycode.session_stats",
				"sessionId": id,
				"stats":     nil,
			})
			return
		}
		http.Error(w, `{"error":{"message":"read failed"}}`, http.StatusInternalServerError)
		return
	}
	if live := st.GetLastContextBreakdown(); live != nil {
		cp := *live
		stats.ContextBreakdown = &cp
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object":    "foxxycode.session_stats",
		"sessionId": id,
		"stats":     stats,
	})
}

// foxxycodeSessionDebugGet returns the persisted debug-trace events for a session
// (one record per turn/LLM/tool boundary), collected only while debug.enabled is on.
// The raw LLM HTTP bodies are written to the process log; this endpoint surfaces the
// lightweight structured timeline. A missing trace is reported as events: null.
func (s *Server) foxxycodeSessionDebugGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	st := s.foxxycodeEnsureLoaded(w, r, id)
	if st == nil {
		return
	}
	sd := strings.TrimSpace(st.GetPersistedSessionDir())
	if sd == "" {
		http.Error(w, `{"error":{"message":"debug trace unavailable"}}`, http.StatusServiceUnavailable)
		return
	}
	events, err := session.ReadDebugTrace(sd)
	if err != nil {
		if os.IsNotExist(err) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"object":    "foxxycode.session_debug",
				"sessionId": id,
				"events":    nil,
			})
			return
		}
		http.Error(w, `{"error":{"message":"read failed"}}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object":    "foxxycode.session_debug",
		"sessionId": id,
		"events":    events,
	})
}

func (s *Server) foxxycodeRequireStore(w http.ResponseWriter) *session.FileStore {
	fs := s.mgr.FileStore()
	if fs == nil || fs.Root == "" {
		http.Error(w, `{"error":{"message":"session store unavailable"}}`, http.StatusServiceUnavailable)
		return nil
	}
	return fs
}

func foxxycodeMustSession(w http.ResponseWriter, s *session.Manager, id string, loadFromDisk func() (*session.State, error)) *session.State {
	if err := session.ValidateFolderSessionID(id); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusBadRequest)
		return nil
	}
	if st := s.SessionByID(id); st != nil {
		return st
	}
	st, err := loadFromDisk()
	if err != nil {
		http.Error(w, `{"error":{"message":"session not found"}}`, http.StatusNotFound)
		return nil
	}
	return st
}

// foxxycodeEnsureLoaded resolves the session a per-session route was asked about, loading it
// from disk when it is not live. Goes through LoadPersistedSession rather than
// HandleSessionLoad: a panel opens ten of these routes at once, and only one of them may
// actually read the bundle (see internal/session/manager_load_flight.go).
func (s *Server) foxxycodeEnsureLoaded(w http.ResponseWriter, r *http.Request, id string) *session.State {
	fs := s.foxxycodeRequireStore(w)
	if fs == nil {
		return nil
	}
	load := func() (*session.State, error) {
		return s.mgr.LoadPersistedSession(r.Context(), id, s.sessionDefaultCWD())
	}
	return foxxycodeMustSession(w, s.mgr, id, load)
}

func parseLimitCursor(q url.Values) (limit, offset int) {
	limit = 50
	if v := strings.TrimSpace(q.Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 100 {
		limit = 100
	}
	if v := strings.TrimSpace(q.Get("cursor")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}

func (s *Server) foxxycodeSessionsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	fs := s.foxxycodeRequireStore(w)
	if fs == nil {
		return
	}
	includeScheduler := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_scheduler")), "true")
	includeSubagents := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_subagents")), "true")
	rows, err := fs.ListSnapshotsWith(session.ListOptions{
		IncludeSchedulerRuns: includeScheduler,
		IncludeSubagents:     includeSubagents,
	})
	if err != nil {
		s.log.Error("foxxycode sessions list", "error", err)
		http.Error(w, `{"error":{"message":"list failed"}}`, http.StatusInternalServerError)
		return
	}
	// Project scope: the IDE plugins run one server per project and pass the
	// project root so History does not mix every workspace the user ever opened.
	// Applied before paging so hasMore/nextCursor describe the filtered list.
	if scope := strings.TrimSpace(r.URL.Query().Get("cwd")); scope != "" {
		inScope := session.NewWorkspaceScope(scope)
		kept := rows[:0]
		for _, row := range rows {
			if inScope(row.CWD) {
				kept = append(kept, row)
			}
		}
		rows = kept
	}
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		rows, err = fs.FilterSnapshotListForSearch(rows, q)
		if err != nil {
			s.log.Error("foxxycode sessions list filter", "error", err)
			http.Error(w, `{"error":{"message":"list failed"}}`, http.StatusInternalServerError)
			return
		}
	}
	limit, offset := parseLimitCursor(r.URL.Query())
	start := offset
	if start >= len(rows) {
		out := map[string]interface{}{
			"object":     "foxxycode.session_list",
			"sessions":   []interface{}{},
			"nextCursor": nil,
			"hasMore":    false,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
		return
	}
	end := start + limit
	if end > len(rows) {
		end = len(rows)
	}
	slice := rows[start:end]
	includeActivity := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_activity")), "true")
	includeStats := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_stats")), "true")
	sessions := make([]map[string]interface{}, 0, len(slice))
	for _, row := range slice {
		ent := map[string]interface{}{
			"id": row.SessionID,
		}
		if row.Title != "" {
			ent["title"] = row.Title
		}
		if row.UpdatedAt != "" {
			ent["updatedAt"] = row.UpdatedAt
		}
		if row.CWD != "" {
			ent["cwd"] = row.CWD
		}
		if includeSubagents {
			if link := subagentRowLink(row); link != nil {
				ent["subagent"] = link
			}
		}
		if includeStats {
			// createdAt is absent for a bundle stored before the field existed;
			// model is absent for a session that never overrode the configured
			// default. Both stay out of the row rather than being guessed, so a
			// table can render an explicit "unknown" for them.
			if row.CreatedAt != "" {
				ent["createdAt"] = row.CreatedAt
			}
			if row.Model != "" {
				ent["model"] = row.Model
			}
			// Counted for the rows of this page only: the listing above read
			// session.json alone, and a transcript is by far the largest file of
			// a bundle. A file that cannot be read counts zero, as a transcript
			// that does not parse does when the session is opened.
			messageCount, _ := fs.MessageCount(row.SessionID)
			ent["messageCount"] = messageCount
			ent["tokenUsage"] = foxxycodeSessionTokenUsage(fs, row.SessionID)
		}
		if includeActivity {
			dir := fs.SessionPath(row.SessionID)
			turnActive := s.mgr.SessionTurnActiveInProcess(row.SessionID) || session.TurnLockHeld(dir)
			actSeq, readSeq, _ := fs.ReadDiskActivity(row.SessionID)
			ent["turnActive"] = turnActive
			ent["activitySeq"] = actSeq
			ent["readActivitySeq"] = readSeq
			ent["unreadComplete"] = actSeq > readSeq && !turnActive
			ent["permissionPending"] = session.PendingPermissionHeld(dir)
		}
		sessions = append(sessions, ent)
	}
	var nextCursor interface{}
	if end < len(rows) {
		nextCursor = strconv.Itoa(end)
	}
	out := map[string]interface{}{
		"object":     "foxxycode.session_list",
		"sessions":   sessions,
		"nextCursor": nextCursor,
		"hasMore":    end < len(rows),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// foxxycodeSessionTokenUsage reads the provider token totals a session accumulated.
// A bundle with no stats.json yet (a chat that never completed a model call)
// reports zeroes rather than nothing, so every row of the table has the same
// shape and sorts numerically.
func foxxycodeSessionTokenUsage(fs *session.FileStore, id string) map[string]int {
	usage := map[string]int{"inputTokens": 0, "outputTokens": 0, "totalTokens": 0}
	stats, err := session.ReadSessionStats(fs.SessionPath(id))
	if err != nil || stats == nil {
		return usage
	}
	usage["inputTokens"] = stats.TokenUsageTotal.InputTokens
	usage["outputTokens"] = stats.TokenUsageTotal.OutputTokens
	usage["totalTokens"] = stats.TokenUsageTotal.TotalTokens
	return usage
}

func (s *Server) foxxycodeSessionActivityGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if err := session.ValidateFolderSessionID(id); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusBadRequest)
		return
	}
	fs := s.foxxycodeRequireStore(w)
	if fs == nil {
		return
	}
	if !fs.HasPersistedSnapshot(id) {
		http.Error(w, `{"error":{"message":"session not found"}}`, http.StatusNotFound)
		return
	}
	dir := fs.SessionPath(id)
	turnActive := s.mgr.SessionTurnActiveInProcess(id) || session.TurnLockHeld(dir)
	actSeq, readSeq, err := fs.ReadDiskActivity(id)
	if err != nil {
		s.log.Error("foxxycode session activity", "error", err)
		http.Error(w, `{"error":{"message":"read failed"}}`, http.StatusInternalServerError)
		return
	}
	out := map[string]interface{}{
		"object":            "foxxycode.session_activity",
		"sessionId":         id,
		"turnActive":        turnActive,
		"activitySeq":       actSeq,
		"readActivitySeq":   readSeq,
		"unreadComplete":    actSeq > readSeq && !turnActive,
		"permissionPending": session.PendingPermissionHeld(dir),
	}
	// messageSeq moves inside a turn, which activitySeq does not: it advances
	// once per completed turn. A client polling a long turn uses it to skip a
	// transcript reload that would return the same thing. Only a session live
	// in this process can answer - the endpoint stays a cheap disk probe and
	// must not load a bundle to reply.
	if st := s.mgr.SessionByID(id); st != nil {
		out["messageSeq"] = st.MessageCount()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func llmMsgsToFoxxyCodeOpenAI(msgs []llm.Message) []map[string]interface{} {
	return llmMsgsToFoxxyCodeOpenAIForSession("", msgs)
}

// llmMsgsToFoxxyCodeOpenAIForSession is the transcript serializer with a session id,
// which is what a user row needs to point at its persisted image previews.
func llmMsgsToFoxxyCodeOpenAIForSession(sessionID string, msgs []llm.Message) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(msgs))
	for _, m := range msgs {
		item := map[string]interface{}{
			"role":    string(m.Role),
			"content": m.Content,
		}
		if strings.TrimSpace(m.Reasoning) != "" {
			item["reasoning"] = m.Reasoning
		}
		if m.ReasoningDurationMs > 0 {
			item["reasoning_duration_ms"] = m.ReasoningDurationMs
		}
		if m.Role == llm.RoleTool && m.ToolCallID != "" {
			item["tool_call_id"] = m.ToolCallID
		}
		if len(m.ToolCalls) > 0 {
			tc := make([]map[string]interface{}, 0, len(m.ToolCalls))
			for _, c := range m.ToolCalls {
				tc = append(tc, map[string]interface{}{
					"id":   c.ID,
					"type": "function",
					"function": map[string]string{
						"name":      c.Name,
						"arguments": c.InputJSON,
					},
				})
			}
			item["tool_calls"] = tc
		}
		if m.Role == llm.RoleAssistant && strings.TrimSpace(m.Model) != "" {
			item["model"] = strings.TrimSpace(m.Model)
		}
		if cat := strings.TrimSpace(m.CreatedAt); cat != "" {
			item["created_at"] = cat
		}
		// Compaction markers so the SPA renders the summary row as its own
		// CompactionMessage foldout and hides messages folded away by the
		// opencode engine. The coddy engine sets compaction_summary; the
		// opencode engine additionally sets compacted on the folded head.
		if m.CompactionSummary {
			item["compaction_summary"] = true
		}
		if m.Compacted {
			item["compacted"] = true
		}
		if m.Role == llm.RoleUser && len(m.ImageParts) > 0 {
			files := make([]map[string]interface{}, 0, len(m.ImageParts))
			for _, part := range m.ImageParts {
				name := strings.TrimSpace(part.Name)
				if name == "" && part.FilePath != "" {
					name = filepath.Base(part.FilePath)
				}
				file := map[string]interface{}{
					"name":      name,
					"mime_type": imagePartMIMEType(part),
				}
				if sessionID != "" && part.FilePath != "" && part.ThumbnailPath != "" {
					assetName := filepath.Base(part.FilePath)
					file["preview_url"] = "/foxxycode/sessions/" + url.PathEscape(sessionID) +
						"/assets/" + url.PathEscape(assetName) + "/thumbnail"
				}
				files = append(files, file)
			}
			item["files"] = files
		}
		if m.PlanDocument != nil {
			item["plan_document"] = map[string]interface{}{
				"slug":      m.PlanDocument.Slug,
				"name":      m.PlanDocument.Name,
				"overview":  m.PlanDocument.Overview,
				"content":   m.PlanDocument.Content,
				"body":      m.PlanDocument.Body,
				"path":      m.PlanDocument.Path,
				"discarded": m.PlanDocument.Discarded,
				"updatedAt": m.PlanDocument.UpdatedAt,
			}
		}
		out = append(out, item)
	}
	return out
}

// imagePartMIMEType names the media type of a persisted attachment: the data URI it
// arrived with when available, else the extension of its name or on-disk path.
func imagePartMIMEType(part llm.ImagePart) string {
	if strings.HasPrefix(part.DataURL, "data:") {
		end := strings.IndexAny(part.DataURL[5:], ";,")
		if end >= 0 {
			raw := part.DataURL[5 : 5+end]
			if mediaType, _, err := mime.ParseMediaType(raw); err == nil && mediaType != "" {
				return mediaType
			}
		}
	}
	for _, name := range []string{part.Name, part.FilePath} {
		if mediaType := mime.TypeByExtension(filepath.Ext(name)); mediaType != "" {
			if base, _, err := mime.ParseMediaType(mediaType); err == nil {
				return base
			}
			return mediaType
		}
	}
	return "application/octet-stream"
}

// foxxycodeSessionAssetThumbnailGet serves the bounded PNG preview of one uploaded
// image. Only thumbnails are exposed: the original asset bytes stay off the HTTP
// surface, and the name is constrained to a single path element.
func (s *Server) foxxycodeSessionAssetThumbnailGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	st := s.foxxycodeEnsureLoaded(w, r, id)
	if st == nil {
		return
	}
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		http.Error(w, `{"error":{"message":"invalid asset name"}}`, http.StatusBadRequest)
		return
	}
	sessionDir := strings.TrimSpace(st.GetPersistedSessionDir())
	if sessionDir == "" {
		http.NotFound(w, r)
		return
	}
	path := session.AssetThumbnailPath(sessionDir, name)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		s.log.Error("open session asset thumbnail", "error", err)
		http.Error(w, `{"error":{"message":"thumbnail unavailable"}}`, http.StatusInternalServerError)
		return
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, name+".png", info.ModTime(), f)
}

func (s *Server) foxxycodeSessionMessagesGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	st := s.foxxycodeEnsureLoaded(w, r, id)
	if st == nil {
		return
	}
	out := map[string]interface{}{
		"object":    "foxxycode.messages",
		"sessionId": id,
		"messages":  llmMsgsToFoxxyCodeOpenAIForSession(id, st.GetMessages()),
	}
	// A child session is a read-only transcript: the SPA drops the composer
	// and links back to the parent chat and to the task in its drawer.
	if meta := st.Subagent(); meta != nil {
		out["readOnly"] = true
		out["subagent"] = subagentLink(meta.ParentSessionID, meta.Name, meta.TaskID)
	}
	if s.activeCfg() != nil {
		out["selectedModelId"] = strings.TrimSpace(st.GetSelectedModelID())
		out["model"] = effectiveYAMLModel(s.activeCfg(), st)
		out["selectedReasoning"] = st.EffectiveReasoning(s.activeCfg())
		// The session profile, so a remote client restores it on load instead of
		// dropping every reopened session back to agent.
		out["mode"] = string(st.GetMode())
	}
	if u := st.GetUILog(); len(u) > 0 {
		rows := make([]map[string]interface{}, 0, len(u))
		for _, e := range u {
			rows = append(rows, map[string]interface{}{
				"id":            e.ID,
				"level":         e.Level,
				"message":       e.Message,
				"userTurnIndex": e.UserTurnIndex,
				"createdAt":     e.CreatedAt,
			})
		}
		out["uiLog"] = rows
	}
	if sd := strings.TrimSpace(st.GetPersistedSessionDir()); sd != "" {
		if env, err := session.ReadMemoryTrace(sd); err == nil && env != nil && len(env.Turns) > 0 {
			out["memoryTurns"] = env.Turns
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) foxxycodeSessionPatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.NotFound(w, r)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	var body struct {
		Title             string  `json:"title"`
		MarkActivityRead  bool    `json:"markActivityRead"`
		SelectedModelID   *string `json:"selectedModelId"`
		SelectedReasoning *string `json:"selectedReasoning"`
		Mode              *string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":{"message":"invalid JSON"}}`, http.StatusBadRequest)
		return
	}
	// Validated before anything is written, so a rejected mode cannot leave the
	// other keys of the same patch half-applied.
	patchMode := ""
	if body.Mode != nil {
		patchMode = strings.TrimSpace(*body.Mode)
		if !session.IsValidMode(patchMode) {
			http.Error(w, `{"error":{"message":"invalid mode"}}`, http.StatusBadRequest)
			return
		}
	}
	st := s.foxxycodeEnsureLoaded(w, r, id)
	if st == nil {
		return
	}
	fs := s.mgr.FileStore()
	resp := map[string]interface{}{
		"object": "foxxycode.session_patched",
		"id":     id,
	}
	did := false
	// The session profile, so the composer's Mode survives a reload even when
	// the user switched it without sending a turn. A turn writes it too
	// (POST /v1/responses carries the profile as its top-level model), but that
	// leaves the gap between picking a mode and using it.
	if patchMode != "" {
		st.SetMode(patchMode)
		did = true
		resp["mode"] = string(st.GetMode())
	}
	if body.SelectedModelID != nil {
		if err := applySessionYAMLModel(s.activeCfg(), st, *body.SelectedModelID); err != nil {
			if errors.Is(err, ErrUnknownMetadataModel) {
				http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusBadRequest)
				return
			}
			http.Error(w, `{"error":{"message":"invalid selectedModelId"}}`, http.StatusBadRequest)
			return
		}
		did = true
		resp["selectedModelId"] = strings.TrimSpace(st.GetSelectedModelID())
		if s.activeCfg() != nil {
			resp["model"] = effectiveYAMLModel(s.activeCfg(), st)
		}
	}
	if body.SelectedReasoning != nil {
		if err := applySessionReasoning(s.activeCfg(), st, *body.SelectedReasoning); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusBadRequest)
			return
		}
		did = true
		resp["selectedReasoning"] = strings.TrimSpace(st.GetSelectedReasoning())
	}
	if body.MarkActivityRead {
		st.MarkActivityReadSynced()
		did = true
		resp["activitySeq"] = st.GetActivitySeq()
		resp["readActivitySeq"] = st.GetReadActivitySeq()
		if fs != nil {
			if err := fs.PatchSessionMetaActivitySync(st); err != nil {
				s.log.Warn("patch session meta activity", "id", id, "error", err)
			}
		}
	}
	t := strings.TrimSpace(body.Title)
	if t != "" {
		st.SetTitlePinned(t)
		did = true
		resp["title"] = t
	}
	if !did {
		http.Error(w, `{"error":{"message":"title, markActivityRead, mode, selectedModelId, or selectedReasoning required"}}`, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// deleteSessionBundle removes one session tree. It is the shared body of
// DELETE /foxxycode/sessions/{id} and of every id a bulk delete works through, so
// both routes retract branch references and stop background work the same way.
// An id with no bundle on disk removes nothing and reports no error.
func (s *Server) deleteSessionBundle(id string) error {
	// Retract the session from the branch file of whatever it forked from, so the
	// branch navigator stops offering a thread that no longer exists. Read-side
	// filtering covers the failure case, so a prune error must not block delete.
	// It reads the session's own branch file, so it runs before the bundle goes.
	if err := s.mgr.PruneBranchRefs(id); err != nil {
		s.log.Warn("prune branch refs on delete", "session", id, "error", err)
	}
	// The manager removes the whole tree: the tasks representing this session's
	// subagent runs (and their descendants) are stopped and awaited first, then
	// every remaining task of every node, then the bundles deepest first, so
	// nothing writes into a directory that is already gone.
	return s.mgr.DeleteSessionTree(id, bgtask.Default())
}

// sessionDeleteStatus maps a delete failure onto the status the single-session
// route answers with: a tree that would not settle is a retryable conflict,
// anything else is a server error.
func sessionDeleteStatus(err error) int {
	if errors.Is(err, session.ErrTurnNotSettled) || errors.Is(err, session.ErrTreeUnstable) {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

func (s *Server) foxxycodeSessionDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.NotFound(w, r)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if err := session.ValidateFolderSessionID(id); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusBadRequest)
		return
	}
	fs := s.foxxycodeRequireStore(w)
	if fs == nil {
		return
	}
	if err := s.deleteSessionBundle(id); err != nil {
		status := sessionDeleteStatus(err)
		if status == http.StatusConflict {
			// A turn of the tree ignored its cancellation, or descendants
			// kept appearing while the tree was being marked; nothing was
			// removed, the client may retry once the tree is quiet.
			http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), status)
			return
		}
		s.log.Error("foxxycode session delete", "error", err)
		http.Error(w, `{"error":{"message":"delete failed"}}`, status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"object": "foxxycode.session_deleted", "id": id})
}

// foxxycodeSessionsBulkDeleteRequest is the body of POST /foxxycode/sessions/bulk-delete.
// Either an explicit list of ids, or scope "all" with an optional keep list -
// the session management table needs both: the rows an operator ticked, and
// "everything except the conversation I am in", which the client cannot spell
// as a list because it only ever holds one page of the history.
type foxxycodeSessionsBulkDeleteRequest struct {
	Scope  string   `json:"scope"`
	IDs    []string `json:"ids"`
	Except []string `json:"except"`
	// CWD confines the request to one workspace, compared as folders the way
	// the cwd filter of GET /foxxycode/sessions compares them. The editor
	// plugins run one server per project over a home every project shares, so a
	// table scoped to one project must not be able to remove another's chats.
	CWD string `json:"cwd"`
}

func (s *Server) foxxycodeSessionsBulkDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	fs := s.foxxycodeRequireStore(w)
	if fs == nil {
		return
	}
	var req foxxycodeSessionsBulkDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":{"message":"invalid JSON body"}}`, http.StatusBadRequest)
		return
	}
	scope := strings.ToLower(strings.TrimSpace(req.Scope))
	if scope == "" {
		scope = "ids"
	}
	workspace := strings.TrimSpace(req.CWD)
	if workspace != "" && !filepath.IsAbs(filepath.FromSlash(workspace)) {
		// Resolved against the server's own directory, a relative path would
		// scope a destructive request to a folder the caller never named.
		http.Error(w, `{"error":{"message":"cwd must be an absolute directory"}}`, http.StatusBadRequest)
		return
	}
	inWorkspace := session.NewWorkspaceScope(workspace)
	var targets []string
	switch scope {
	case "ids":
		if len(req.IDs) == 0 {
			http.Error(w, `{"error":{"message":"ids must not be empty"}}`, http.StatusBadRequest)
			return
		}
		if len(req.Except) > 0 {
			// Silently ignoring it would let a caller believe a session was
			// spared when the list never consulted the field.
			http.Error(w, `{"error":{"message":"except applies to scope \"all\" only"}}`, http.StatusBadRequest)
			return
		}
		for _, raw := range req.IDs {
			id := strings.TrimSpace(raw)
			if err := session.ValidateFolderSessionID(id); err != nil {
				http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusBadRequest)
				return
			}
			// Checked for every id before anything is removed, like a malformed
			// one: half a batch must not go before the refusal.
			if workspace != "" && !storedSessionInWorkspace(fs, id, inWorkspace) {
				http.Error(w, fmt.Sprintf(`{"error":{"message":"ids names a session outside cwd: %s"}}`, id), http.StatusBadRequest)
				return
			}
			targets = append(targets, id)
		}
	case "all":
		if len(req.IDs) > 0 {
			http.Error(w, `{"error":{"message":"ids and scope \"all\" are mutually exclusive"}}`, http.StatusBadRequest)
			return
		}
		// An exception is a promise that a named session survives, so it is
		// checked before anything is removed: a misspelt id that matched
		// nothing would quietly turn "keep this one" into "delete everything".
		keep := make(map[string]struct{}, len(req.Except))
		for _, raw := range req.Except {
			id := strings.TrimSpace(raw)
			if err := session.ValidateFolderSessionID(id); err != nil {
				http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusBadRequest)
				return
			}
			if !fs.HasPersistedSnapshot(id) {
				http.Error(w, fmt.Sprintf(`{"error":{"message":"except names a session that is not stored: %s"}}`, id), http.StatusBadRequest)
				return
			}
			// A session survives only if no ancestor of it is removed: the
			// delete takes a whole tree, so keeping a subagent child means
			// keeping the parent it hangs from. The child is not in the
			// listing below, its parent is.
			for _, ancestor := range sessionAncestry(fs, id) {
				keep[ancestor] = struct{}{}
			}
		}
		// "all" is resolved server side against the same listing the table
		// renders, so it means the whole history rather than the page the
		// client happens to have loaded. Scheduler runs stay out of it, and
		// subagent children go with the parent they belong to.
		rows, err := fs.ListSnapshotsWith(session.ListOptions{})
		if err != nil {
			s.log.Error("foxxycode sessions bulk delete list", "error", err)
			http.Error(w, `{"error":{"message":"list failed"}}`, http.StatusInternalServerError)
			return
		}
		for _, row := range rows {
			if _, skip := keep[row.SessionID]; skip {
				continue
			}
			if !inWorkspace(row.CWD) {
				continue
			}
			targets = append(targets, row.SessionID)
		}
	default:
		http.Error(w, `{"error":{"message":"scope must be \"ids\" or \"all\""}}`, http.StatusBadRequest)
		return
	}

	requested, deleted, failed := bulkDeleteSessions(targets, func(id string) error {
		err := s.deleteSessionBundle(id)
		if err != nil && sessionDeleteStatus(err) != http.StatusConflict {
			s.log.Error("foxxycode sessions bulk delete", "session", id, "error", err)
		}
		return err
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object":    "foxxycode.sessions_bulk_deleted",
		"requested": requested,
		"deleted":   deleted,
		"failed":    failed,
	})
}

// storedSessionInWorkspace reports whether a bulk delete confined to one
// workspace may remove id. An id with no bundle is admitted, because removing
// it removes nothing (the single-session route answers 200 for it as well); a
// stored one only when its session.json names a cwd inside the scope, so a
// bundle whose metadata cannot be read is refused rather than guessed at.
func storedSessionInWorkspace(fs *session.FileStore, id string, inWorkspace func(string) bool) bool {
	if !fs.HasPersistedSnapshot(id) {
		return true
	}
	meta, err := fs.ReadMeta(id)
	if err != nil {
		return false
	}
	return inWorkspace(meta.CWD)
}

// maxSessionAncestry bounds the parent walk so a corrupted bundle that points
// at itself, or a cycle written by an older build, cannot spin here.
const maxSessionAncestry = 32

// sessionAncestry returns id followed by every ancestor reachable through
// parentSessionId. Deleting any of them takes the whole subtree with it, so a
// caller that wants id to survive has to spare all of them.
func sessionAncestry(fs *session.FileStore, id string) []string {
	out := []string{id}
	seen := map[string]struct{}{id: {}}
	current := id
	for i := 0; i < maxSessionAncestry; i++ {
		// session.json names the parent; the transcript is not needed for it.
		meta, err := fs.ReadMeta(current)
		if err != nil {
			return out
		}
		parent := strings.TrimSpace(meta.ParentSessionID)
		if parent == "" {
			return out
		}
		if _, loop := seen[parent]; loop {
			return out
		}
		if err := session.ValidateFolderSessionID(parent); err != nil {
			return out
		}
		seen[parent] = struct{}{}
		out = append(out, parent)
		current = parent
	}
	return out
}

// bulkDeleteSessions removes every target once, in order, and reports what went
// and what stayed. One failing tree must not abandon the rest: a session in the
// middle of a turn answers a conflict and the batch carries on, so the table
// can drop the deleted rows and keep the others with their reason. The error
// text is the sentinel's own only for the two retryable conflicts; anything
// else is generic, because a filesystem error names paths the caller has no
// business reading.
func bulkDeleteSessions(targets []string, del func(string) error) (requested int, deleted []string, failed []map[string]string) {
	deleted = make([]string, 0, len(targets))
	failed = make([]map[string]string, 0)
	seen := make(map[string]struct{}, len(targets))
	for _, id := range targets {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		if err := del(id); err != nil {
			message := "delete failed"
			if sessionDeleteStatus(err) == http.StatusConflict {
				message = err.Error()
			}
			failed = append(failed, map[string]string{"id": id, "error": message})
			continue
		}
		deleted = append(deleted, id)
	}
	return len(seen), deleted, failed
}

func (s *Server) foxxycodePlanGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	st := s.foxxycodeEnsureLoaded(w, r, id)
	if st == nil {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object":  "foxxycode.plan",
		"entries": st.GetPlan(),
	})
}

func (s *Server) foxxycodePlanPut(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.NotFound(w, r)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	var body struct {
		Entries []acp.PlanEntry `json:"entries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":{"message":"invalid JSON"}}`, http.StatusBadRequest)
		return
	}
	st := s.foxxycodeEnsureLoaded(w, r, id)
	if st == nil {
		return
	}
	st.SetPlan(body.Entries)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"object": "foxxycode.plan_updated",
		"count":  len(body.Entries),
	})
}

func (s *Server) foxxycodePlanArchivePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	st := s.foxxycodeEnsureLoaded(w, r, id)
	if st == nil {
		return
	}
	entries := st.GetPlan()
	if len(entries) == 0 {
		st.SetPlan(nil)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"object": "foxxycode.plan_archived", "note": "no active items"})
		return
	}
	for i := range entries {
		if entries[i].Status != "completed" {
			entries[i].Status = "completed"
		}
	}
	md := todo.FormatPlanMarkdown(entries)
	sd := strings.TrimSpace(st.GetPersistedSessionDir())
	pathNote := ""
	if sd != "" {
		dest, err := session.WritePlanArchivedMarkdown(sd, md)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusInternalServerError)
			return
		}
		pathNote = dest
	}
	st.SetPlan(nil)
	resp := map[string]interface{}{"object": "foxxycode.plan_archived"}
	if pathNote != "" {
		resp["archivePath"] = pathNote
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
