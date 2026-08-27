//go:build http

package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/bgtask"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/project"
	"github.com/hijera/foxxycode-agent/internal/session"
)

var errSessionNotFound = errors.New("session not found")

var errInvalidSessionHeader = errors.New("invalid X-FoxxyCode-Session-ID")

// Server serves OpenAI-compatible HTTP endpoints.
type Server struct {
	cfgAt                atomic.Pointer[config.Config]
	mgr                  *session.Manager
	log                  *slog.Logger
	defaultCWD           string
	mux                  *http.ServeMux
	providerFactory      func(*config.Config) (llm.Provider, error)
	agentProviderFactory func(llm.ProviderInput) (llm.Provider, error)
	// extraAuthTokens are bearer tokens supplied out-of-band via --auth-token / FOXXYCODE_HTTP_TOKEN.
	extraAuthTokens []string
	// makeLLMFromYAML builds an LLM backend for a configured models[].model selector (direct completion). Tests override.
	makeLLMFromYAML func(*config.Config, string) (llm.Provider, error)

	// projects tracks the current project folder and recent list; nil
	// degrades the /foxxycode/project endpoints gracefully.
	projects     *project.Store
	folderPicker FolderPickerFunc
	pickerBusy   atomic.Bool

	slashMu    sync.Mutex
	slashCache map[string]slashListCacheEntry

	// mcpProbeCache holds probed MCP tool inventories for /foxxycode/mcp (keyed
	// by server name, invalidated on config fingerprint change or edit).
	mcpProbeMu    sync.Mutex
	mcpProbeCache map[string]mcpProbeEntry

	composerRelayMu sync.Mutex
	composerRelays  map[string]*composerStreamRelay

	// miniAppsState is initialized lazily by the optional Mini Apps transport.
	// It remains an any value so the lean http build does not import that tag.
	miniAppsMu    sync.Mutex
	miniAppsState any

	// events fans server-wide turn lifecycle events out to GET /foxxycode/events subscribers.
	events             *serverEventsHub
	removeTurnObserver func()

	codexAuthIssuer string
	// codexAuthMu guards both browser-login attempt maps; the attempts share
	// one bookkeeping shape and one drain path.
	codexAuthMu          sync.Mutex
	codexAuthLogins      map[string]*codexAuthLoginAttempt
	neuralDeepAuthLogins map[string]*codexAuthLoginAttempt

	permissionResumeWG sync.WaitGroup
	bgWG               sync.WaitGroup
}

// Drain waits for all background goroutines (e.g. turn-diff writers) to finish.
// Call after closing the HTTP server and before tearing down any session directories.
func (s *Server) Drain() {
	if s.removeTurnObserver != nil {
		s.removeTurnObserver()
	}
	s.cancelCodexAuthLogins()
	s.cancelNeuralDeepAuthLogins()
	s.miniAppsDrain()
	// Background tasks are children of this process; leaving them running would
	// orphan whole shell trees the operator can no longer see or stop. Close the
	// pool first so a turn that is still winding down cannot start one more, and
	// so a wake retrying on a busy session gives up instead of holding bgWG.
	bgtask.Default().SetDraining(true)
	bgtask.Default().StopAll()
	s.bgWG.Wait()
}

// New creates an HTTP server wrapper (handlers registered on mux).
func New(cfg *config.Config, mgr *session.Manager, log *slog.Logger, defaultCWD string) *Server {
	s := &Server{
		mgr:                  mgr,
		log:                  log,
		defaultCWD:           defaultCWD,
		mux:                  http.NewServeMux(),
		providerFactory:      defaultProviderFromAgentModel,
		agentProviderFactory: llm.NewProvider,
		makeLLMFromYAML:      defaultMakeLLMFromYAML,
		slashCache:           make(map[string]slashListCacheEntry),
		codexAuthIssuer:      llm.CodexIssuerURL,
		codexAuthLogins:      make(map[string]*codexAuthLoginAttempt),
		neuralDeepAuthLogins: make(map[string]*codexAuthLoginAttempt),
		events:               newServerEventsHub(),
	}
	s.cfgAt.Store(cfg)
	// Several servers may share one manager (tests do), so each takes its own removable
	// observer registration rather than a single manager-wide slot.
	if mgr != nil {
		s.removeTurnObserver = mgr.AddTurnObserver(s.publishTurnEvent)
	}
	// A fresh server means this process intends to serve again, so reopen the
	// task pool a previous Drain closed.
	bgtask.Default().SetDraining(false)
	s.attachBackgroundWaker()
	s.mux.HandleFunc("GET /v1/models", s.handleModels)
	s.mux.HandleFunc("POST /v1/chat/completions", s.handleChatCompletions)
	s.mux.HandleFunc("POST /v1/responses", s.handleResponsesCreate)
	s.mux.HandleFunc("GET /v1/responses/{id}", s.handleResponsesGetPath)
	s.registerFoxxyCodeRoutes()
	s.registerProjectRoutes()
	s.registerOnboardingRoutes()
	s.registerConfigRoutes()
	s.registerProvidersRoutes()
	s.mux.HandleFunc("GET /openapi.yaml", s.handleOpenAPIYAML)
	s.mux.HandleFunc("GET /openapi.json", s.handleOpenAPIJSON)
	s.mux.HandleFunc("GET /docs", s.redirectDocsTrailingSlash)
	swaggerSub, err := fs.Sub(swaggerStatic, "swagger-static")
	if err != nil {
		log.Error("swagger static subtree", "error", err)
	} else {
		s.mux.Handle("GET /docs/", http.StripPrefix("/docs/", http.FileServer(http.FS(swaggerSub))))
	}
	mountEmbeddedSPARoot(s)
	return s
}

func (s *Server) activeCfg() *config.Config {
	return s.cfgAt.Load()
}

// ReplaceConfig updates the in-memory config used by HTTP handlers.
func (s *Server) ReplaceConfig(c *config.Config) {
	if c != nil {
		s.cfgAt.Store(c)
	}
}

func defaultProviderFromAgentModel(cfg *config.Config) (llm.Provider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config unavailable")
	}
	modelRef := strings.TrimSpace(cfg.Agent.Model)
	if modelRef == "" {
		return nil, fmt.Errorf("agent.model is empty")
	}
	rm, err := cfg.ResolveLLM(modelRef)
	if err != nil {
		return nil, err
	}
	maxTok := rm.MaxTokens
	if maxTok <= 0 || maxTok > 96 {
		maxTok = 96
	}
	return llm.NewProvider(llm.WithAgentResilience(llm.ProviderInput{
		Type:          rm.ProviderType,
		Model:         rm.Model,
		APIKey:        rm.APIKey,
		BaseURL:       rm.BaseURL,
		ProxyURL:      rm.ProxyURL,
		AuthPath:      rm.AuthPath,
		MaxTokens:     maxTok,
		Temperature:   rm.Temperature,
		DisableStream: !rm.Stream,
	}, cfg.Agent.EffectiveLLMRetryMax(), cfg.Agent.LLMRetryBaseMS, cfg.Agent.LLMMinIntervalMS))
}

func defaultMakeLLMFromYAML(cfg *config.Config, yamlSel string) (llm.Provider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config unavailable")
	}
	yamlSel = strings.TrimSpace(yamlSel)
	if yamlSel == "" {
		return nil, fmt.Errorf("model selector empty")
	}
	rm, err := cfg.ResolveLLM(yamlSel)
	if err != nil {
		return nil, err
	}
	maxTok := resolveDirectYAMLMaxTokens(rm)
	return llm.NewProvider(llm.WithAgentResilience(llm.ProviderInput{
		Type:          rm.ProviderType,
		Model:         rm.Model,
		APIKey:        rm.APIKey,
		BaseURL:       rm.BaseURL,
		ProxyURL:      rm.ProxyURL,
		AuthPath:      rm.AuthPath,
		MaxTokens:     maxTok,
		Temperature:   rm.Temperature,
		DisableStream: !rm.Stream,
	}, cfg.Agent.EffectiveLLMRetryMax(), cfg.Agent.LLMRetryBaseMS, cfg.Agent.LLMMinIntervalMS))
}

func (s *Server) redirectDocsTrailingSlash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/docs/", http.StatusFound)
}

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler {
	return s.corsMiddleware(s.authGate(s.slowRequestLog(s.mux)))
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	type modelObj struct {
		ID               string   `json:"id"`
		Object           string   `json:"object"`
		Created          int64    `json:"created"`
		OwnedBy          string   `json:"owned_by"`
		MaxContextTokens int      `json:"max_context_tokens,omitempty"`
		Multimodal       bool     `json:"multimodal,omitempty"`
		ReasoningLevels  []string `json:"reasoning_levels,omitempty"`
		ReasoningDefault string   `json:"reasoning_default,omitempty"`
	}
	out := struct {
		Object            string     `json:"object"`
		Data              []modelObj `json:"data"`
		DefaultAgentModel string     `json:"default_agent_model,omitempty"`
	}{
		Object: "list",
		Data:   nil,
	}
	if s.activeCfg() != nil {
		if dm := strings.TrimSpace(s.activeCfg().Agent.Model); dm != "" {
			out.DefaultAgentModel = dm
		}
	}
	maxCtx := maxContextDefault(s)
	for _, mode := range []session.Mode{session.ModeAgent, session.ModePlan, session.ModeDocs, session.ModeAsk} {
		out.Data = append(out.Data, modelObj{
			ID:               string(mode),
			Object:           "model",
			Created:          0,
			OwnedBy:          ownedByFoxxyCodeSession,
			MaxContextTokens: maxCtx,
		})
	}
	if s.activeCfg() != nil {
		for i := range s.activeCfg().Models {
			ent := &s.activeCfg().Models[i]
			mid := strings.TrimSpace(ent.Model)
			if mid == "" {
				continue
			}
			mc := maxCtx
			if ent.MaxContextTokens > 0 {
				mc = ent.MaxContextTokens
			}
			out.Data = append(out.Data, modelObj{
				ID:               mid,
				Object:           "model",
				Created:          0,
				OwnedBy:          ent.ProviderName(),
				MaxContextTokens: mc,
				Multimodal:       ent.Multimodal,
				ReasoningLevels:  s.activeCfg().ReasoningLevelsFor(ent),
				ReasoningDefault: s.activeCfg().DefaultReasoningLevelFor(ent),
			})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

type chatCompletionRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	MaxTok   int             `json:"max_tokens"`
	Temp     float64         `json:"temperature"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

type openAIMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCallID string          `json:"tool_call_id"`
	Name       string          `json:"name"`
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var req chatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":{"message":"invalid JSON"}}`, http.StatusBadRequest)
		return
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		http.Error(w, `{"error":{"message":"model is required"}}`, http.StatusBadRequest)
		return
	}
	if !httpModelListed(s.activeCfg(), model) {
		http.Error(w, `{"error":{"message":"unknown model"}}`, http.StatusBadRequest)
		return
	}
	if err := coerceMetadataJSON(req.Metadata); err != nil {
		http.Error(w, `{"error":{"message":"invalid metadata"}}`, http.StatusBadRequest)
		return
	}
	msgs, err := openAIMessagesToLLM(req.Messages)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusBadRequest)
		return
	}
	if len(msgs) == 0 {
		http.Error(w, `{"error":{"message":"messages required"}}`, http.StatusBadRequest)
		return
	}
	last := msgs[len(msgs)-1]
	if last.Role != llm.RoleUser {
		http.Error(w, `{"error":{"message":"last message must be user"}}`, http.StatusBadRequest)
		return
	}
	prefix := msgs[:len(msgs)-1]

	ctx := r.Context()
	st, sessionID, createdNew, err := s.resolveSession(ctx, r)
	if err != nil {
		if errors.Is(err, errSessionNotFound) {
			http.Error(w, `{"error":{"message":"session not found"}}`, http.StatusNotFound)
			return
		}
		if errors.Is(err, errInvalidSessionHeader) {
			http.Error(w, `{"error":{"message":"invalid X-FoxxyCode-Session-ID"}}`, http.StatusBadRequest)
			return
		}
		http.Error(w, `{"error":{"message":"session unavailable"}}`, http.StatusInternalServerError)
		return
	}
	if createdNew {
		w.Header().Set("X-FoxxyCode-Session-ID", sessionID)
	}

	if httpModelIsFoxxyCodeProfile(model) {
		st.SetMode(model)
		if _, err := profileMetadataPatch(s.activeCfg(), st, req.Metadata); err != nil {
			if errors.Is(err, ErrInvalidMetadataModel) || errors.Is(err, ErrUnknownMetadataModel) {
				http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusBadRequest)
				return
			}
			http.Error(w, `{"error":{"message":"invalid metadata"}}`, http.StatusBadRequest)
			return
		}
	} else if completionMetadataForbidden(req.Metadata) {
		http.Error(w, `{"error":{"message":"metadata.model is not allowed for direct completion"}}`, http.StatusBadRequest)
		return
	}

	var bridge *Sender
	if httpModelIsFoxxyCodeProfile(model) {
		st.ReplaceMessagesWithoutPersist(prefix)
		prompt := []acp.ContentBlock{{Type: "text", Text: last.Content}}
		// Every profile turn publishes to a relay, whatever shape the caller asked its own
		// answer to take: a script POSTing stream:false is exactly the turn someone wants to
		// watch from a browser. The lock is taken first in both branches - beginComposerRelay
		// evicts any relay already registered for the session, so an unlocked second POST
		// could cut the watchers off the first turn.
		unlock, lockErr := s.mgr.AcquireComposerTurnLockWaiting(ctx, sessionID, st, composerTurnLockWait)
		if lockErr != nil {
			if errors.Is(lockErr, session.ErrSessionTurnBusy) {
				writeSessionBusy(w, sessionID, sessionBusyMessage)
				return
			}
			s.log.Error("session turn lock", "error", lockErr)
			http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, lockErr.Error()), http.StatusInternalServerError)
			return
		}
		defer unlock()
		rel := s.beginComposerRelay(sessionID)
		defer s.endComposerRelay(sessionID, rel)
		if req.Stream {
			writeSSEHeaders(w)
			bridge = NewSender(s.activeCfg(), &teeSSEWriter{ResponseWriter: w, relay: rel}, true, model)
		} else {
			bridge = NewRelaySender(s.activeCfg(), rel, model)
		}
		wireBridgeSession(bridge, st)
		promptOpts := &session.PromptRunOpts{SkipTurnLock: true, DetachFromRequest: req.Stream}
		beforeSnap := session.TakeWorkspaceSnapshot(st.GetCWD())
		// A model configured with stream: false emits nothing until its whole answer is
		// generated, so the stream has to announce it is still alive by itself.
		stopKeepalive := bridge.StartIdleKeepalive()
		defer stopKeepalive()
		promptRes, err := s.mgr.HandleSessionPromptWithSender(ctx, acp.SessionPromptParams{
			SessionID: sessionID,
			Prompt:    prompt,
			Meta:      sessionPromptMetaFromHTTP(req.Metadata),
		}, bridge, promptOpts)
		stopKeepalive()
		if err != nil {
			s.log.Error("session prompt", "error", err)
			// Watchers hear about the failure either way; only the caller's own answer
			// differs between the two response shapes.
			_ = bridge.SendError(err)
			_ = bridge.FinishStream()
			if !req.Stream {
				if errors.Is(err, session.ErrSessionTurnBusy) {
					writeSessionBusy(w, sessionID, sessionBusyMessage)
					return
				}
				http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusInternalServerError)
			}
			return
		}
		s.captureAndStoreTurnDiff(st, beforeSnap)
		meta := metadataResponse(s.activeCfg(), effectiveYAMLModel(s.activeCfg(), st))
		if promptRes != nil && promptRes.StopReason != "" {
			// Remote clients (internal/remote) recover the ACP stop reason
			// from here; [DONE] alone cannot carry it.
			meta["stop_reason"] = string(promptRes.StopReason)
		}
		// Unconditional: for a relay sender this terminates the watched stream and writes
		// nothing to w, so the JSON body below is unchanged.
		_ = bridge.FinishStreamWithMetadata(meta)
		if req.Stream {
			return
		}
		reply := lastAssistantContent(st)
		resp := map[string]interface{}{
			"id":       bridge.ChatID(),
			"object":   "chat.completion",
			"created":  time.Now().Unix(),
			"model":    model,
			"metadata": meta,
			"choices": []map[string]interface{}{{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": reply,
				},
				"finish_reason": "stop",
			}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	if req.Stream {
		writeSSEHeaders(w)
		bridge = NewSender(s.activeCfg(), w, true, model)
	} else {
		bridge = NewSender(s.activeCfg(), nil, false, model)
	}
	st.ReplaceMessagesWithoutPersist(prefix)
	st.AddMessage(llm.Message{
		Role:      llm.RoleUser,
		Content:   last.Content,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
	turnCtx, cancelTurn := context.WithCancel(ctx)
	st.SetCancel(cancelTurn)
	defer cancelTurn()
	if _, err := s.runDirectYAMLCompletion(turnCtx, st, sessionID, model, bridge); err != nil {
		if errors.Is(err, context.Canceled) && req.Stream {
			meta := metadataResponse(s.activeCfg(), model)
			_ = bridge.FinishStreamWithMetadata(meta)
			return
		}
		if !errors.Is(err, context.Canceled) {
			st.AppendUILogError(session.CountUserTurns(st.GetMessages()), err.Error())
		}
		s.log.Error("direct completion", "error", err)
		if req.Stream {
			_ = bridge.SendError(err)
		} else {
			http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusInternalServerError)
		}
		return
	}
	meta := metadataResponse(s.activeCfg(), model)
	if req.Stream {
		_ = bridge.FinishStreamWithMetadata(meta)
		return
	}
	reply := lastAssistantContent(st)
	resp := map[string]interface{}{
		"id":       bridge.ChatID(),
		"object":   "chat.completion",
		"created":  time.Now().Unix(),
		"model":    model,
		"metadata": meta,
		"choices": []map[string]interface{}{{
			"index": 0,
			"message": map[string]string{
				"role":    "assistant",
				"content": reply,
			},
			"finish_reason": "stop",
		}},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// sessionBusyMessage is the human-readable half of every session-busy conflict.
const sessionBusyMessage = "session busy: another agent turn is in progress"

// composerTurnLockWait is how long a streaming composer POST waits for the previous turn
// to release the session before reporting it busy. Sized for the Stop-then-resend case
// (a cancelled turn still persists and diffs the workspace on its way out), not for
// queueing behind a turn that is genuinely still working.
const composerTurnLockWait = 3 * time.Second

// writeSessionBusy answers a 409 with a machine-readable body so clients can react to a
// live turn (re-attach to it) instead of only surfacing the message text. sessionID may be
// empty when the handler does not know it.
func writeSessionBusy(w http.ResponseWriter, sessionID, message string) {
	if strings.TrimSpace(message) == "" {
		message = sessionBusyMessage
	}
	errBody := map[string]interface{}{
		"message":    message,
		"code":       "session_busy",
		"turnActive": true,
	}
	if id := strings.TrimSpace(sessionID); id != "" {
		errBody["sessionId"] = id
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": errBody})
}

func (s *Server) resolveSession(ctx context.Context, r *http.Request) (st *session.State, id string, createdNew bool, err error) {
	sid := strings.TrimSpace(r.Header.Get("X-FoxxyCode-Session-ID"))
	if sid != "" {
		if err := session.ValidateFolderSessionID(sid); err != nil {
			return nil, "", false, errInvalidSessionHeader
		}
		st2, err := s.mgr.EnsureHTTPSession(ctx, sid, s.sessionDefaultCWD())
		if err != nil {
			return nil, "", false, err
		}
		return st2, sid, false, nil
	}
	res, err := s.mgr.HandleSessionNew(ctx, acp.SessionNewParams{CWD: s.sessionDefaultCWD()})
	if err != nil {
		return nil, "", false, err
	}
	st = s.mgr.SessionByID(res.SessionID)
	if st == nil {
		return nil, "", false, fmt.Errorf("internal session")
	}
	return st, res.SessionID, true, nil
}

func openAIMessagesToLLM(messages []openAIMessage) ([]llm.Message, error) {
	out := make([]llm.Message, 0, len(messages))
	for _, m := range messages {
		role := strings.TrimSpace(m.Role)
		switch role {
		case "system":
			txt, err := stringContent(m.Content)
			if err != nil {
				return nil, err
			}
			out = append(out, llm.Message{Role: llm.RoleSystem, Content: txt})
		case "user":
			txt, err := stringContent(m.Content)
			if err != nil {
				return nil, err
			}
			out = append(out, llm.Message{Role: llm.RoleUser, Content: txt})
		case "assistant":
			txt, err := stringContent(m.Content)
			if err != nil {
				return nil, err
			}
			out = append(out, llm.Message{Role: llm.RoleAssistant, Content: txt})
		case "tool":
			txt, err := stringContent(m.Content)
			if err != nil {
				return nil, err
			}
			out = append(out, llm.Message{
				Role:       llm.RoleTool,
				Content:    txt,
				ToolCallID: strings.TrimSpace(m.ToolCallID),
			})
		default:
			return nil, fmt.Errorf("unsupported role %q", role)
		}
	}
	return out, nil
}

func stringContent(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", err
		}
		return s, nil
	}
	return string(raw), nil
}

// inlineFileJSON is a base64-encoded file sent from the browser file picker.
type inlineFileJSON struct {
	// Name is the original file name (e.g. "photo.png").
	Name string `json:"name"`
	// DataURL is a data URI: "data:<mime>;base64,<bytes>".
	DataURL string `json:"data_url"`
}

func inlineFilesToImageParts(files []inlineFileJSON) []llm.ImagePart {
	if len(files) == 0 {
		return nil
	}
	parts := make([]llm.ImagePart, len(files))
	for i, f := range files {
		parts[i] = llm.ImagePart{DataURL: f.DataURL, Name: f.Name}
	}
	return parts
}

func lastAssistantContent(st *session.State) string {
	msgs := st.GetMessages()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == llm.RoleAssistant {
			return msgs[i].Content
		}
	}
	return ""
}

// POST /v1/responses accepts model, input, and optional stream (SSE).
func (s *Server) handleResponsesCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var body struct {
		Model       string                         `json:"model"`
		Input       string                         `json:"input"`
		Stream      bool                           `json:"stream"`
		Metadata    json.RawMessage                `json:"metadata,omitempty"`
		Attachments []session.PromptFileAttachment `json:"attachments,omitempty"`
		// InlineFiles carries base64 data URIs from the browser file picker.
		// It is forwarded only when the effective YAML model is multimodal.
		InlineFiles []inlineFileJSON `json:"inline_files,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":{"message":"invalid JSON"}}`, http.StatusBadRequest)
		return
	}
	model := strings.TrimSpace(body.Model)
	if model == "" {
		http.Error(w, `{"error":{"message":"unknown or missing model"}}`, http.StatusBadRequest)
		return
	}
	if !httpModelListed(s.activeCfg(), model) {
		http.Error(w, `{"error":{"message":"unknown or missing model"}}`, http.StatusBadRequest)
		return
	}
	if err := coerceMetadataJSON(body.Metadata); err != nil {
		http.Error(w, `{"error":{"message":"invalid metadata"}}`, http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	st, sid, createdNew, err := s.resolveSession(ctx, r)
	if err != nil {
		if errors.Is(err, errSessionNotFound) {
			http.Error(w, `{"error":{"message":"session not found"}}`, http.StatusNotFound)
			return
		}
		if errors.Is(err, errInvalidSessionHeader) {
			http.Error(w, `{"error":{"message":"invalid X-FoxxyCode-Session-ID"}}`, http.StatusBadRequest)
			return
		}
		http.Error(w, `{"error":{"message":"session unavailable"}}`, http.StatusInternalServerError)
		return
	}
	if createdNew {
		w.Header().Set("X-FoxxyCode-Session-ID", sid)
	}

	if httpModelIsFoxxyCodeProfile(model) {
		st.SetMode(model)
		if _, err := profileMetadataPatch(s.activeCfg(), st, body.Metadata); err != nil {
			if errors.Is(err, ErrInvalidMetadataModel) || errors.Is(err, ErrUnknownMetadataModel) {
				http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusBadRequest)
				return
			}
			http.Error(w, `{"error":{"message":"invalid metadata"}}`, http.StatusBadRequest)
			return
		}
	} else if completionMetadataForbidden(body.Metadata) {
		http.Error(w, `{"error":{"message":"metadata.model is not allowed for direct completion"}}`, http.StatusBadRequest)
		return
	}
	if len(body.Attachments) > 0 && !httpModelIsFoxxyCodeProfile(model) {
		http.Error(w, `{"error":{"message":"attachments are only supported for agent, plan, docs, or ask model"}}`, http.StatusBadRequest)
		return
	}
	// Fail closed: a model that never declared multimodal must not be handed image
	// bytes, whichever surface uploaded them. The composer already hides the control,
	// but a stale tab or a script can still send them.
	inlineFiles := body.InlineFiles
	effectiveModel := model
	if httpModelIsFoxxyCodeProfile(model) {
		effectiveModel = effectiveYAMLModel(s.activeCfg(), st)
	}
	if !configuredModelMultimodal(s.activeCfg(), effectiveModel) {
		inlineFiles = nil
	}
	// inline_files are supported for both direct YAML calls and session profiles.

	if httpModelIsFoxxyCodeProfile(model) {
		cwdAbs, err := filepath.Abs(st.GetCWD())
		if err != nil {
			s.log.Error("responses prompt cwd", "error", err)
			http.Error(w, `{"error":{"message":"session cwd unavailable"}}`, http.StatusInternalServerError)
			return
		}
		promptBlocks, err := session.BuildHydratedComposerPrompt(cwdAbs, strings.TrimSpace(body.Input), body.Attachments)
		if err != nil {
			code := http.StatusBadRequest
			if !errors.Is(err, session.ErrPathTraversal) &&
				!errors.Is(err, session.ErrFolderAttach) &&
				!errors.Is(err, session.ErrNotDecodableText) &&
				!os.IsNotExist(err) &&
				!strings.Contains(err.Error(), "file too large") &&
				!strings.Contains(err.Error(), "UTF-8") &&
				!strings.Contains(err.Error(), "invalid attachment") {
				code = http.StatusInternalServerError
			}
			if code == http.StatusInternalServerError {
				s.log.Error("responses prompt attachments", "error", err)
			}
			if body.Stream {
				writeSSEHeaders(w)
				_, _ = io.WriteString(w, fmt.Sprintf("data: {\"error\":{\"message\":%q}}\n\n", err.Error()))
			} else {
				http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), code)
			}
			return
		}

		// See the same block in handleChatCompletions: the relay is opened for every profile
		// turn, and the turn lock is taken before it in both branches.
		var bridge *Sender
		unlock, lockErr := s.mgr.AcquireComposerTurnLockWaiting(ctx, sid, st, composerTurnLockWait)
		if lockErr != nil {
			if errors.Is(lockErr, session.ErrSessionTurnBusy) {
				writeSessionBusy(w, sid, sessionBusyMessage)
				return
			}
			s.log.Error("session turn lock", "error", lockErr)
			http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, lockErr.Error()), http.StatusInternalServerError)
			return
		}
		defer unlock()
		rel := s.beginComposerRelay(sid)
		defer s.endComposerRelay(sid, rel)
		if body.Stream {
			writeSSEHeaders(w)
			bridge = NewSender(s.activeCfg(), &teeSSEWriter{ResponseWriter: w, relay: rel}, true, model)
		} else {
			bridge = NewRelaySender(s.activeCfg(), rel, model)
		}
		wireBridgeSession(bridge, st)
		promptOpts := &session.PromptRunOpts{SkipTurnLock: true, DetachFromRequest: body.Stream}
		beforeSnap2 := session.TakeWorkspaceSnapshot(st.GetCWD())
		promptParams := acp.SessionPromptParams{
			SessionID: sid,
			Prompt:    promptBlocks,
			Meta:      sessionPromptMetaFromHTTP(body.Metadata),
		}
		if len(inlineFiles) > 0 {
			promptParams.ImageParts = make([]acp.ImagePartRef, len(inlineFiles))
			for i, f := range inlineFiles {
				promptParams.ImageParts[i] = acp.ImagePartRef{DataURL: f.DataURL, Name: f.Name}
			}
		}
		// See the /v1/chat/completions path: a blocking model turn is silent on the
		// wire until it finishes, and idle proxies drop a stream that says nothing.
		stopKeepalive := bridge.StartIdleKeepalive()
		defer stopKeepalive()
		promptRes, err := s.mgr.HandleSessionPromptWithSender(ctx, promptParams, bridge, promptOpts)
		stopKeepalive()
		if err != nil {
			s.log.Error("responses prompt", "error", err)
			_ = bridge.SendError(err)
			_ = bridge.FinishStream()
			if !body.Stream {
				if errors.Is(err, session.ErrSessionTurnBusy) {
					writeSessionBusy(w, sid, sessionBusyMessage)
					return
				}
				http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusInternalServerError)
			}
			return
		}
		s.captureAndStoreTurnDiff(st, beforeSnap2)
		meta := metadataResponse(s.activeCfg(), effectiveYAMLModel(s.activeCfg(), st))
		if promptRes != nil && promptRes.StopReason != "" {
			// Remote clients (internal/remote) recover the ACP stop reason
			// from here; [DONE] alone cannot carry it.
			meta["stop_reason"] = string(promptRes.StopReason)
		}
		_ = bridge.FinishStreamWithMetadata(meta)
		if body.Stream {
			return
		}
		text := lastAssistantContent(st)
		out := map[string]interface{}{
			"id":       sid,
			"object":   "response",
			"status":   "completed",
			"model":    model,
			"metadata": meta,
			"output":   []map[string]string{{"type": "text", "text": text}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
		return
	}

	var bridge *Sender
	// Persist the direct-completion attachments too, so the transcript can show the
	// same chips and thumbnails a profile turn gets.
	imageParts := inlineFilesToImageParts(inlineFiles)
	if err := session.SavePartsToAssets(imageParts, st.GetPersistedSessionDir()); err != nil {
		s.log.Error("responses direct completion assets", "error", err)
		http.Error(w, `{"error":{"message":"save inline files failed"}}`, http.StatusInternalServerError)
		return
	}

	if body.Stream {
		writeSSEHeaders(w)
		bridge = NewSender(s.activeCfg(), w, true, model)
	} else {
		bridge = NewSender(s.activeCfg(), nil, false, model)
	}
	st.AddMessage(llm.Message{
		Role:       llm.RoleUser,
		Content:    strings.TrimSpace(body.Input),
		ImageParts: imageParts,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
	})
	respTurnCtx, respCancelTurn := context.WithCancel(ctx)
	st.SetCancel(respCancelTurn)
	defer respCancelTurn()
	if _, err := s.runDirectYAMLCompletion(respTurnCtx, st, sid, model, bridge); err != nil {
		if errors.Is(err, context.Canceled) && body.Stream {
			meta := metadataResponse(s.activeCfg(), model)
			_ = bridge.FinishStreamWithMetadata(meta)
			return
		}
		if !errors.Is(err, context.Canceled) {
			st.AppendUILogError(session.CountUserTurns(st.GetMessages()), err.Error())
		}
		s.log.Error("responses direct completion", "error", err)
		if body.Stream {
			_ = bridge.SendError(err)
		} else {
			http.Error(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()), http.StatusInternalServerError)
		}
		return
	}
	meta := metadataResponse(s.activeCfg(), model)
	if body.Stream {
		_ = bridge.FinishStreamWithMetadata(meta)
		return
	}
	text := lastAssistantContent(st)
	out := map[string]interface{}{
		"id":       sid,
		"object":   "response",
		"status":   "completed",
		"model":    model,
		"metadata": meta,
		"output":   []map[string]string{{"type": "text", "text": text}},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) handleResponsesGetPath(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}
	st := s.mgr.SessionByID(id)
	if st == nil {
		http.Error(w, `{"error":{"message":"not found"}}`, http.StatusNotFound)
		return
	}
	out := map[string]interface{}{
		"id":     id,
		"object": "response",
		"status": "completed",
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
