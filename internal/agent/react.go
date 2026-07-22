// Package agent implements the ReAct (Reasoning + Acting) loop for a session turn.
// System prompts are rendered via internal/prompts (embedded templates or prompts.dir).
package agent

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/ideenv"
	"github.com/hijera/foxxycode-agent/internal/ideterm"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/mcp"
	"github.com/hijera/foxxycode-agent/internal/permission"
	"github.com/hijera/foxxycode-agent/internal/plans"
	"github.com/hijera/foxxycode-agent/internal/platform"
	"github.com/hijera/foxxycode-agent/internal/session"
	"github.com/hijera/foxxycode-agent/internal/skills"
	"github.com/hijera/foxxycode-agent/internal/tools"
)

// SessionState is the interface Agent needs from a session.
// It is implemented by session.State without requiring a direct import.
type SessionState interface {
	GetID() string
	GetCWD() string
	GetMode() string
	SetMode(mode string)
	EffectiveModelID(cfg *config.Config) string
	EffectiveReasoning(cfg *config.Config) string
	AddMessage(msg llm.Message)
	GetMessages() []llm.Message
	ReplaceMessagesAndPersist(msgs []llm.Message)
	InsertCompactionSummary(idx int, msg llm.Message)
	GetMCPClients() []*mcp.Client
	GetSkills() []*skills.Skill
	GetAgentMemory() string
	GetMemoryCopilotBlock() string
	SetMemoryCopilotBlock(text string)
	ClearMemoryCopilotBlock()
	GetPlan() []acp.PlanEntry
	SetPlan([]acp.PlanEntry)
	GetPersistedSessionDir() string
	AppendPlanDocument(plans.Document)
	DiscardedPlanSlugs() []string
	TakePendingPlanContext() string
	TakePendingImageParts() []llm.ImagePart
	GetPermissionMode() string
	IsUserCancelledTurn() bool
	GetTitlePinned() string
	GetTitleAuto() string
	SetTitleAuto(text string)
}

// Agent runs the ReAct loop for a single session turn.
type Agent struct {
	cfg             *config.Config
	state           SessionState
	server          acp.UpdateSender
	log             *slog.Logger
	registry        *tools.Registry
	environment     platform.Environment
	providerFactory func(llm.ProviderInput) (llm.Provider, error)

	imgMu             sync.Mutex
	pendingToolImages []llm.ImagePart
}

// addToolImage buffers an image produced by a tool (e.g. a browser screenshot) so the
// ReAct loop can inject it as a user-role vision block for the next model turn.
func (a *Agent) addToolImage(part llm.ImagePart) {
	a.imgMu.Lock()
	defer a.imgMu.Unlock()
	a.pendingToolImages = append(a.pendingToolImages, part)
}

// takeToolImages returns and clears the buffered tool images.
func (a *Agent) takeToolImages() []llm.ImagePart {
	a.imgMu.Lock()
	defer a.imgMu.Unlock()
	if len(a.pendingToolImages) == 0 {
		return nil
	}
	out := a.pendingToolImages
	a.pendingToolImages = nil
	return out
}

// browserVisionNote accompanies screenshots injected after browser tool calls so the
// model knows the attached images show the current page state.
const browserVisionNote = "The image(s) below are screenshot(s) captured by the browser tool, showing the current page. Inspect them before deciding the next action."

// NewAgent creates an Agent for a prompt turn.
func NewAgent(cfg *config.Config, state SessionState, server acp.UpdateSender, log *slog.Logger) *Agent {
	environment := platform.CurrentEnvironment()
	return &Agent{
		cfg:             cfg,
		state:           state,
		server:          server,
		log:             log,
		registry:        tools.NewRegistryForEnvironment(cfg, environment),
		environment:     environment,
		providerFactory: llm.NewProvider,
	}
}

// SetProviderFactory replaces the LLM provider factory used by subsequent turns.
func (a *Agent) SetProviderFactory(mk func(llm.ProviderInput) (llm.Provider, error)) {
	if a == nil || mk == nil {
		return
	}
	a.providerFactory = mk
}

// Run executes the ReAct loop and returns the stop reason.
func (a *Agent) Run(ctx context.Context, prompt []acp.ContentBlock) (string, error) {
	mode := a.state.GetMode()

	// Build the user message from prompt content blocks.
	a.state.ClearMemoryCopilotBlock()
	userText := contentBlocksToText(prompt)

	// The built-in /compact command compacts history instead of running the ReAct
	// loop. runCompactCommand persists the command text itself (so it shows in the
	// transcript like any other message), and under the opencode engine returns a
	// short notice instead of compacting.
	if instructions, ok := parseCompactCommand(userText); ok {
		return a.runCompactCommand(ctx, instructions, userText)
	}
	// The built-in /plugin command manages skill plugins and marketplaces
	// deterministically, without an LLM turn; the command text is persisted too.
	if args, ok := parsePluginCommand(userText); ok {
		return a.runPluginCommand(ctx, args, userText)
	}

	imageParts := a.state.TakePendingImageParts()
	messageContent := userText
	if note := filePathsNote(imageParts); note != "" {
		messageContent = messageContent + "\n\n" + note
	}
	if note := ideEnvNote(a.state.GetCWD()); note != "" {
		messageContent = messageContent + "\n\n" + note
	}
	if note := terminalEnvNote(); note != "" {
		messageContent = messageContent + "\n\n" + note
	}
	if note := terminalMentionNote(userText); note != "" {
		messageContent = messageContent + "\n\n" + note
	}
	a.state.AddMessage(llm.Message{
		Role:       llm.RoleUser,
		Content:    messageContent,
		ImageParts: imageParts,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
	})
	a.runMemoryBeforeTurn(ctx, userText)

	// Collect context files from the prompt for skill filtering.
	contextFiles := extractContextFiles(prompt)

	// Load skills applicable to this context.
	activeSkills := FilterSkillsForContext(a.state.GetSkills(), contextFiles)

	toolSet := ToolSetForMode(mode)
	toolDefs := FilterToolDefinitions(a.registry.AllToolDefinitions(), toolSet)
	if ModeAllowsMCPTools(mode) {
		for _, mcpClient := range a.state.GetMCPClients() {
			for _, t := range mcpClient.Tools() {
				toolDefs = append(toolDefs, t.ToLLMToolDefinition(mcpClient.Name()))
			}
		}
	}

	// Get or create LLM provider.
	provider, err := a.getProvider(mode)
	if err != nil {
		return string(acp.StopReasonRefused), fmt.Errorf("no LLM configured: %w", err)
	}

	// Restore existing plan via session/update if one was set by foxxycode todo tools in a previous turn.
	if existing := a.state.GetPlan(); len(existing) > 0 {
		if err := a.sendPlan(a.state.GetID(), existing); err != nil {
			a.log.Warn("failed to restore plan", "error", err)
		}
	}

	// Build the full message list starting with system prompt (refreshed each ReAct turn).
	messages := a.buildMessages(a.buildSystemPrompt(mode, activeSkills, toolDefs, userText, contextFiles))

	// Coddy engine: buildSystemPrompt refreshed the context breakdown, so compact
	// before the first LLM call when the estimate crossed the auto-compaction
	// threshold, then rebuild the payload from the windowed history.
	if a.cfg.Compaction.EngineIsCoddy() && a.maybeAutoCompact(ctx) {
		messages = a.buildMessages(a.buildSystemPrompt(mode, activeSkills, toolDefs, userText, contextFiles))
	}

	maxTurns := a.cfg.Agent.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 30
	}

	sd := strings.TrimSpace(a.state.GetPersistedSessionDir())

	toolEnv := &tools.Env{
		CWD:              a.state.GetCWD(),
		PermissionMode:   effectivePermMode(a.state, a.cfg),
		CommandAllowlist: a.cfg.Tools.CommandAllowlist,
		SessionID:        a.state.GetID(),
		SessionDir:       sd,
		ArchiveActiveMarkdown: func() error {
			if sd == "" {
				return nil
			}
			return session.ArchiveActiveTodo(sd)
		},
		WriteArchivedPlanMarkdown: func(md string) (string, error) {
			if sd == "" {
				return "", nil
			}
			return session.WritePlanArchivedMarkdown(sd, md)
		},
		Sender:  a.server,
		GetPlan: a.state.GetPlan,
		SetPlan: a.state.SetPlan,
		SetSessionMode: func(mode string) error {
			a.state.SetMode(strings.TrimSpace(mode))
			return nil
		},
		PersistPlanDocument: func(doc plans.Document) {
			a.state.AppendPlanDocument(doc)
		},
		SSHConnectTimeout: a.cfg.Tools.SSHConnectTimeout,
		LoadSkillBody:     a.loadSkillBody,
	}
	toolEnv.SendDesignPlanUpdate = func(doc plans.Document) {
		tools.SendDesignPlanUpdate(toolEnv, doc)
	}
	toolEnv.AddToolImage = func(dataURL, filePath, name string) {
		a.addToolImage(llm.ImagePart{DataURL: dataURL, FilePath: filePath, Name: name})
	}
	a.wireFileEditHook(toolEnv)

	return a.runReActLoop(ctx, mode, messages, toolDefs, provider, toolEnv, sd, userText, contextFiles, activeSkills, maxTurns, true)
}

// wireFileEditHook connects Env.OnFileEdit to the update sender so filesystem writes are
// surfaced as acp.FileEditUpdate events (consumed by native editor clients for diffs).
func (a *Agent) wireFileEditHook(env *tools.Env) {
	if a.server == nil {
		return
	}
	env.OnFileEdit = func(toolName, absPath string, before, after []byte) {
		_ = a.server.SendSessionUpdate(a.state.GetID(), acp.FileEditUpdate{
			SessionUpdate: acp.UpdateTypeFileEdit,
			ToolCallID:    env.ToolCallID,
			ToolName:      toolName,
			Path:          absPath,
			Before:        string(before),
			After:         string(after),
		})
	}
}

// maxEmptyAssistantContinuations bounds how many times the ReAct loop re-prompts a model
// that ended a turn with no visible answer and no tool call (only reasoning, or nothing).
// It guards against dead-ending the conversation on a thinking-only bubble — seen with
// gpt-oss / harmony endpoints that leak a tool call into the reasoning channel — while
// preventing an unbounded empty-turn loop.
const maxEmptyAssistantContinuations = 2

// emptyAssistantContinuationNudge is injected into the LLM-facing message slice (never
// persisted to the transcript) to prompt the model to produce its answer or a tool call
// after an empty turn.
const emptyAssistantContinuationNudge = "Your previous message had no answer text and no tool call. Continue now: call the appropriate tool to act, or write your reply to the user."

func (a *Agent) runReActLoop(
	ctx context.Context,
	mode string,
	messages []llm.Message,
	toolDefs []llm.ToolDefinition,
	provider llm.Provider,
	toolEnv *tools.Env,
	sd, userText string,
	contextFiles []string,
	activeSkills []*skills.Skill,
	maxTurns int,
	allowTitleGen bool,
) (string, error) {
	var totalInputTokens, totalOutputTokens int
	var turnIndex int
	var lastStatsWrite time.Time
	var emptyContinuations int
	var lastInputTokens int

	for turn := 0; turn < maxTurns; turn++ {
		if ctx.Err() != nil {
			return string(acp.StopReasonCancelled), nil
		}

		// System prompt is rebuilt every turn so conditional sections (e.g. todo checklist) match
		// state after foxxycode_todo_* tools in the same user turn.
		if len(messages) > 0 && messages[0].Role == llm.RoleSystem {
			messages[0].Content = a.buildSystemPrompt(mode, activeSkills, toolDefs, userText, contextFiles)
		}

		// Auto-compaction: when the conversation approaches the context window, summarize older
		// turns and rebuild the payload from the rewritten history. Non-fatal on error. The coddy
		// engine re-checks between turns (the first check ran before the loop); the opencode engine
		// checks every turn against the provider's real input-token count.
		if a.cfg.Compaction.EngineIsCoddy() {
			if turn > 0 && a.maybeAutoCompact(ctx) {
				messages = a.buildMessages(a.buildSystemPrompt(mode, activeSkills, toolDefs, userText, contextFiles))
			}
		} else if did, err := a.maybeCompact(ctx, provider, lastInputTokens); err != nil {
			if a.log != nil {
				a.log.Warn("context compaction failed", "err", err)
			}
		} else if did {
			messages = a.buildMessages(a.buildSystemPrompt(mode, activeSkills, toolDefs, userText, contextFiles))
		}

		// Call LLM and stream response.
		var response *llm.Response
		var streamErr error
		var reasoningBuf strings.Builder

		reasonClockStart := time.Time{}
		reasonClockEnd := time.Time{}
		maybeMarkReasonEnd := func(now time.Time) {
			if reasonClockStart.IsZero() || !reasonClockEnd.IsZero() {
				return
			}
			if strings.TrimSpace(reasoningBuf.String()) == "" {
				return
			}
			reasonClockEnd = now
		}

		sessionID := a.state.GetID()

		// Cancel the stream if no tokens arrive within 90 s (API hang guard).
		const firstTokenTimeout = 90 * time.Second
		streamCtx, streamCancel := context.WithCancel(ctx)
		firstTokenTimer := time.AfterFunc(firstTokenTimeout, streamCancel)

		emitReason := func(d string, now time.Time) {
			firstTokenTimer.Stop()
			reasoningBuf.WriteString(d)
			if reasonClockStart.IsZero() {
				reasonClockStart = now
			}
			_ = a.server.SendSessionUpdate(sessionID, acp.MessageChunkUpdate{
				SessionUpdate: acp.UpdateTypeAgentMessageChunk,
				Content:       acp.ContentBlock{Type: acp.ContentTypeReasoning, Text: d},
			})
		}
		emitText := func(delta string, now time.Time, markReasonEnd bool) {
			firstTokenTimer.Stop()
			if markReasonEnd && strings.TrimSpace(delta) != "" {
				maybeMarkReasonEnd(now)
			}
			_ = a.server.SendSessionUpdate(sessionID, acp.MessageChunkUpdate{
				SessionUpdate: acp.UpdateTypeAgentMessageChunk,
				Content:       acp.ContentBlock{Type: acp.ContentTypeText, Text: delta},
			})
		}

		response, streamErr = provider.Stream(streamCtx, messages, toolDefs, func(chunk llm.StreamChunk) {
			if streamCtx.Err() != nil {
				return
			}
			now := time.Now()
			if chunk.ReasoningDelta != "" {
				emitReason(chunk.ReasoningDelta, now)
			}
			if chunk.TextDelta != "" && strings.TrimSpace(chunk.TextDelta) != "" {
				emitText(chunk.TextDelta, now, true)
			} else if chunk.TextDelta != "" {
				emitText(chunk.TextDelta, now, false)
			}
			if chunk.ToolCall != nil && chunk.ToolCall.Name != "" {
				maybeMarkReasonEnd(now)
				if st := sessionStatePtr(a.state); st != nil {
					if sd := strings.TrimSpace(st.GetPersistedSessionDir()); sd != "" && strings.TrimSpace(chunk.ToolCall.ID) != "" {
						_ = session.WriteToolCallMeta(sd, chunk.ToolCall.ID, session.ToolCallMeta{
							ToolCallID: strings.TrimSpace(chunk.ToolCall.ID),
							Name:       chunk.ToolCall.Name,
							Kind:       toolKind(chunk.ToolCall.Name),
							Status:     "pending",
						})
					}
				}
				_ = a.server.SendSessionUpdate(sessionID, acp.ToolCallUpdate{
					SessionUpdate: acp.UpdateTypeToolCall,
					ToolCallID:    chunk.ToolCall.ID,
					Title:         chunk.ToolCall.Name, // plain name, no "Calling: " prefix
					Kind:          toolKind(chunk.ToolCall.Name),
					Status:        "pending",
				})
				firstTokenTimer.Stop()
			}
		})
		firstTokenTimer.Stop()
		streamCancel()

		// If the stream was cancelled by the first-token timer (no output produced, no user cancel),
		// surface a timeout error instead of a silent failure.
		if streamErr != nil && errors.Is(streamErr, context.Canceled) && !a.state.IsUserCancelledTurn() {
			hasAnyOutput := response != nil && (strings.TrimSpace(response.Content) != "" ||
				len(response.ToolCalls) > 0 || strings.TrimSpace(reasoningBuf.String()) != "")
			if !hasAnyOutput {
				return string(acp.StopReasonRefused), fmt.Errorf("model did not respond (no output within %v)", firstTokenTimeout)
			}
		}

		if streamErr != nil {
			if errors.Is(streamErr, context.Canceled) && response != nil {
				reasonTrim := strings.TrimSpace(reasoningBuf.String())
				hasText := strings.TrimSpace(response.Content) != ""
				hasTools := len(response.ToolCalls) > 0
				if hasText || hasTools || reasonTrim != "" {
					var reasoningMs int64
					if reasonTrim != "" && !reasonClockStart.IsZero() {
						end := reasonClockEnd
						if end.IsZero() {
							end = time.Now()
						}
						d := end.Sub(reasonClockStart)
						if d < 0 {
							d = 0
						}
						reasoningMs = d.Milliseconds()
					}
					reasonStore, reasonSig := reasoningForStorage(reasonTrim, reasoningBuf.String(), response)
					assistantMsg := llm.Message{
						Role:                llm.RoleAssistant,
						Content:             response.Content,
						Reasoning:           reasonStore,
						ReasoningSignature:  reasonSig,
						ToolCalls:           response.ToolCalls,
						ReasoningDurationMs: reasoningMs,
						Model:               a.state.EffectiveModelID(a.cfg),
						CreatedAt:           time.Now().UTC().Format(time.RFC3339),
					}
					a.state.AddMessage(assistantMsg)
				}
			}
			if errors.Is(streamErr, context.Canceled) {
				// If output was already streamed, treat as a clean user-stop regardless.
				hasAnyOutput := response != nil && (strings.TrimSpace(response.Content) != "" ||
					len(response.ToolCalls) > 0 || strings.TrimSpace(reasoningBuf.String()) != "")
				if hasAnyOutput || a.state.IsUserCancelledTurn() {
					return string(acp.StopReasonCancelled), nil
				}
				// Stream was interrupted before producing any output and the user did not stop it —
				// surface an error so the UI can show feedback instead of silently completing.
				return string(acp.StopReasonRefused), fmt.Errorf("generation was interrupted before producing a response")
			}
			if ctx.Err() != nil {
				// Context cancelled for non-context-Canceled stream error: still propagate the real error.
				return string(acp.StopReasonRefused), fmt.Errorf("LLM error: %w", streamErr)
			}
			return string(acp.StopReasonRefused), fmt.Errorf("LLM error: %w", streamErr)
		}

		// Accumulate and broadcast token usage after each LLM call.
		totalInputTokens += response.InputTokens
		totalOutputTokens += response.OutputTokens
		lastInputTokens = response.InputTokens
		_ = a.server.SendSessionUpdate(sessionID, acp.TokenUsageUpdate{
			SessionUpdate: acp.UpdateTypeTokenUsage,
			InputTokens:   response.InputTokens,
			OutputTokens:  response.OutputTokens,
			TotalTokens:   totalInputTokens + totalOutputTokens,
		})

		if sd != "" {
			now := time.Now().UTC()
			if lastStatsWrite.IsZero() || now.Sub(lastStatsWrite) > 750*time.Millisecond {
				lastStatsWrite = now
				stats := session.SessionStats{
					Version:   1,
					UpdatedAt: now.Format(time.RFC3339),
					TokenUsageTotal: session.TokenUsageTotals{
						InputTokens:  totalInputTokens,
						OutputTokens: totalOutputTokens,
						TotalTokens:  totalInputTokens + totalOutputTokens,
					},
					TokenUsageByTurn: []session.TokenUsageTurn{{
						TurnIndex:    turnIndex,
						InputTokens:  response.InputTokens,
						OutputTokens: response.OutputTokens,
						TotalTokens:  totalInputTokens + totalOutputTokens,
						Timestamp:    now.Format(time.RFC3339),
					}},
				}
				if rs, ok := a.state.(rulesState); ok {
					if b := rs.GetLastContextBreakdown(); b != nil {
						cp := *b
						stats.ContextBreakdown = &cp
					}
				}
				_ = session.WriteSessionStats(sd, stats)
			}
		}
		turnIndex++

		reasonTrim := strings.TrimSpace(reasoningBuf.String())
		var reasoningMs int64
		if reasonTrim != "" && !reasonClockStart.IsZero() {
			end := reasonClockEnd
			if end.IsZero() {
				end = time.Now()
			}
			d := end.Sub(reasonClockStart)
			if d < 0 {
				d = 0
			}
			reasoningMs = d.Milliseconds()
		}

		// Append assistant message to history.
		reasonStore, reasonSig := reasoningForStorage(reasonTrim, reasoningBuf.String(), response)
		assistantMsg := llm.Message{
			Role:                llm.RoleAssistant,
			Content:             response.Content,
			Reasoning:           reasonStore,
			ReasoningSignature:  reasonSig,
			ToolCalls:           response.ToolCalls,
			ReasoningDurationMs: reasoningMs,
			Model:               a.state.EffectiveModelID(a.cfg),
			CreatedAt:           time.Now().UTC().Format(time.RFC3339),
		}
		messages = append(messages, assistantMsg)
		a.state.AddMessage(assistantMsg)

		// After the first assistant response, generate a short session title off the hot path.
		// Internal guards make this a no-op when the title is pinned or already generated, and it
		// uses a detached context so it survives the turn ending. Non-fatal by design. Only the
		// fresh-prompt path titles; resume/continue turns never do.
		if allowTitleGen && turn == 0 {
			titleCtx, titleCancel := context.WithTimeout(context.Background(), 30*time.Second)
			go func() {
				defer titleCancel()
				a.maybeGenerateTitle(titleCtx, provider)
			}()
		}

		// If no tool calls, we're done — unless the model produced no visible answer at
		// all (empty content). Some models (notably gpt-oss / harmony endpoints) sometimes
		// end a turn with only internal reasoning — occasionally leaking a tool call into
		// the reasoning channel — emitting neither final content nor a tool_calls array.
		// Returning here would dead-end the conversation on a lone "thinking" bubble, so
		// re-prompt the model a bounded number of times before giving up.
		if len(response.ToolCalls) == 0 {
			if strings.TrimSpace(response.Content) == "" && emptyContinuations < maxEmptyAssistantContinuations {
				emptyContinuations++
				// LLM-facing only; never persisted to the transcript.
				messages = append(messages, llm.Message{
					Role:    llm.RoleUser,
					Content: emptyAssistantContinuationNudge,
				})
				continue
			}
			if response.StopReason == "max_tokens" {
				return string(acp.StopReasonMaxTokens), nil
			}
			return string(acp.StopReasonEndTurn), nil
		}

		// Execute all tool calls.
		for _, tc := range response.ToolCalls {
			if ctx.Err() != nil {
				return string(acp.StopReasonCancelled), nil
			}

			result, execErr := a.executeToolCall(ctx, tc, toolEnv, mode, a.state.GetID(), false)

			var toolResultMsg llm.Message
			if execErr != nil {
				toolResultMsg = llm.Message{
					Role:       llm.RoleTool,
					Content:    fmt.Sprintf("error: %v", execErr),
					ToolCallID: tc.ID,
				}
			} else {
				toolResultMsg = llm.Message{
					Role:       llm.RoleTool,
					Content:    result,
					ToolCallID: tc.ID,
				}
			}

			messages = append(messages, toolResultMsg)
			a.state.AddMessage(toolResultMsg)
		}
		// The model made progress (executed tool calls), so reset the empty-turn counter. The
		// give-up notice is for CONSECUTIVE stalls (no answer and no tool call), not for a slow
		// multi-step task that keeps acting between reasoning-only thoughts — otherwise a model
		// that alternates thinking and tool calls (gpt-oss / harmony) is abandoned mid-task.
		emptyContinuations = 0

		// Inject any screenshots produced by browser tools this round as a user-role
		// vision block so the model can see the page. This reuses the existing image
		// path (RoleUser ImageParts) rather than extending the text-only tool-result
		// contract. It is added to the live LLM message slice only (not persisted): the
		// UI renders the screenshot from the tool-call result's saved path, and keeping
		// it out of history avoids a spurious user bubble in the transcript and re-sending
		// every screenshot on later turns.
		if imgs := a.takeToolImages(); len(imgs) > 0 {
			messages = append(messages, llm.Message{
				Role:       llm.RoleUser,
				Content:    browserVisionNote,
				ImageParts: imgs,
				CreatedAt:  time.Now().UTC().Format(time.RFC3339),
			})
		}
	}

	return string(acp.StopReasonMaxTurns), nil
}

// executeToolCall runs a single tool call and reports updates to the client.
func (a *Agent) executeToolCall(ctx context.Context, tc llm.ToolCall, env *tools.Env, mode, sessionID string, skipPermission bool) (string, error) {
	env.ToolCallID = strings.TrimSpace(tc.ID)
	defer func() { env.ToolCallID = "" }()

	sessionDir := ""
	if st := sessionStatePtr(a.state); st != nil {
		sessionDir = strings.TrimSpace(st.GetPersistedSessionDir())
	}

	if sessionDir != "" && strings.TrimSpace(tc.ID) != "" {
		_ = session.MarkToolCallStarted(sessionDir, tc.ID, tc.Name, toolKind(tc.Name), "in_progress")
		_ = session.WriteToolCallArgs(sessionDir, tc.ID, tc.InputJSON)
	}

	// Mark as in_progress, include raw InputJSON so connected clients can show args.
	_ = a.server.SendSessionUpdate(sessionID, acp.ToolCallStatusUpdate{
		SessionUpdate: acp.UpdateTypeToolCallUpdate,
		ToolCallID:    tc.ID,
		Status:        "in_progress",
		Content: []acp.ToolCallResultItem{
			{Type: "content", Content: acp.ContentBlock{Type: "text", Text: tc.InputJSON}},
		},
	})

	// Check if tool requires permission.
	tool, ok := a.registry.Get(tc.Name)
	requiresPerm := ok && tool.RequiresPermission

	var sessCmdGrants, sessWriteGrants []string
	if st := sessionStatePtr(a.state); st != nil {
		sessCmdGrants = st.GetPermissionCommandGrants()
		sessWriteGrants = st.GetPermissionWriteGrants()
	}

	if tc.Name == "run_command" {
		switch env.PermissionMode {
		case config.PermModeBypass:
			requiresPerm = false
		case config.PermModeAcceptEdits:
			cmd := permission.ExtractRunCommand(tc.InputJSON)
			if permission.CommandAllowedWithSession(env, sessCmdGrants, cmd) {
				requiresPerm = false
			} else {
				requiresPerm = true
			}
		default: // ask
			cmd := permission.ExtractRunCommand(tc.InputJSON)
			if permission.CommandAllowedWithSession(env, sessCmdGrants, cmd) {
				requiresPerm = false
			} else {
				requiresPerm = true
			}
		}
	} else if filesystemWriteTool(tc.Name) {
		switch env.PermissionMode {
		case config.PermModeBypass, config.PermModeAcceptEdits:
			keys := permission.WriteGrantKeys(tc.Name, tc.InputJSON, env.CWD)
			if permission.AllWriteKeysGranted(sessWriteGrants, keys) {
				requiresPerm = false
			} else {
				requiresPerm = false // auto-approve
			}
		default: // ask
			keys := permission.WriteGrantKeys(tc.Name, tc.InputJSON, env.CWD)
			if permission.AllWriteKeysGranted(sessWriteGrants, keys) {
				requiresPerm = false
			} else {
				requiresPerm = true
			}
		}
	}

	if requiresPerm && !skipPermission {
		permResult, err := a.server.RequestPermission(ctx, acp.PermissionRequestParams{
			SessionID: sessionID,
			ToolCall: acp.PermissionToolCall{
				ToolCallID: tc.ID,
				Title:      fmt.Sprintf("Run: %s", tc.Name),
				Kind:       toolKind(tc.Name),
				Status:     "pending",
				Content: []acp.ToolCallResultItem{
					{Type: "content", Content: acp.ContentBlock{Type: "text", Text: permission.PromptBody(tc.Name, tc.InputJSON)}},
				},
			},
			Options: []acp.PermissionOption{
				{OptionID: "allow", Name: "Allow", Kind: "allow_once"},
				{OptionID: "allow_always", Name: "Allow always", Kind: "allow_always"},
				{OptionID: "reject", Name: "Reject", Kind: "reject_once"},
			},
		})

		if err != nil || permResult == nil || permResult.Outcome == "cancelled" || permResult.OptionID == "reject" {
			_ = a.server.SendSessionUpdate(sessionID, acp.ToolCallStatusUpdate{
				SessionUpdate: acp.UpdateTypeToolCallUpdate,
				ToolCallID:    tc.ID,
				Status:        "cancelled",
			})
			return "permission denied by user", nil
		}
		if st := sessionStatePtr(a.state); st != nil {
			permission.RecordAllowAlways(st, tc.Name, tc.InputJSON, env.CWD, permResult)
		}
	}

	// Execute the tool.
	var result string
	var execErr error

	// Check if it's an MCP tool (name contains __).
	if idx := strings.Index(tc.Name, "__"); idx >= 0 {
		serverName := tc.Name[:idx]
		toolName := tc.Name[idx+2:]
		result, execErr = a.callMCPTool(ctx, serverName, toolName, tc.InputJSON)
	} else {
		result, execErr = a.registry.Execute(ctx, tc.Name, tc.InputJSON, env)
	}

	status := "completed"
	if execErr != nil {
		status = "failed"
	}

	if sessionDir != "" && strings.TrimSpace(tc.ID) != "" {
		finalText := result
		if execErr != nil {
			finalText = fmt.Sprintf("error: %v", execErr)
		}
		_ = session.WriteToolCallResult(sessionDir, tc.ID, finalText)
		_ = session.MarkToolCallFinished(sessionDir, tc.ID, tc.Name, toolKind(tc.Name), status)
	}

	payload := result
	if execErr != nil {
		payload = fmt.Sprintf("error: %v", execErr)
	}
	var content []acp.ToolCallResultItem
	var previewMeta map[string]interface{}
	if strings.TrimSpace(payload) != "" {
		display, meta := session.PreviewToolResultForSessionUpdate(tc.Name, payload)
		previewMeta = meta
		content = []acp.ToolCallResultItem{
			{Type: "content", Content: acp.ContentBlock{Type: "text", Text: display}},
		}
	}

	_ = a.server.SendSessionUpdate(sessionID, acp.ToolCallStatusUpdate{
		SessionUpdate: acp.UpdateTypeToolCallUpdate,
		ToolCallID:    tc.ID,
		Status:        status,
		Content:       content,
		Meta:          previewMeta,
	})

	return result, execErr
}

// callMCPTool routes a tool call to the appropriate MCP client.
func (a *Agent) callMCPTool(ctx context.Context, serverName, toolName, argsJSON string) (string, error) {
	for _, client := range a.state.GetMCPClients() {
		if client.Name() == serverName {
			return client.CallTool(ctx, toolName, argsJSON)
		}
	}
	return "", fmt.Errorf("MCP server not found: %s", serverName)
}

// buildMessages constructs the message slice to send to the LLM.
// The most recent user message is augmented with bodies of any explicitly invoked (/name) skills
// so the LLM sees the full skill instructions immediately before the user's request.
// The stored history content is never modified — only the slice sent to the LLM differs.
func (a *Agent) buildMessages(systemPrompt string) []llm.Message {
	history := a.state.GetMessages()
	// The coddy compaction engine replays only the window from the last summary onward; earlier
	// history stays in the transcript for the UI. The opencode engine keeps the full slice and
	// relies on isLLMHistoryMessage to drop messages flagged Compacted.
	if a.cfg.Compaction.EngineIsCoddy() {
		history = session.MessagesForLLM(history)
	}
	allSkills := a.state.GetSkills()
	msgs := make([]llm.Message, 0, len(history)+1)
	msgs = append(msgs, llm.Message{Role: llm.RoleSystem, Content: systemPrompt})

	// Find the index of the most recent user message to augment it.
	lastUserIdx := -1
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == llm.RoleUser {
			lastUserIdx = i
			break
		}
	}

	for i, m := range history {
		if !isLLMHistoryMessage(m) {
			continue
		}
		if i == lastUserIdx && len(allSkills) > 0 {
			if aug := augmentUserMessageWithInvokedSkills(m.Content, allSkills); aug != m.Content {
				m.Content = aug
			}
		}
		msgs = append(msgs, m)
	}
	return msgs
}

// augmentUserMessageWithInvokedSkills prepends full skill bodies for any /name commands found
// in userText. The original text is preserved at the end so the LLM sees both the skill context
// and the user's exact request. Returns userText unchanged when no skills are invoked.
func augmentUserMessageWithInvokedSkills(userText string, allSkills []*skills.Skill) string {
	names := skills.ParseInvokedCommandNames(userText)
	if len(names) == 0 {
		return userText
	}
	idx := skills.SkillBySlashName(allSkills)
	var prefix strings.Builder
	for _, n := range names {
		sk, ok := idx[n]
		if !ok {
			continue
		}
		body := strings.TrimSpace(sk.Content)
		if body == "" {
			continue
		}
		prefix.WriteString("## Invoked skill: /")
		prefix.WriteString(n)
		prefix.WriteString("\n\n")
		prefix.WriteString(body)
		prefix.WriteString("\n\n---\n\n")
	}
	if prefix.Len() == 0 {
		return userText
	}
	return prefix.String() + userText
}

func isLLMHistoryMessage(m llm.Message) bool {
	if m.PlanDocument != nil && strings.TrimSpace(m.Content) == "" && len(m.ToolCalls) == 0 && strings.TrimSpace(m.Reasoning) == "" {
		return false
	}
	// Messages superseded by a compaction summary stay in the transcript for UI/replay but are
	// excluded from the payload sent to the model (the summary carries their content).
	if m.Compacted {
		return false
	}
	return true
}

// sendPlan sends the plan update to the client.
func (a *Agent) sendPlan(sessionID string, entries []acp.PlanEntry) error {
	return a.server.SendSessionUpdate(sessionID, acp.PlanUpdate{
		SessionUpdate: acp.UpdateTypePlan,
		Entries:       entries,
	})
}

// reasoningForStorage picks the reasoning text and signature to persist on an assistant message.
// When the provider signs the reasoning (Anthropic extended thinking), the exact unmodified text
// must be stored so the signature validates on replay; otherwise the trimmed text is used for display.
func reasoningForStorage(trimmed, exact string, response *llm.Response) (text, signature string) {
	if response != nil && response.ReasoningSignature != "" {
		return exact, response.ReasoningSignature
	}
	return trimmed, ""
}

// getProvider creates the LLM provider for the given mode.
func (a *Agent) getProvider(mode string) (llm.Provider, error) {
	modelID := a.state.EffectiveModelID(a.cfg)
	if modelID == "" {
		return nil, fmt.Errorf("no model configured")
	}

	rm, err := a.cfg.ResolveLLM(modelID)
	if err != nil {
		return nil, err
	}

	mk := a.providerFactory
	if mk == nil {
		mk = llm.NewProvider
	}
	in := a.llmProviderInput(rm)
	in.ReasoningEffort = a.state.EffectiveReasoning(a.cfg)
	return mk(in)
}

func (a *Agent) llmProviderInput(rm *config.ResolvedLLM) llm.ProviderInput {
	return llm.WithAgentResilience(llm.ProviderInput{
		Type:        rm.ProviderType,
		Model:       rm.Model,
		APIKey:      rm.APIKey,
		BaseURL:     rm.BaseURL,
		ProxyURL:    rm.ProxyURL,
		MaxTokens:   rm.MaxTokens,
		Temperature: rm.Temperature,
	}, a.cfg.Agent.LLMRetryMax, a.cfg.Agent.LLMRetryBaseMS, a.cfg.Agent.LLMMinIntervalMS)
}

// contentBlocksToText converts ACP content blocks to a plain text string.
// Hydrated attachments become **<foxxycode_attachment path="..." name="...">…</foxxycode_attachment>**
// with file body inside CDATA so the SPA can strip tags for display while the model retains full context.
func contentBlocksToText(blocks []acp.ContentBlock) string {
	var parts []string
	for _, b := range blocks {
		switch b.Type {
		case "text":
			parts = append(parts, b.Text)
		case "resource":
			if b.Resource != nil {
				parts = append(parts, resourceBlockToXMLAttachment(b.Resource))
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

func xmlEscapedAttr(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}

func wrapXMLCDATA(body string) string {
	// Split CDATA if the payload contains the terminator sequence.
	escaped := strings.ReplaceAll(body, "]]>", "]]]]><![CDATA[>")
	return "<![CDATA[" + escaped + "]]>"
}

func resourceBlockToXMLAttachment(res *acp.Resource) string {
	pathRaw := strings.TrimSpace(res.URI)
	pathRaw = strings.TrimPrefix(pathRaw, "file://")
	pathFwd := filepath.ToSlash(pathRaw)
	name := filepath.Base(pathFwd)
	if name == "." || name == "/" {
		name = pathFwd
	}
	var b strings.Builder
	b.WriteString(`<foxxycode_attachment path="`)
	b.WriteString(xmlEscapedAttr(pathFwd))
	b.WriteString(`" name="`)
	b.WriteString(xmlEscapedAttr(name))
	b.WriteString(`">`)
	b.WriteByte('\n')
	b.WriteString(wrapXMLCDATA(res.Text))
	b.WriteString("\n</foxxycode_attachment>")
	return b.String()
}

// extractContextFiles returns file paths referenced in content blocks.
func extractContextFiles(blocks []acp.ContentBlock) []string {
	var files []string
	for _, b := range blocks {
		if b.Type == "resource" && b.Resource != nil {
			uri := b.Resource.URI
			if strings.HasPrefix(uri, "file://") {
				files = append(files, strings.TrimPrefix(uri, "file://"))
			}
		}
	}
	return files
}

// toolKind maps a tool name to an ACP tool call kind.
func toolKind(name string) string {
	switch name {
	case "read", "glob", "grep", "websearch", "webfetch":
		return "read"
	case "write", "edit", "apply_patch", "mkdir", "rmdir", "touch", "rm", "mv":
		return "write"
	case "run_command":
		return "run_command"
	default:
		return "other"
	}
}

func filesystemWriteTool(name string) bool {
	switch name {
	case "write", "edit", "apply_patch", "mkdir", "rmdir", "touch", "rm", "mv":
		return true
	default:
		return false
	}
}

// effectivePermMode returns the session-level permission mode override, falling back to the config default.
func effectivePermMode(state SessionState, cfg *config.Config) string {
	if m := state.GetPermissionMode(); m != "" {
		return m
	}
	return cfg.Tools.ResolvedPermMode()
}

// extractCommand parses the "command" field from run_command JSON args.
func extractCommand(argsJSON string) string {
	return permission.ExtractRunCommand(argsJSON)
}

func sessionStatePtr(s SessionState) *session.State {
	st, ok := s.(*session.State)
	if !ok {
		return nil
	}
	return st
}

// filePathsNote builds an XML annotation listing the on-disk paths where
// uploaded files were saved.  Returns an empty string when no part has a
// FilePath set (e.g. sessions without a persistent directory).
// The tag is stripped from the user-visible bubble by the SPA's
// stripFoxxyCodeAttachmentsForUserDisplay function.
func filePathsNote(parts []llm.ImagePart) string {
	var lines []string
	for _, p := range parts {
		if p.FilePath == "" {
			continue
		}
		line := "- " + p.FilePath
		if p.Name != "" && p.Name != filepath.Base(p.FilePath) {
			line += " (" + p.Name + ")"
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<foxxycode_session_assets>Uploaded files saved to session assets (read-only). You can read or copy them:\n")
	for _, l := range lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	b.WriteString("</foxxycode_session_assets>")
	return b.String()
}

// ideEnvMaxTabs caps how many open tabs are listed in the IDE context block to
// bound token usage on workspaces with many editors open.
const ideEnvMaxTabs = 50

// ideEnvNote builds an XML annotation describing the files the user currently
// has open in their IDE (the focused tab plus every open tab), mirroring the
// environment context other coding agents inject each turn. Paths are made
// relative to cwd when they live under it. Returns an empty string when no IDE
// has reported any editor state.
//
// The tag is stripped from the user-visible bubble by the SPA's
// stripFoxxyCodeAttachmentsForUserDisplay function.
func ideEnvNote(cwd string) string {
	snap := ideenv.Get()
	if snap.ActiveFile == "" && len(snap.OpenFiles) == 0 {
		return ""
	}
	rel := func(p string) string {
		p = strings.TrimSpace(p)
		if p == "" {
			return ""
		}
		if cwd != "" {
			if r, err := filepath.Rel(cwd, p); err == nil && !strings.HasPrefix(r, "..") {
				return filepath.ToSlash(r)
			}
		}
		return filepath.ToSlash(p)
	}
	var b strings.Builder
	b.WriteString("<foxxycode_ide_context>\n# Active File\n")
	if af := rel(snap.ActiveFile); af != "" {
		b.WriteString(af)
	} else {
		b.WriteString("(none)")
	}
	b.WriteString("\n\n# Open Tabs\n")
	if len(snap.OpenFiles) == 0 {
		b.WriteString("(none)")
	} else {
		n := len(snap.OpenFiles)
		if n > ideEnvMaxTabs {
			n = ideEnvMaxTabs
		}
		for i := 0; i < n; i++ {
			if i > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(rel(snap.OpenFiles[i]))
		}
	}
	b.WriteString("\n</foxxycode_ide_context>")
	return b.String()
}

// terminalEnvMaxTerminals caps how many terminals are summarized in the
// always-on terminal context block, bounding token usage.
const terminalEnvMaxTerminals = 8

// terminalEnvMaxContextBytes caps the per-terminal output tail included in the
// always-on context block. It is kept short on purpose — the @terminal mention
// (terminalMentionNote) pulls the fuller buffer on demand.
const terminalEnvMaxContextBytes = 2 * 1024

// terminalEnvNote builds an XML annotation summarizing the IDE terminals the
// user currently has open (each with a short tail of recent output), mirroring
// ideEnvNote. The active terminal is listed first. Returns "" when no IDE has
// reported any terminal state.
//
// The tag is stripped from the user-visible bubble by the SPA's
// stripFoxxyCodeAttachmentsForUserDisplay function.
func terminalEnvNote() string {
	snap := ideterm.Get()
	if len(snap.Terminals) == 0 {
		return ""
	}
	ordered := terminalsActiveFirst(snap.Terminals)
	n := len(ordered)
	if n > terminalEnvMaxTerminals {
		n = terminalEnvMaxTerminals
	}
	blocks := make([]string, 0, n)
	for i := 0; i < n; i++ {
		tm := ordered[i]
		var b strings.Builder
		if tm.Active {
			b.WriteString("# Active Terminal: ")
		} else {
			b.WriteString("# Terminal: ")
		}
		b.WriteString(tm.Name)
		if lc := strings.TrimSpace(tm.LastCommand); lc != "" {
			b.WriteString("\n$ ")
			b.WriteString(lc)
		}
		if out := strings.TrimRight(tailBytes(tm.Output, terminalEnvMaxContextBytes), "\n"); out != "" {
			b.WriteByte('\n')
			b.WriteString(out)
		}
		blocks = append(blocks, b.String())
	}
	return "<foxxycode_terminal_context>\n" + strings.Join(blocks, "\n\n") + "\n</foxxycode_terminal_context>"
}

// terminalMentionRe matches an @terminal mention: bare `@terminal` or
// `@terminal:<name>` (name runs to the next whitespace). A leading boundary
// avoids matching inside another token (e.g. an email-like `x@terminal`).
var terminalMentionRe = regexp.MustCompile(`(?:^|\s)@terminal(?::(\S+))?`)

// terminalMentionNote expands @terminal / @terminal:<name> mentions found in the
// user text into a fuller <foxxycode_terminal_output> block carrying the
// complete captured buffer of the referenced terminal (the active terminal for
// a bare @terminal). Returns "" when there is no mention or no matching
// terminal. The tag is stripped from the user-visible bubble by the SPA.
func terminalMentionNote(userText string) string {
	matches := terminalMentionRe.FindAllStringSubmatch(userText, -1)
	if len(matches) == 0 {
		return ""
	}
	snap := ideterm.Get()
	if len(snap.Terminals) == 0 {
		return ""
	}
	seen := make(map[string]bool)
	blocks := make([]string, 0, len(matches))
	for _, m := range matches {
		tm := pickTerminal(snap.Terminals, strings.TrimSpace(m[1]))
		if tm == nil {
			continue
		}
		key := tm.ID + "\x00" + tm.Name
		if seen[key] {
			continue
		}
		seen[key] = true
		var b strings.Builder
		b.WriteString(`<foxxycode_terminal_output name="`)
		b.WriteString(xmlEscapedAttr(tm.Name))
		b.WriteString("\">\n")
		if out := strings.TrimRight(tm.Output, "\n"); out != "" {
			b.WriteString(out)
			b.WriteByte('\n')
		}
		b.WriteString("</foxxycode_terminal_output>")
		blocks = append(blocks, b.String())
	}
	if len(blocks) == 0 {
		return ""
	}
	return strings.Join(blocks, "\n\n")
}

// terminalsActiveFirst returns the terminals reordered so the active one(s)
// come first, preserving relative order otherwise.
func terminalsActiveFirst(ts []ideterm.Terminal) []ideterm.Terminal {
	out := make([]ideterm.Terminal, 0, len(ts))
	for _, t := range ts {
		if t.Active {
			out = append(out, t)
		}
	}
	for _, t := range ts {
		if !t.Active {
			out = append(out, t)
		}
	}
	return out
}

// pickTerminal returns the terminal matching name (case-insensitive), or the
// active terminal (falling back to the first) when name is empty. Returns nil
// when a named terminal is not found.
func pickTerminal(ts []ideterm.Terminal, name string) *ideterm.Terminal {
	if name == "" {
		for i := range ts {
			if ts[i].Active {
				return &ts[i]
			}
		}
		if len(ts) > 0 {
			return &ts[0]
		}
		return nil
	}
	for i := range ts {
		if strings.EqualFold(ts[i].Name, name) {
			return &ts[i]
		}
	}
	return nil
}

// tailBytes returns the last maxBytes bytes of s (trimmed to a rune boundary)
// when s exceeds the cap, otherwise s unchanged.
func tailBytes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	tail := s[len(s)-maxBytes:]
	for i := 0; i < len(tail) && i < 4; i++ {
		if tail[i]&0xC0 != 0x80 {
			return tail[i:]
		}
	}
	return tail
}
