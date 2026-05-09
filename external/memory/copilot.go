package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/EvilFreelancer/coddy-agent/internal/config"
	"github.com/EvilFreelancer/coddy-agent/internal/llm"
)

const recallSystem = `You are the memory retrieval step for a coding agent. You never speak to the end user directly.
Your only job is to load useful long-term notes from disk BEFORE the main assistant answers.

Tools available here are READ-ONLY (search + read only). Do not attempt to save, edit, or delete files in this phase: persistence is handled by a separate curator after the main reply.

Paths are always scope:relative where scope is global or project.
Global memory uses memory.dir from config when set, otherwise $CODDY_HOME/memory (often ~/.coddy/memory). Project memory is always <session cwd>/memory.

Rules:
- Call coddy_memory_search first unless you already know the exact path to read.
- Prefer short factual bullets in your final reply. Do not expose raw tool JSON.
- Do not invent facts you did not read from files.
- If nothing is relevant, reply with a single line exactly: (no memory hits)
- When you finish gathering, answer in plain text without further tool calls.`

const judgeSystem = `You are a strict memory curator for a coding agent. You decide whether ONE distilled fact from this assistant turn should be persisted for future sessions.
Reply with a single JSON object only, no markdown fences. Schema:
{"save":false,"title":"","body":"","scope":"global","reason":"..."}
When save is true use scope "global" or "project". Body must be markdown, at most 900 characters, no API keys, tokens, passwords, or one-off secrets.

Set save to true ONLY when at least one holds:
- The user explicitly asked to remember / store / save something for later sessions, and the assistant agreed to a concrete fact; OR
- The assistant stated a durable preference or project fact (stack, coding style, naming, architecture decision) that will clearly help future turns in this repo.

Set save to false for: transient debugging, one-off errors, task status, chat filler, duplicate of obvious context, pure social chat, or anything that is not clearly reusable later.
When in doubt, save false.`

func clampProviderMax(rm *config.ResolvedLLM, cap int) {
	if rm == nil || cap <= 0 {
		return
	}
	if rm.MaxTokens <= 0 || rm.MaxTokens > cap {
		rm.MaxTokens = cap
	}
}

func newCopilotProvider(cfg *config.Config, modelRef string) (llm.Provider, error) {
	ref := strings.TrimSpace(modelRef)
	if ref == "" {
		ref = strings.TrimSpace(cfg.Agent.Model)
	}
	rm, err := cfg.ResolveLLM(ref)
	if err != nil {
		return nil, err
	}
	cap := cfg.Memory.CopilotMaxTokens
	clampProviderMax(rm, cap)
	return llm.NewProvider(rm.ProviderType, rm.Model, rm.APIKey, rm.BaseURL, rm.MaxTokens, rm.Temperature)
}

// RunRecall runs the recall sub-agent and returns text for the main prompt memory section.
// When opts is non-nil, OnStream receives streamed model text and reasoning deltas (recall phase only).
// ReadPaths lists scope:relative paths successfully read via coddy_memory_read (deduped, order preserved).
// Returns final recall text, wall-clock duration in milliseconds, read paths, and error.
func RunRecall(ctx context.Context, log *slog.Logger, cfg *config.Config, cwd, userQuery, modelRef string, opts *RunRecallOptions) (string, int64, []string, error) {
	var readPaths []string
	if !cfg.Memory.Enabled {
		return "", 0, nil, nil
	}
	store, err := NewStore(&cfg.Memory, cfg.Paths, cwd)
	if err != nil {
		return "", 0, nil, err
	}
	if !store.HasAnyFiles() {
		return "", 0, nil, nil
	}
	prov, err := newCopilotProvider(cfg, modelRef)
	if err != nil {
		return "", 0, nil, err
	}
	tools := RecallToolDefinitions()
	msgs := []llm.Message{
		{Role: llm.RoleSystem, Content: recallSystem},
		{Role: llm.RoleUser, Content: "User message for this turn:\n" + userQuery},
	}
	max := cfg.Memory.RecallMaxTurns
	recallStarted := timeNowMs()
	if opts != nil && opts.OnPhaseStart != nil {
		opts.OnPhaseStart()
	}
	for step := 0; step < max; step++ {
		if ctx.Err() != nil {
			return "", timeNowMs() - recallStarted, readPaths, ctx.Err()
		}
		var resp *llm.Response
		if opts != nil && opts.OnStream != nil {
			resp, err = runRecallStreamRound(ctx, prov, msgs, tools, opts.OnStream)
		} else {
			resp, err = prov.Complete(ctx, msgs, tools)
		}
		if err != nil {
			return "", timeNowMs() - recallStarted, readPaths, err
		}
		if len(resp.ToolCalls) == 0 {
			out := strings.TrimSpace(resp.Content)
			dur := timeNowMs() - recallStarted
			if out == "" {
				return "", dur, readPaths, nil
			}
			return out, dur, readPaths, nil
		}
		msgs = append(msgs, llm.Message{Role: llm.RoleAssistant, Content: resp.Content, ToolCalls: resp.ToolCalls})
		for _, tc := range resp.ToolCalls {
			res, ex := execTool(store, &cfg.Memory, tc.Name, tc.InputJSON)
			if tc.Name == "coddy_memory_read" && ex == nil {
				var ra struct {
					Path string `json:"path"`
				}
				if uerr := json.Unmarshal([]byte(tc.InputJSON), &ra); uerr == nil {
					readPaths = appendRecallReadPath(readPaths, ra.Path)
				}
			}
			if ex != nil {
				res = "error: " + ex.Error()
			}
			msgs = append(msgs, llm.Message{Role: llm.RoleTool, ToolCallID: tc.ID, Content: res})
		}
	}
	if log != nil {
		log.Warn("memory recall exceeded max turns")
	}
	dur := timeNowMs() - recallStarted
	return "", dur, readPaths, nil
}

// RunRecallOptions configures optional streaming hooks for recall.
type RunRecallOptions struct {
	// OnStream receives text or reasoning deltas from the model (not tool JSON).
	OnStream func(kind StreamKind, delta string)
	// OnPhaseStart is invoked once before the first LLM call (for wall-clock UI).
	OnPhaseStart func()
}

// StreamKind discriminates streamed memory copilot content.
type StreamKind string

const (
	StreamKindText      StreamKind = "text"
	StreamKindReasoning StreamKind = "reasoning"
)

func timeNowMs() int64 { return time.Now().UnixMilli() }

func appendRecallReadPath(slice []string, p string) []string {
	p = strings.TrimSpace(p)
	if p == "" {
		return slice
	}
	for _, x := range slice {
		if x == p {
			return slice
		}
	}
	return append(slice, p)
}

func runRecallStreamRound(ctx context.Context, prov llm.Provider, msgs []llm.Message, tools []llm.ToolDefinition, onStream func(kind StreamKind, delta string)) (*llm.Response, error) {
	resp, err := prov.Stream(ctx, msgs, tools, func(ch llm.StreamChunk) {
		if onStream == nil {
			return
		}
		if ch.TextDelta != "" {
			onStream(StreamKindText, ch.TextDelta)
		}
		if ch.ReasoningDelta != "" {
			onStream(StreamKindReasoning, ch.ReasoningDelta)
		}
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

type judgeResult struct {
	Save   bool   `json:"save"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	Scope  string `json:"scope"`
	Reason string `json:"reason"`
}

// PersistOutcome is the structured result after the memory judge runs.
type PersistOutcome struct {
	Saved        bool
	Scope        string
	RelativePath string // path under memory root written (scope:rel) when Saved
	Title        string
	Body         string // markdown body written when Saved (trimmed, for UI)
	Reason       string
	RawJudge     string // full aggregated model output (for UI trace)
}

// RunPersistOptions configures optional hooks for persist (judge) streaming.
type RunPersistOptions struct {
	OnPhaseStart func()
	OnStream     func(kind StreamKind, delta string)
}

// RunPersist optionally writes a new memory file after an LLM-as-judge step.
// Returns structured outcome, wall-clock milliseconds for the judge LLM call, and error when the LLM fails.
func RunPersist(ctx context.Context, log *slog.Logger, cfg *config.Config, cwd, modelRef, userQuery, assistantReply string, opts *RunPersistOptions) (PersistOutcome, int64, error) {
	out := PersistOutcome{}
	if !cfg.Memory.Enabled {
		return out, 0, nil
	}
	assistantReply = strings.TrimSpace(assistantReply)
	if assistantReply == "" {
		return out, 0, nil
	}
	prov, err := newCopilotProvider(cfg, modelRef)
	if err != nil {
		return out, 0, err
	}
	userPayload := fmt.Sprintf("User:\n%s\n\nAssistant:\n%s\n", userQuery, assistantReply)
	msgs := []llm.Message{
		{Role: llm.RoleSystem, Content: judgeSystem},
		{Role: llm.RoleUser, Content: userPayload},
	}
	persistStarted := timeNowMs()
	if opts != nil && opts.OnPhaseStart != nil {
		opts.OnPhaseStart()
	}
	var resp *llm.Response
	if opts != nil && opts.OnStream != nil {
		resp, err = runJudgeStreamRound(ctx, prov, msgs, opts.OnStream)
	} else {
		resp, err = prov.Complete(ctx, msgs, nil)
	}
	if err != nil {
		return out, timeNowMs() - persistStarted, err
	}
	out.RawJudge = strings.TrimSpace(resp.Content)
	raw := extractJSONObject(resp.Content)
	var jr judgeResult
	dur := timeNowMs() - persistStarted
	if err := json.Unmarshal([]byte(raw), &jr); err != nil {
		if log != nil {
			log.Warn("memory judge parse failed", "error", err)
		}
		return out, dur, nil
	}
	if !jr.Save {
		return out, dur, nil
	}
	scope := strings.ToLower(strings.TrimSpace(jr.Scope))
	if scope != "global" && scope != "project" {
		scope = "global"
	}
	store, err := NewStore(&cfg.Memory, cfg.Paths, cwd)
	if err != nil {
		return out, dur, err
	}
	body := strings.TrimSpace(jr.Body)
	if len(body) > 900 {
		body = body[:900] + "\n..."
	}
	relPath, err := store.Write(scope, jr.Title, body)
	if err != nil {
		return out, dur, err
	}
	out.Saved = true
	out.Scope = scope
	out.RelativePath = relPath
	out.Title = jr.Title
	out.Body = body
	out.Reason = jr.Reason
	if log != nil {
		log.Info("memory saved", "scope", scope, "reason", jr.Reason)
	}
	return out, dur, nil
}

func runJudgeStreamRound(ctx context.Context, prov llm.Provider, msgs []llm.Message, onStream func(kind StreamKind, delta string)) (*llm.Response, error) {
	return prov.Stream(ctx, msgs, nil, func(ch llm.StreamChunk) {
		if onStream == nil {
			return
		}
		if ch.TextDelta != "" {
			onStream(StreamKindText, ch.TextDelta)
		}
		if ch.ReasoningDelta != "" {
			onStream(StreamKindReasoning, ch.ReasoningDelta)
		}
	})
}

func extractJSONObject(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "```"); i >= 0 {
		s = strings.TrimSpace(s[i+3:])
		if j := strings.Index(s, "\n"); j >= 0 && strings.HasPrefix(strings.TrimSpace(s[:j]), "json") {
			s = strings.TrimSpace(s[j+1:])
		}
		if k := strings.Index(s, "```"); k >= 0 {
			s = strings.TrimSpace(s[:k])
		}
	}
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return "{}"
	}
	return s[start : end+1]
}
