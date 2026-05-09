package memory

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/EvilFreelancer/coddy-agent/internal/config"
	"github.com/EvilFreelancer/coddy-agent/internal/llm"
)

// beforeTurnSystem is the single memory copilot pass that runs before the main agent each user turn.
// The model must choose one primary mode: RECALL (read-only tools) or PERSIST (may write), not both in one turn.
const beforeTurnSystem = `You are the memory copilot for a coding agent. You run exactly ONCE per user message, BEFORE the main assistant model runs. You never speak to the end user directly.

You have all memory tools. Each user turn you must follow exactly ONE mode:

MODE RECALL - load context from disk for the main assistant
- Use ONLY coddy_memory_search, coddy_memory_list, coddy_memory_read. Do NOT call coddy_memory_mkdir, coddy_memory_save, or coddy_memory_delete.
- Choose RECALL when the user wants help that benefits from prior saved facts, project context, or preferences, or when they did not clearly ask only to store or forget something.
- Default to RECALL when unsure.
- Search uses word overlap between your query and file paths plus bodies. Notes may be written in a different language than the user's message. If the user asks how you are called, your name, identity, or similar (any language), run coddy_memory_search with scope "both" using (1) their wording and (2) a second query with English keywords such as: assistant name identity preferences how to address you call you.
- If searches still show nothing relevant, try coddy_memory_list on global: and project: then coddy_memory_read plausible paths (for example assistant or preferences folders).

MODE PERSIST - update long-term storage based on this user message alone (you do not have the assistant reply yet)
- You MAY use coddy_memory_search, coddy_memory_list, coddy_memory_read, coddy_memory_mkdir, coddy_memory_save, coddy_memory_delete.
- Choose PERSIST when the user explicitly asks to remember, save, store for later, forget, delete a saved fact, or rename their preference; or when the clear primary intent is writing durable notes from what they said.
- Before saving: read existing notes to avoid duplicates. Use coddy_memory_mkdir before first save under a new folder branch.

Opt-out: if the user clearly forbids consulting saved notes for this message, skip RECALL tools and reply with one short line; no paths or tool jargon.

Paths use scope:relative (global:... or project:...). Global root defaults to $CODDY_HOME/memory; project root is cwd/memory.

RECALL finishing text (plain only, no tools): structure with "Already on disk" and optional "Not in notes" bullets. Write only facts the main assistant should apply - no memory paths, no scope prefixes (global:/project:), no file names, extensions, or citations like "see ...md". Do not name where a fact was stored. If nothing matched after search/read, reply exactly: (no memory hits)

PERSIST finishing text (plain only, no tools): briefly what you verified on disk and what you saved, skipped, or deleted.

Secrets: never store API keys, tokens, passwords, or one-off credentials in coddy_memory_save body.

When finished with tools in your chosen mode, respond with plain text only (no tool calls).`

// BeforeTurnOutcome is the result of the single pre-main-agent memory copilot pass.
type BeforeTurnOutcome struct {
	Mode        string // "recall" or "persist"
	ContextText string // merged into Session memory for the main agent
	ReadPaths   []string
	Persist     PersistOutcome
}

// PersistOutcome is structured data when coddy_memory_save completed in the same pass.
type PersistOutcome struct {
	Saved        bool
	Scope        string
	RelativePath string
	Title        string
	Body         string
	Reason       string
	RawFinalText string
}

// RunBeforeTurnOptions configures streaming hooks for the unified memory pass.
type RunBeforeTurnOptions struct {
	OnPhaseStart func()
	OnStream     func(kind StreamKind, delta string)
}

// StreamKind discriminates streamed memory copilot content.
type StreamKind string

const (
	StreamKindText      StreamKind = "text"
	StreamKindReasoning StreamKind = "reasoning"
)

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

type saveCapture struct {
	scopeLabel   string
	relativePath string
	title        string
	body         string
}

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

func runCopilotStreamRound(ctx context.Context, prov llm.Provider, msgs []llm.Message, tools []llm.ToolDefinition, onStream func(kind StreamKind, delta string)) (*llm.Response, error) {
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

// RunBeforeTurn runs the unified memory copilot once before the main agent: either recall or persist, never both.
func RunBeforeTurn(ctx context.Context, log *slog.Logger, cfg *config.Config, cwd, userQuery, modelRef string, opts *RunBeforeTurnOptions) (BeforeTurnOutcome, int64, error) {
	out := BeforeTurnOutcome{Mode: "recall"}
	if !cfg.Memory.Enabled {
		return out, 0, nil
	}
	store, err := NewStore(&cfg.Memory, cfg.Paths, cwd)
	if err != nil {
		return out, 0, err
	}
	prov, err := newCopilotProvider(cfg, modelRef)
	if err != nil {
		return out, 0, err
	}
	tools := PersistToolDefinitions()
	msgs := []llm.Message{
		{Role: llm.RoleSystem, Content: beforeTurnSystem},
		{Role: llm.RoleUser, Content: "User message for this turn:\n" + userQuery},
	}
	maxTurns := cfg.Memory.PersistMaxTurns
	if cfg.Memory.RecallMaxTurns > maxTurns {
		maxTurns = cfg.Memory.RecallMaxTurns
	}
	started := timeNowMs()
	if opts != nil && opts.OnPhaseStart != nil {
		opts.OnPhaseStart()
	}

	var readPaths []string
	var lastSave *saveCapture
	mutationSeen := false

	for step := 0; step < maxTurns; step++ {
		if ctx.Err() != nil {
			return out, timeNowMs() - started, ctx.Err()
		}
		var resp *llm.Response
		var err error
		if opts != nil && opts.OnStream != nil {
			resp, err = runCopilotStreamRound(ctx, prov, msgs, tools, opts.OnStream)
		} else {
			resp, err = prov.Complete(ctx, msgs, tools)
		}
		if err != nil {
			return out, timeNowMs() - started, err
		}
		if len(resp.ToolCalls) == 0 {
			finalText := strings.TrimSpace(resp.Content)
			out.ContextText = finalText
			out.Persist.RawFinalText = finalText
			out.Persist.Reason = finalText
			if lastSave != nil {
				out.Persist.Saved = true
				out.Persist.Scope = lastSave.scopeLabel
				out.Persist.RelativePath = lastSave.relativePath
				out.Persist.Title = lastSave.title
				out.Persist.Body = lastSave.body
				if log != nil {
					log.Info("memory saved", "path", lastSave.relativePath)
				}
			}
			if mutationSeen || out.Persist.Saved {
				out.Mode = "persist"
			}
			return out, timeNowMs() - started, nil
		}
		msgs = append(msgs, llm.Message{Role: llm.RoleAssistant, Content: resp.Content, ToolCalls: resp.ToolCalls})
		for _, tc := range resp.ToolCalls {
			switch tc.Name {
			case "coddy_memory_save", "coddy_memory_mkdir", "coddy_memory_delete":
				mutationSeen = true
			}
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
			} else if tc.Name == "coddy_memory_save" {
				var args struct {
					Title        string `json:"title"`
					Body         string `json:"body"`
					Scope        string `json:"scope"`
					RelativePath string `json:"relative_path"`
				}
				if uerr := json.Unmarshal([]byte(tc.InputJSON), &args); uerr == nil {
					written := strings.TrimPrefix(strings.TrimSpace(res), "saved as")
					written = strings.TrimSpace(written)
					body := strings.TrimSpace(args.Body)
					if len(body) > 900 {
						body = body[:900] + "\n..."
					}
					lastSave = &saveCapture{
						scopeLabel:   strings.ToLower(strings.TrimSpace(args.Scope)),
						relativePath: written,
						title:        strings.TrimSpace(args.Title),
						body:         body,
					}
				}
			}
			msgs = append(msgs, llm.Message{Role: llm.RoleTool, ToolCallID: tc.ID, Content: res})
		}
	}
	if log != nil {
		log.Warn("memory before-turn exceeded max turns")
	}
	dur := timeNowMs() - started
	if lastSave != nil {
		out.Persist.Saved = true
		out.Persist.Scope = lastSave.scopeLabel
		out.Persist.RelativePath = lastSave.relativePath
		out.Persist.Title = lastSave.title
		out.Persist.Body = lastSave.body
		out.Persist.Reason = "memory copilot stopped at max turns after a save"
		out.Persist.RawFinalText = out.Persist.Reason
		out.ContextText = out.Persist.Reason
		out.Mode = "persist"
	}
	return out, dur, nil
}
