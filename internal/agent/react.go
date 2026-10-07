// Package agent implements the ReAct (Reasoning + Acting) loop for a session turn.
// System prompts are rendered via internal/prompts (embedded templates or prompts.dir).
package agent

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/hooks"
	"github.com/hijera/foxxycode-agent/internal/ideenv"
	"github.com/hijera/foxxycode-agent/internal/ideterm"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/mcp"
	"github.com/hijera/foxxycode-agent/internal/mention"
	"github.com/hijera/foxxycode-agent/internal/permission"
	"github.com/hijera/foxxycode-agent/internal/plans"
	"github.com/hijera/foxxycode-agent/internal/platform"
	"github.com/hijera/foxxycode-agent/internal/session"
	"github.com/hijera/foxxycode-agent/internal/skills"
	"github.com/hijera/foxxycode-agent/internal/tooling"
	"github.com/hijera/foxxycode-agent/internal/tools"
	"github.com/hijera/foxxycode-agent/internal/tools/todo"
	toolweb "github.com/hijera/foxxycode-agent/internal/tools/web"
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
	GetMCPToolFilter() func(server, tool string) bool
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
	PendingPlanContext() string
	ClearPendingPlanContext()
	TakePendingImageParts() []llm.ImagePart
	GetPermissionMode() string
	// The settings the running turn works with (session/settings_state.go):
	// its own when a --once / --count override, a skill or the model's
	// switch_model set one, the session's otherwise. SettingsRevision moves
	// whenever one a model request reads changes.
	EffectiveMode() string
	EffectivePermissionMode() string
	SettingsRevision() uint64
	SetTurnSetting(setting, value string)
	TurnSetting(setting string) string
	// How the session_describe tool reaches the session's own filing
	// (session_filing.go). The writers report what they moved and do their own
	// merging, so the tool never has to read a filing it is about to write.
	ConversationTitle() string
	GetTags() []string
	ReplaceTitlePinned(title string) bool
	ReplaceTags(tags []string) (stored []string, changed bool)
	UpdateTags(add, remove []string) (stored []string, changed bool)
	IsUserCancelledTurn() bool
	// SetTurnStopNotice records why the running turn stopped before its
	// answer, for the session manager to hand to its caller (stop_notice.go).
	SetTurnStopNotice(msg string)
	GetTitlePinned() string
	GetTitleAuto() string
	SetTitleAuto(text string)
	// TakeQueuedMessages drains the follow-ups written while this turn runs
	// (session/turn_queue.go). The loop reads them between its own steps.
	TakeQueuedMessages() []session.QueuedMessage
	// QueuedMessages is what is still waiting after that drain: a message may
	// have been written while the batch was being read in.
	QueuedMessages() []session.QueuedMessage
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
	configReloader  func(context.Context) ([]string, error)

	// titleOnce keeps the session-title pass to a single launch per turn, whichever
	// exit path reaches it first.
	titleOnce sync.Once

	imgMu             sync.Mutex
	pendingToolImages []llm.ImagePart

	// subagentRuntime owns child sessions; nil when this surface cannot spawn.
	subagentRuntime SubagentRuntime
	// detachedPermissions answers a child's prompt after the parent turn that
	// spawned it has ended; nil on surfaces with nowhere to put one.
	detachedPermissions DetachedPermissionBroker
	// subagent is set when this session is itself a child run (see subagent.go).
	subagent *session.SubagentMeta
	// progress is the running turn's clock and token count (turn_progress.go).
	progress *turnProgress
	// limitWaitHeartbeat overrides how often a waiting turn re-sends its
	// countdown (tests); zero means limitWaitHeartbeat.
	limitWaitHeartbeat time.Duration
	// limitLedger is the user turn's account of time spent on usage
	// limits (limit_wait.go); Run starts a fresh one.
	limitLedger *limitWaitLedger
	// currentToolCallID is the tool call being executed, so a spawn can link
	// its task to the transcript row.
	currentToolCallID string

	// loopPins names read/grep results the loop guard rescued from eviction after a
	// re-read loop, so the content the model kept circling back for stays in the
	// projection. loopQuarantine names the calls it took away for the rest of the
	// turn; the projection reads it too, to collapse the repeats those calls left
	// behind. Both are guarded: the projection also runs from the compaction pass.
	pinMu          sync.Mutex
	loopPins       map[string]struct{}
	loopQuarantine map[string]struct{}

	// hooks is the operator hook runner of the current turn, built on first
	// use from the definition files (hooks.go). hookStopReason carries a
	// continue:false answered by a hook to the loop, which ends the turn.
	hooks          *hooks.Runner
	hooksMu        sync.Mutex
	hooksLoaded    bool
	hookStopReason string
	// turnHookContext is what UserPromptSubmit hooks handed over for this
	// turn's system prompt (hooks.go).
	turnHookContext string
	// autoCompactSkipLogged records that this turn already logged an
	// automatic compaction with nothing to fold (compact.go).
	autoCompactSkipLogged bool
	// clock is the wall clock the turn context block reads; nil means
	// time.Now. Tests that assert on a rendered timestamp set it.
	clock func() time.Time
	// memoryRun is the memory subagent this turn started, or nil
	// (memory_run.go). The Agent lives for one turn, so it needs no reset.
	memoryRun *memoryTurnRun
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
	if log == nil {
		log = slog.Default()
	}
	environment := platform.CurrentEnvironment()
	a := &Agent{
		cfg:             cfg,
		state:           state,
		server:          server,
		log:             log,
		registry:        tools.NewRegistryForEnvironment(cfg, environment),
		environment:     environment,
		providerFactory: llm.NewProvider,
	}
	if st := sessionStatePtr(state); st != nil {
		a.subagent = st.Subagent()
	}
	// The system memory child gets its tools here, in its own registry;
	// nothing else ever sees them.
	a.registerMemoryChildTools()
	return a
}

// SetProviderFactory replaces the LLM provider factory used by subsequent turns.
func (a *Agent) SetProviderFactory(mk func(llm.ProviderInput) (llm.Provider, error)) {
	if a == nil || mk == nil {
		return
	}
	a.providerFactory = mk
}

// SetConfigReloader wires config_commit and config_rollback to the process/session runtime owner.
func (a *Agent) SetConfigReloader(reload func(context.Context) ([]string, error)) {
	if a == nil {
		return
	}
	a.configReloader = reload
}

// Run executes the ReAct loop and returns the stop reason.
func (a *Agent) Run(ctx context.Context, prompt []acp.ContentBlock) (string, error) {
	if a.log != nil {
		ctx = llm.WithDiagnostics(ctx, a.log.With("session", a.state.GetID()))
		a.log.DebugContext(ctx, "agent.run.start", "session", a.state.GetID())
		started := time.Now()
		defer func() {
			a.log.DebugContext(ctx, "agent.run.end", "session", a.state.GetID(), "elapsed_ms", time.Since(started).Milliseconds())
		}()
	}
	mode := a.state.EffectiveMode()
	// A new user turn starts its account of time spent on usage limits
	// (limit_wait.go); the built-ins below never touch it.
	a.limitLedger = &limitWaitLedger{}
	// Hook definitions are re-read for every turn.
	a.resetHooks()
	a.hookStopReason = ""
	a.turnHookContext = ""

	// Build the user message from prompt content blocks.
	a.state.ClearMemoryCopilotBlock()
	userText := contentBlocksToText(prompt)

	// The built-in /compact, /plugin, and /export commands are operator input:
	// they run deterministically, outside the tool set and the permission
	// gate. A child's prompt is written by the parent model, so for a subagent
	// the same text is an ordinary task and never reaches the built-ins.
	// Attachments are data: only the operator's typed text names a command.
	if a.subagent == nil {
		typed := typedText(prompt)
		// The built-in /compact command compacts history instead of running the ReAct
		// loop. runCompactCommand persists the command text itself (so it shows in the
		// transcript like any other message), and under the opencode engine returns a
		// short notice instead of compacting.
		if args, ok := parseCompactCommand(typed); ok {
			return a.runCompactCommand(ctx, args, userText)
		}
		// The built-in /plugin command manages skill plugins and marketplaces
		// deterministically, without an LLM turn; the command text is persisted too.
		if args, ok := parsePluginCommand(typed); ok {
			return a.runPluginCommand(ctx, args, userText)
		}
		// The built-in /export command writes the transcript to a file in the
		// workspace; the command text is persisted after the export is built.
		if args, ok := parseExportCommand(typed); ok {
			return a.runExportCommand(ctx, args, userText)
		}
		// The built-in /export command writes the transcript to a file in the
		// workspace; the command text is persisted after the export is built.
		if args, ok := parseExportCommand(userText); ok {
			return a.runExportCommand(ctx, args, userText)
		}
	}
	// The bodies of the skills the prompt invokes as /name, and the rules its
	// mentioned paths activate, ride in this message (mentions.go), never in
	// the system prompt and never added per request.
	for _, inv := range invokedSkills(typedText(prompt), a.state.GetSkills()) {
		a.applySkillSettings(ctx, inv.name, inv.skill)
	}
	if extra := invokedSkillBlocks(typedText(prompt), a.state.GetSkills()); len(extra) > 0 {
		prompt = append(append([]acp.ContentBlock(nil), prompt...), extra...)
		userText = contentBlocksToText(prompt)
	}
	if withRules := a.attachActivatedRules(prompt); len(withRules) != len(prompt) {
		prompt = withRules
		userText = contentBlocksToText(prompt)
	}
	// UserPromptSubmit hooks see the prompt before it becomes a message: a
	// rejected prompt is never added, and the turn ends with the reason.
	if reason, rejected := a.runUserPromptHooks(ctx, mode, userText); rejected {
		return string(acp.StopReasonRefused), fmt.Errorf("prompt rejected by hook: %s", reason)
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
	// A turn finished background tasks started opens with the wake. The pool
	// marks the tasks that woke the agent first, so a surface that reads its
	// task list again on the wake finds the mark; the clients hear the wake
	// before the message is persisted, like every frame a message describes;
	// and the message keeps the marker, so no surface shows the instruction
	// as something the operator typed, after a reload either.
	wake := a.takeTurnWake()
	if wake != nil {
		a.markWokeTasks(wake)
		_ = a.server.SendSessionUpdate(a.state.GetID(), session.BackgroundWakeUpdate(wake))
	}
	a.state.AddMessage(llm.Message{
		Role:           llm.RoleUser,
		Content:        messageContent,
		ImageParts:     imageParts,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
		BackgroundWake: wake,
	})
	a.setHookTurn(session.CountUserTurns(a.state.GetMessages()))
	// The turn's clock is announced before anything slow happens - the memory
	// run below, the first model call - so a surface counts from the start.
	a.beginTurnProgress()
	defer a.endTurnProgress()
	finishMemory := a.debugStage(ctx, "memory_before_turn")
	a.runMemoryBeforeTurn(ctx, userText, mode)
	finishMemory()
	// A report that lands after this turn returned is history in the Tasks
	// drawer, never the next turn's context.
	defer a.finishMemoryTurn()

	// Collect context files from the prompt for skill filtering.
	contextFiles := extractContextFiles(prompt)

	// Load skills applicable to this context.
	activeSkills := FilterSkillsForContext(a.state.GetSkills(), contextFiles)

	toolDefs := a.currentToolDefinitions(mode)

	// Get or create LLM provider.
	finishProvider := a.debugStage(ctx, "provider_setup")
	transport, err := a.getProvider(mode)
	finishProvider()
	if err != nil {
		return string(acp.StopReasonRefused), fmt.Errorf("no LLM configured: %w", err)
	}

	// Restore existing plan via session/update if one was set by foxxycode todo tools in a previous turn.
	if existing := a.state.GetPlan(); len(existing) > 0 {
		if err := a.sendPlan(a.state.GetID(), existing); err != nil {
			a.log.Warn("failed to restore plan", "error", err)
		}
	}

	// Build the full message list starting with the system prompt. It is
	// rendered once here and then frozen for the whole turn so the provider's
	// prefix cache keeps the conversation behind it (buildSystemPromptParts).
	finishContext := a.debugStage(ctx, "build_context")
	sys := a.buildSystemPromptParts(mode, activeSkills, toolDefs, contextFiles)
	messages := a.buildMessages(sys.Content)
	finishContext()
	// The hand-off belongs to this turn and to its continuation after a
	// permission prompt, and to nothing after that.
	defer a.releasePlanContext()

	// FoxxyCode engine: buildSystemPromptParts refreshed the context breakdown, so
	// compact before the first LLM call when the estimate crossed the
	// auto-compaction threshold, then rebuild the payload from the windowed
	// history. A compaction is a legitimate reason to render the system message
	// again: the prefix behind it has just been rewritten anyway.
	finishCompaction := a.debugStage(ctx, "initial_compaction")
	if a.cfg.Compaction.EngineIsCoddy() && a.maybeAutoCompact(ctx) {
		sys = a.buildSystemPromptParts(mode, activeSkills, toolDefs, contextFiles)
		messages = a.buildMessages(sys.Content)
	}
	finishCompaction()

	// Restarting the app resets every guard in this file; it does not reset the
	// transcript that tripped them. A turn that died writing one answer over and
	// over is still on disk, and replaying it unremarked invites the next attempt to
	// open exactly the same way - which is what "after a restart it starts the same
	// dialogue again" is. Re-derived from the messages rather than a stored marker,
	// because the process that would have written one is usually the process that
	// was killed. LLM-facing only, like every other nudge.
	if restarts, steps := priorTurnRestarts(a.state.GetMessages()); restarts > 0 {
		a.log.Warn("previous turn ended writing the same answer over; telling the model", "restarts", restarts)
		messages = append(messages, llm.Message{Role: llm.RoleUser, Content: resumedRestartNudge(restarts, steps)})
	}
	finishCompaction()

	maxTurns := a.cfg.Agent.MaxTurns
	if maxTurns <= 0 {
		// fork(max-turns-default): upstream 1.2.9 reads an unset max_turns as
		// no step limit at all; FoxxyCode keeps 30.
		maxTurns = 30
	}
	if a.subagent != nil {
		maxTurns = a.cfg.Subagents.EffectiveMaxTurns(a.cfg.Agent.MaxTurns)
		if a.subagent.MaxTurns > 0 {
			maxTurns = a.subagent.MaxTurns
		}
	}

	sd := strings.TrimSpace(a.state.GetPersistedSessionDir())

	toolEnv := &tools.Env{
		CWD:              a.state.GetCWD(),
		PermissionMode:   effectivePermMode(a.state, a.cfg),
		CommandAllowlist: a.cfg.Tools.CommandAllowlist,
		HTTPAllowlist:    a.cfg.Tools.HTTPRequest.Allowlist,
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
		Sender:         a.server,
		GetPlan:        a.state.GetPlan,
		SetPlan:        a.state.SetPlan,
		SetSessionMode: a.setSessionModeAnnounced,
		CompactSession: a.compactFromTool,
		FileSession: func(upd tooling.SessionFilingUpdate) (tooling.SessionFilingResult, error) {
			return applySessionFiling(a.state, upd)
		},
		PersistPlanDocument: func(doc plans.Document) {
			a.state.AppendPlanDocument(doc)
		},
		SSHConnectTimeout: a.cfg.Tools.SSHConnectTimeout,
		LoadSkillBody:     a.loadSkillBody,
		ConfigPath:        a.cfg.Paths.ConfigPath,
		ConfigHome:        a.cfg.Paths.Home,
		ConfigCWD:         a.cfg.Paths.CWD,
		OutputLineLimits:  a.cfg.Tools.OutputLimits.AsMap(),
		Background:        a.backgroundPool(sd),
		BackgroundEnabled: a.cfg.Tools.Background.ResolvedEnabled(),
		WebSearch:         webSearchSettings(a.cfg),
		PreviewServer:     previewServerSettings(a.cfg),
	}
	// The model's own model switch; a subagent runs on what its parent chose.
	if a.subagent == nil && a.settings() != nil {
		toolEnv.SwitchModel = a.switchModel
	}
	if a.configReloader != nil {
		toolEnv.ReloadConfig = func(ctx context.Context) ([]string, error) {
			warnings, err := a.configReloader(ctx)
			if err != nil {
				return warnings, err
			}
			next, err := config.LoadWithPaths(a.cfg.Paths)
			if err != nil {
				return warnings, err
			}
			a.cfg = next
			a.registry = tools.NewRegistryForEnvironment(next, a.environment)
			return warnings, nil
		}
	}
	toolEnv.SendDesignPlanUpdate = func(doc plans.Document) {
		tools.SendDesignPlanUpdate(toolEnv, doc)
	}
	toolEnv.AddToolImage = func(dataURL, filePath, name string) {
		a.addToolImage(llm.ImagePart{DataURL: dataURL, FilePath: filePath, Name: name})
	}
	a.wireFileEditHook(toolEnv)

	a.applySubagentEnv(toolEnv, mode)

	return a.runReActLoop(ctx, mode, sys, messages, toolDefs, transport, toolEnv, sd, userText, contextFiles, activeSkills, maxTurns, true)
}

// releasePlanContext hands back the design plan hand-off once the turn that ran
// the plan is really over.
//
// A turn stopped on a permission prompt is not over: the user answers it later,
// possibly in another process, and ResumeAfterPermission renders this turn's
// system prompt again. So the gate left in the bundle is what decides - while
// one is held, the hand-off stays where the continuation can find it.
//
// A process that dies mid-turn with no gate held leaves the record behind, and
// the next turn of that session carries the plan text once more before
// releasing it. That is the right way round: the plan was not finished, and
// one extra turn of context costs less than dropping it.
func (a *Agent) releasePlanContext() {
	if sd := strings.TrimSpace(a.state.GetPersistedSessionDir()); sd != "" && session.PendingPermissionHeld(sd) {
		return
	}
	a.state.ClearPendingPlanContext()
}

// setSessionModeAnnounced switches the session profile and tells the client it
// happened, the way session.RunPlan does. plan_exit is the model's own way out
// of plan mode, so a client that never hears about it keeps posting "plan" and
// the next turn writes that back onto the session.
func (a *Agent) setSessionModeAnnounced(mode string) error {
	m := strings.TrimSpace(mode)
	a.state.SetMode(m)
	if a.server == nil {
		return nil
	}
	if err := a.server.SendSessionUpdate(a.state.GetID(), acp.ModeUpdate{
		SessionUpdate: acp.UpdateTypeCurrentModeUpdate,
		CurrentModeID: m,
	}); err != nil {
		a.log.Warn("failed to send mode update", "error", err)
	}
	return nil
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

// maxEmptyAssistantReissues bounds how many times an empty turn is answered by
// replaying the identical request before the model is talked to in words. One
// model name at a proxy is usually a group of interchangeable deployments, and
// a member that returns reasoning with neither text nor a tool call fails that
// way per attempt: the replay lands on another member and comes back with the
// tool call the first one lost. Only after that is the model itself nudged,
// which is the recovery that helps when the model, not the lane, is at fault.
const maxEmptyAssistantReissues = 1

// maxFirstTokenReissues bounds how many times a streamed call the first-token
// guard cut with nothing produced is replayed immediately, before the stall
// ladder takes over with its minute-scale waits. The two answer different
// failures: a sick member of a load-balanced group fails per attempt and the
// very next draw succeeds, while a saturated gateway is out for minutes and
// only a wait helps. Trying the cheap one first costs one round trip and
// spares the user the wait whenever the lane, not the provider, was at fault.
// Safe by construction: the cut call emitted no chunk to the client and
// appended no message, so the replay cannot show or store anything twice.
//
// It is the first rung of the same ladder, so it answers to the same switch:
// llm_stall_retry off means a call that produced nothing ends the turn, as it
// did before either recovery existed.
const maxFirstTokenReissues = 1

// emptyAssistantContinuationNudge is injected into the LLM-facing message slice (never
// persisted to the transcript) to prompt the model to produce its answer or a tool call
// after an empty turn.
const emptyAssistantContinuationNudge = "Your previous message had no answer text and no tool call. Continue now: call the appropriate tool to act, or write your reply to the user."

// emptyRecoveryProjection removes unanswered assistant messages only from the
// request, retaining their signed reasoning in session history. A rebuild after
// compaction restores that entire tail, so remove all of it and restore the
// local-only nudges the recovery has already earned.
func emptyRecoveryProjection(messages []llm.Message, nudges int) []llm.Message {
	for len(messages) > 0 {
		last := messages[len(messages)-1]
		if last.Role != llm.RoleAssistant || strings.TrimSpace(last.Content) != "" || len(last.ToolCalls) != 0 {
			break
		}
		messages = messages[:len(messages)-1]
	}
	for i := 0; i < nudges; i++ {
		messages = append(messages, llm.Message{Role: llm.RoleUser, Content: emptyAssistantContinuationNudge})
	}
	return messages
}

// Loop-guard nudges are injected into the LLM-facing message slice only (never
// persisted to the transcript), the same way emptyAssistantContinuationNudge is.
// The repeated passage itself is stripped from the assistant message before the
// replay, so the nudge does not carry the loop straight back into the model.
const (
	streamLoopNudge = "Your previous response was cut off because it degenerated into repeating the same passage over and over. Do not continue that text. Decide what is actually left to do, then either call the appropriate tool or write a short, concrete reply to the user."

	reasoningLoopNudge = "Your previous turn was cut off because your reasoning kept repeating the same thought without reaching a conclusion. Stop deliberating and act: call the appropriate tool, or write your reply to the user now."

	toolLoopNudge = "You have requested the same tool call with identical arguments several times in a row, so it was not executed again. Repeating it will not produce a different result. Use what you already have: try a different tool or different arguments, or answer the user with the information you have."

	toolLoopSkippedResult = "not executed: the loop guard stopped this turn after repeated identical tool calls"

	// permissionDeniedByUser is what a tool call gets when the gate was
	// answered with a refusal. It is matched verbatim elsewhere (the
	// context-eviction pass reads it as "this write never happened"), so it
	// stays a constant rather than a literal repeated per call site.
	permissionDeniedByUser = "permission denied by user"

	// permissionNotGrantedPrefix opens the refusals nobody actually answered -
	// a detached subagent's prompt that reached no client, say. The model must
	// not read those as a user saying no, and the eviction pass must still
	// treat the write as not done, so both share this prefix.
	permissionNotGrantedPrefix = "permission not granted: "

	toolCycleNudge = "This call repeats a sequence of tool calls you have already run in this turn with the same arguments, so it was not executed again. Running it again will not return anything new. Note that an \"[evicted: ...]\" placeholder only means the text was dropped from your context to save room - the file has not changed, and re-reading it will produce the same content you already reasoned about. Work from what you have and take a genuinely different step, or tell the user what you cannot determine; do not repeat the sequence."

	toolCycleSkippedResult = "not executed: the loop guard stopped this turn after a repeating sequence of tool calls"

	// permissionTimeoutReason explains a gate that tools.permission_timeout_seconds
	// closed. It travels as a PermissionResult.Reason so permissionDeniedResult
	// renders it behind permissionNotGrantedPrefix: nobody refused the call, and
	// the model must not tell the operator they rejected a prompt they never saw.
	permissionTimeoutReason = "the permission request timed out with no answer from the operator"

	// toolCycleStopNotice is the notice surfaced when a turn keeps cycling through
	// the same sequence after every nudge. Unlike the identical-call notice it
	// interpolates nothing, so the SPA can match it by equality
	// (external/ui/src/ui/chat/loopGuardNotice.ts). Keep the two byte-identical.
	toolCycleStopNotice = "stopped: the model kept repeating the same sequence of tool calls"

	toolQuarantinedResult = "not executed: the loop guard has taken this call away for the rest of this turn because it was being repeated without progress. Its earlier result stands, above in this conversation. Do something different, or answer with what you already know."

	loopAnswerDirective = "The tool calls you keep asking for have been taken away for the rest of this turn, and no tools are available on this request. Write your reply to the user now, using everything you have already gathered. If something could not be determined, say so plainly instead of describing the tool call you would have made."

	// restartAnswerDirective is loopAnswerDirective's twin for a turn the stall
	// guard keeps restarting. Nothing has been taken away from the model there -
	// the connection, not the guard, is eating the answer - so the wording must not
	// accuse it of a tool loop it never ran.
	restartAnswerDirective = "The connection has cut this answer several times and you started it from the top on each attempt. No tools are available on this request: write the reply now, as briefly as you can, from what you have already gathered. If something could not be determined, say so plainly instead of describing the tool call you would have made."
)

// maxBlockedRoundsBeforeAnswer bounds how many rounds may consist purely of
// quarantined calls before the loop takes the tools away for one request and asks
// for the answer. Without it a model that knows nothing but the loop would keep
// asking for blocked calls until max_turns, ending the turn with nothing to show.
const maxBlockedRoundsBeforeAnswer = 2

// permissionDeniedResult renders a refused gate for the model, naming the
// reason when the refusal came from something other than a user's answer.
func permissionDeniedResult(res *acp.PermissionResult) string {
	if res != nil {
		if reason := strings.TrimSpace(res.Reason); reason != "" {
			return permissionNotGrantedPrefix + reason
		}
	}
	return permissionDeniedByUser
}

// loopAbortChannel names the streamed channel that degenerated into a loop.
type loopAbortChannel int

const (
	loopAbortNone loopAbortChannel = iota
	loopAbortText
	loopAbortReasoning
)

func (a *Agent) runReActLoop(
	ctx context.Context,
	mode string,
	sys *systemPromptBuild,
	messages []llm.Message,
	toolDefs []llm.ToolDefinition,
	transport llmTransport,
	toolEnv *tools.Env,
	sd, userText string,
	contextFiles []string,
	activeSkills []*skills.Skill,
	maxTurns int,
	allowTitleGen bool,
) (stop string, runErr error) {
	checkpoint, err := session.ReadExecutionCheckpoint(sd)
	if err != nil {
		return string(acp.StopReasonRefused), err
	}
	recovering := checkpoint.Status == "running" || checkpoint.Status == "interrupted" || checkpoint.Status == "max_turns" || checkpoint.Status == "no_progress"
	scope := session.ObservationHash(userText)
	if allowTitleGen && !continuationRequest(userText) && checkpoint.Scope != "" && checkpoint.Scope != scope {
		checkpoint.Seen = nil
		checkpoint.Repeats = 0
	}
	if !continuationRequest(userText) || checkpoint.Scope == "" {
		checkpoint.Scope = scope
	}
	checkpoint.Status = "running"
	if err := checkpoint.Save(sd); err != nil {
		return string(acp.StopReasonRefused), err
	}
	defer func() {
		if checkpoint.Status != "no_progress" {
			switch {
			case a.state.IsUserCancelledTurn():
				checkpoint.Status = "cancelled"
			case runErr != nil || stop == string(acp.StopReasonCancelled):
				checkpoint.Status = "interrupted"
			case stop == string(acp.StopReasonMaxTurns):
				checkpoint.Status = "max_turns"
			default:
				checkpoint.Status = "completed"
			}
		}
		if err := checkpoint.Save(sd); err != nil && runErr == nil {
			runErr = err
		}
	}()
	var totalInputTokens, totalOutputTokens int
	var lastStatsWrite time.Time
	// Session-global index for the turn this loop is running, resolved once so every
	// mid-turn stats write lands on the same row instead of appending a new one.
	sessionTurnIndex := session.NextTurnIndex(sd)
	var emptyContinuations int
	// Replays of a request the lane, not the model, failed to answer. Both are
	// reset alongside emptyContinuations once the model makes progress.
	var emptyReissues int
	var firstTokenReissues int
	var lastInputTokens int
	// Numbers every model call of the turn, retries and continuations included,
	// for the debug log and the network trace that carries it.
	var llmCalls int
	var turnHadVisibleText bool
	var retryAllowance *llm.RetryAllowance
	nextCallReason := "step"
	// A new step earns a fresh allowance; consecutive unanswered requests do
	// not. Explicit continuations keep their own configured bounds.
	resetRetries := func(reason string) {
		retryAllowance = nil
		emptyContinuations, emptyReissues, firstTokenReissues = 0, 0, 0
		nextCallReason = reason
	}

	// Run opened the turn's progress already; a loop entered another way (a
	// resumed permission) opens its own.
	a.beginTurnProgress()
	defer a.endTurnProgress()

	// Runaway-loop protection. The tool detector spans the whole user turn (a model
	// can repeat the same call across ReAct rounds, not only inside one response);
	// the stream detectors are per LLM call and created below. loopNudges is the
	// shared budget: once it runs out, the next detected loop stops the turn.
	guardOn := a.cfg.Agent.LoopGuardEnabled()
	streamRepeatCycles := 0
	var toolRepeats *toolRepeatDetector
	var toolCycles *toolCycleDetector
	loopNudgeBudget := 0
	if guardOn {
		streamRepeatCycles = a.cfg.Agent.EffectiveLoopStreamRepeatCycles()
		toolRepeats = newToolRepeatDetector(a.cfg.Agent.EffectiveLoopToolRepeatLimit())
		toolCycles = newToolCycleDetector(a.cfg.Agent.EffectiveLoopToolCycleRepeats())
		loopNudgeBudget = a.cfg.Agent.EffectiveLoopNudgeMax()
	}
	loopNudges := 0
	// Stop hooks may send the agent back to work; stopBlocks counts those
	// continuations against hooks.stop_loop_limit and stopHookActive tells
	// the hook that it already did so in this turn.
	stopHookActive := false
	stopBlocks := 0

	// What this turn has spent waiting for hit usage limits (limit_wait.go).
	// The same ledger the provider wrapper charges, so its retry sleeps and
	// the loop's waits share one maximum.
	limitWait := a.limitLedgerFor()
	// stuckAction decides what happens once a tool loop has survived every nudge:
	// quarantine the calls and carry on (the default), or end the turn.
	stuckAction := a.cfg.Agent.EffectiveLoopStuckAction()
	// The quarantine is per turn: what the guard took away last time must not
	// silence a call the user has just asked for again.
	a.resetLoopQuarantine()
	// blockedRounds counts consecutive rounds in which the model asked only for
	// quarantined calls and executed nothing: it has nothing left but the loop.
	blockedRounds := 0

	// fork(retry-recovery-steps): unanswered replays preserve useful max_turns steps.
	// Recovering from a provider that went quiet (agent.llm_stall_retry).
	// recoveryTurns counts loop iterations spent on that rather than on advancing
	// the model's plan - a replayed call that produced nothing, and a continuation
	// after the connection died mid-answer. Neither is a step the model chose, so
	// neither shrinks max_turns; both stay bounded by something else
	// (llm_retry_max, llm_stall_retry_max_wait_ms and agent.llm_continue_max).
	stalls := newStallRetry(&a.cfg.Agent)
	// The memory child already tries its configured fallback models inside the
	// provider call. When all reject the request, settle its task immediately so
	// the parent can continue; the long outage schedule belongs to user turns.
	if a.subagent != nil && a.subagent.Kind == session.SubagentKindMemory {
		stalls.enabled = false
	}
	recoveryTurns := 0
	// stallContinues counts every continuation of a cut answer this turn, after
	// a stall and after a provider failure alike: one budget,
	// agent.llm_continue_max (fork(continue-budget)).
	stallContinues := 0
	// A notice belongs to the turn that set it.
	a.state.SetTurnStopNotice("")
	// Set when the client has been told the turn is parked behind a partial answer,
	// so the same goroutine that sees the provider come back can take it away again.
	// Atomic: it is set on the loop and cleared from the stream callback.
	var announcedPark atomic.Bool
	// A turn the stall guard restarts can come back with the same answer all over
	// again, which no detector in loopguard.go can see: each attempt is a separate,
	// well-formed response, and the repetition exists only between them. See
	// attempt_repeat.go. restartForceAnswer is the escalation, read at the top of the
	// next iteration so it takes the tools off that request.
	attemptRepeats := newAttemptRepeatDetector()
	attemptRestarts := 0
	restartForceAnswer := false
	// replaying marks an iteration that sends the previous request again, or
	// carries on the answer it cut: no step of the model's lies between the two,
	// so a follow-up the operator queued is not folded into it (see the queue
	// read at the top of the loop).
	replaying := false

	// maxTurns bounds the model's reasoning steps, so the bound grows with the
	// iterations spent recovering from a provider failure and reactTurn - the index
	// the loop body reasons with - stays the count of real steps.
	// What the transport was built for, to notice a change between requests.
	transportRev := a.state.SettingsRevision()
	transportKey := a.transportKey()

	for turn := 0; turn < maxTurns+recoveryTurns; turn++ {
		if ctx.Err() != nil {
			return string(acp.StopReasonCancelled), nil
		}
		reactTurn := turn - recoveryTurns

		// Tool definitions for this one request. They are normally the turn's set,
		// but a model with nothing left but quarantined calls gets one tools-free
		// request so it has to answer from what it gathered. The system prompt is
		// rendered from the same slice below, so its tool section disappears too.
		callDefs := toolDefs
		forceAnswer := blockedRounds >= maxBlockedRoundsBeforeAnswer || restartForceAnswer
		if forceAnswer {
			callDefs = nil
		}

		a.emitDebug(turn, "turn_start", mode, "", map[string]interface{}{
			"mode":     mode,
			"model":    a.state.EffectiveModelID(a.cfg),
			"messages": len(messages),
			"tools":    len(callDefs),
		})

		// A follow-up the operator wrote while the previous step ran is read
		// here, before the request that answers that step's tool results is
		// built: that is what puts the correction inside the work instead of
		// after it. It is appended before the rebuild below, so a compaction
		// that replays the transcript carries it too.
		//
		// Not on a recovery iteration. A re-issued request is meant to be the
		// one that failed, and a continuation of a cut answer is meant to follow
		// that answer: a user message slipped in between would be answered by the
		// replay, and in the transcript it would split the half-written answer
		// from its own continuation. The follow-up waits one step - the next real
		// one, the end-of-turn read below, or the manager's boundary.
		if !replaying {
			if a.readQueuedMessages(&messages) {
				resetRetries("queued_followup")
			}
		}
		replaying = false

		// A model or a reasoning level changed since the transport was built -
		// by the operator, a --once override, the model's own switch_model -
		// takes effect from this request, never inside a stream.
		if rev := a.state.SettingsRevision(); rev != transportRev {
			transportRev = rev
			if key := a.transportKey(); key != transportKey {
				next, err := a.getProvider(mode)
				if err != nil {
					a.log.Warn("settings changed mid-turn but the new model is unavailable; keeping the current one", "error", err)
				} else {
					a.log.Info("model settings changed mid-turn", "from", transportKey, "to", key)
					transport, transportKey = next, key
				}
			}
		}

		// The system message stays exactly as the turn rendered it, so the
		// provider's cached copy of everything behind it survives this step.
		// What moved since - the wall clock, the todo checklist after a
		// foxxycode_todo_* call, the rules a filesystem tool activated - travels in
		// the turn context block appended after the history at the send
		// boundary below (turn_context.go).
		//
		// The exception is a template under prompts.dir that prints those facts
		// itself: its own conditionals have to keep matching the state, so it is
		// re-rendered here as every template was before, and carries no block.
		//
		// The tools-free request of a forced answer does not touch the frozen
		// message either: it gets a one-off system prompt at the send boundary.
		if sys.Volatile && len(messages) > 0 && messages[0].Role == llm.RoleSystem {
			sys = a.buildSystemPromptParts(mode, activeSkills, toolDefs, contextFiles)
			messages[0].Content = sys.Content
		}
		turnCtx := a.buildTurnContext(sys)

		// Auto-compaction: when the conversation approaches the context window, summarize older
		// turns and rebuild the payload from the rewritten history. Non-fatal on error. The foxxycode
		// engine re-checks between turns (the first check ran before the loop); the opencode engine
		// checks every turn against the provider's real input-token count.
		//
		// The estimate is refreshed first: the system message no longer is, and the
		// estimate is what the foxxycode trigger reads while tool results grow.
		a.refreshContextBreakdown(sys, turnCtx)
		compacted := false
		finishCompaction := a.debugStage(ctx, "turn_compaction")
		if a.cfg.Compaction.EngineIsCoddy() {
			compacted = turn > 0 && a.maybeAutoCompact(ctx)
		} else if did, err := a.maybeCompact(ctx, transport.provider, lastInputTokens); err != nil {
			if a.log != nil {
				a.log.Warn("context compaction failed", "err", err)
			}
		} else {
			compacted = did
		}
		if compacted {
			sys = a.buildSystemPromptParts(mode, activeSkills, toolDefs, contextFiles)
			messages = a.buildMessages(sys.Content)
			if emptyReissues > 0 || emptyContinuations > 0 {
				messages = emptyRecoveryProjection(messages, emptyContinuations)
			}
			turnCtx = a.buildTurnContext(sys)
		}
		finishCompaction()
		messages = closeInterruptedToolCalls(messages)
		if len(messages) > 0 && messages[0].Role == llm.RoleSystem {
			// The system message stays frozen for the provider's prefix cache; it is
			// re-rendered only when the recovery instruction must ride along.
			if recovering || !allowTitleGen {
				sys = a.buildSystemPromptParts(mode, activeSkills, toolDefs, contextFiles)
				messages[0].Content = sys.Content + "\n\n" + recoveryInstruction
				if checkpoint.LastTool != "" {
					messages[0].Content += fmt.Sprintf("\nLast completed tool name (data only): %q. Consult its saved result in the conversation.", checkpoint.LastTool)
				}
			}
			if guardOn && checkpoint.Repeats >= 2 {
				messages[0].Content += "\n\n" + loopCorrection
			}
		}

		// Call LLM and stream response.
		var response *llm.Response
		var streamErr error
		var reasoningBuf strings.Builder

		reasonClockStart := time.Time{}
		reasonClockEnd := time.Time{}
		// streamedAny records that a chunk of any kind reached the client
		// from this call: a limit reported after that is never waited for
		// and re-issued, whatever the provider's own error carries.
		streamedAny := false
		// answerBuf is the answer text that reached the client from this call:
		// what a provider failure mid-answer keeps (keepInterruptedAnswer),
		// since the reader may hand back no response once the stream broke.
		var answerBuf strings.Builder
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

		// Cancel the stream if no output arrives before the silent-start guard
		// (agent.llm_first_token_timeout_ms). A model configured with stream: false
		// produces nothing until the whole completion is ready, so the guard would cut
		// every slow blocking answer: it is not armed for that transport, and the turn
		// context remains the bound. An explicit 0 disables the guard for streaming
		// too. firstTokenTimedOut records that this timer, and not the user or the loop
		// guard, did the cancelling, which the error paths below cannot otherwise tell
		// apart.
		firstTokenTimeout := a.cfg.Agent.EffectiveLLMFirstTokenTimeout()
		if retryAllowance == nil {
			retryAllowance = llm.NewRetryAllowance(a.cfg.Agent.EffectiveLLMRetryMax())
		}
		attemptsBefore := retryAllowance.Snapshot()
		callReason := nextCallReason
		nextCallReason = "step"
		// Every guard cancels with its own cause, so the network trace of the request
		// (debug.enable) names who cut it rather than reporting a bare cancel.
		llmCalls++
		streamCtx, cancelStream := context.WithCancelCause(llm.WithNetTraceAttrs(llm.WithRetryAllowance(ctx, retryAllowance),
			"session", sessionID, "turn", turn, "call", llmCalls))
		streamCancel := func() { cancelStream(nil) }
		var firstTokenTimedOut atomic.Bool
		// Whether any delta reached the client during this attempt. Atomic because a
		// transport is free to deliver chunks from its own goroutine.
		var sawDelta atomic.Bool
		// Calls announced by name only (llm.StreamChunk.ToolCallNamed). An
		// announcement does not count as output, so the provider may replay a
		// stream that died under it, and the replay names the call under a new
		// id. Whatever was announced and never arrived is retracted below.
		var namedMu sync.Mutex
		namedOnly := map[string]string{}
		var firstTokenTimer *time.Timer
		if transport.streaming && firstTokenTimeout > 0 {
			firstTokenTimer = time.AfterFunc(firstTokenTimeout, func() {
				firstTokenTimedOut.Store(true)
				cancelStream(errFirstTokenCut)
			})
		}
		stopFirstTokenTimer := func() {
			if firstTokenTimer != nil {
				firstTokenTimer.Stop()
			}
		}

		// The mid-stream stall guard (agent.llm_stream_idle_timeout_ms) is the
		// provider's own (llm.WithStreamIdleGuard, applied in getProvider): a
		// saturated hub can deliver thousands of frames and then stop mid-answer
		// with no finish_reason, no [DONE] and the connection held open, and the
		// guard cuts that stream with an error llm.IsStreamStalled recognises.
		streamIdle := time.Duration(0)
		if transport.streaming {
			streamIdle = a.cfg.Agent.EffectiveLLMStreamIdleTimeout()
		}
		// noteProgress is the single point for every sign of life, delivered or
		// not. It retires the first-token guard: a model streaming one large tool
		// call delivers nothing the caller can see - tool-call arguments are
		// accumulated inside the reader - and the silent-start timer would otherwise
		// cut a perfectly healthy stream.
		var firstProgress atomic.Int64
		noteProgress := func(now time.Time) {
			stopFirstTokenTimer()
			firstProgress.CompareAndSwap(0, now.UnixNano())
			// The provider is delivering again, so the "parked" label the client is
			// showing over the frozen bubble has to go before the answer resumes under
			// it. Every live channel routes through here - text, reasoning, tool calls
			// and bare progress frames - so this is the one place that sees it.
			if announcedPark.CompareAndSwap(true, false) {
				_ = a.server.SendSessionUpdate(sessionID, acp.LLMRetryUpdate{
					SessionUpdate: acp.UpdateTypeLLMRetry,
					Phase:         acp.LLMRetryPhaseResumed,
					Attempt:       stallContinues,
				})
			}
		}

		// One detector per streamed channel: a degenerating thinking channel burns
		// exactly as many tokens as visible text while showing nothing in the
		// transcript. Tripping cancels the stream the same way the first-token timer
		// does; the branch after the call decides whether to nudge or stop.
		textLoop := newStreamRepeatDetector(streamRepeatCycles)
		reasonLoop := newStreamRepeatDetector(streamRepeatCycles)
		loopAbort := loopAbortNone

		emitReason := func(d string, now time.Time) {
			noteProgress(now)
			sawDelta.Store(true)
			streamedAny = true
			reasoningBuf.WriteString(d)
			a.progress.streamed(d)
			// The clock measures wall time between the first reasoning delta and the
			// first answer text. A blocking response replays both back to back once
			// generation has already finished, so the only honest reading is none:
			// leaving the clock unset omits reasoning_duration_ms instead of
			// persisting a fabricated millisecond.
			if transport.streaming && reasonClockStart.IsZero() {
				reasonClockStart = now
			}
			_ = a.server.SendSessionUpdate(sessionID, acp.MessageChunkUpdate{
				SessionUpdate: acp.UpdateTypeAgentMessageChunk,
				Content:       acp.ContentBlock{Type: acp.ContentTypeReasoning, Text: d},
			})
		}
		emitText := func(delta string, now time.Time, markReasonEnd bool) {
			noteProgress(now)
			turnHadVisibleText = true
			sawDelta.Store(true)
			streamedAny = true
			answerBuf.WriteString(delta)
			if markReasonEnd && strings.TrimSpace(delta) != "" {
				maybeMarkReasonEnd(now)
			}
			a.progress.streamed(delta)
			_ = a.server.SendSessionUpdate(sessionID, acp.MessageChunkUpdate{
				SessionUpdate: acp.UpdateTypeAgentMessageChunk,
				Content:       acp.ContentBlock{Type: acp.ContentTypeText, Text: delta},
			})
		}

		if forceAnswer {
			// LLM-facing only; never persisted to the transcript, the same way the
			// loop-guard and empty-turn nudges are. Appended after the compaction
			// rebuild above so it cannot be swallowed by one.
			directive, why := loopAnswerDirective, "loop guard asked for an answer with tools withheld"
			if restartForceAnswer {
				directive = restartAnswerDirective
				why = "stall guard asked for an answer with tools withheld after repeated restarts"
				restartForceAnswer = false
			}
			messages = append(messages, llm.Message{Role: llm.RoleUser, Content: directive})
			blockedRounds = 0
			a.log.Warn(why, "turn", turn)
		}

		// Prune only the provider projection. The working slice and persisted
		// transcript retain full tool results.
		sendMessages := withTurnContext(a.prunedForLLM(messages), turnCtx)
		if forceAnswer && len(sendMessages) > 0 && sendMessages[0].Role == llm.RoleSystem {
			// One request with the tools withheld: its system prompt must not
			// describe tools the model cannot call. Rendered for this request only
			// and never written back, so the frozen message - and the provider's
			// cache of the turn - is still there if the turn goes on. The turn is
			// about to end, so a correct forced answer is worth one cache miss.
			noTools := a.buildSystemPromptParts(mode, activeSkills, nil, contextFiles)
			sendMessages = append([]llm.Message(nil), sendMessages...)
			sendMessages[0].Content = noTools.Content
		}
		a.emitDebug(turn, "llm_request", "", "", map[string]interface{}{
			"model":    a.state.EffectiveModelID(a.cfg),
			"messages": len(sendMessages),
			"tools":    len(callDefs),
		})
		callStart := time.Now()
		a.log.Debug("llm call started",
			"session", sessionID, "turn", turn, "call", llmCalls,
			"model", a.state.EffectiveModelID(a.cfg),
			"streaming", transport.streaming,
			"messages", len(sendMessages), "tools", len(callDefs),
			"first_token_timeout", firstTokenTimeout, "stream_idle_timeout", streamIdle)
		a.progress.beginCall()
		inputProgress := make(map[string]*toolInputProgress)
		response, streamErr = transport.provider.Stream(streamCtx, sendMessages, callDefs, func(chunk llm.StreamChunk) {
			if streamCtx.Err() != nil {
				return
			}
			now := time.Now()
			if chunk.Progress {
				// Carries no content by construction: it exists only to say the
				// model is still working, so it re-arms the guards and stops here.
				noteProgress(now)
				return
			}
			if chunk.ReasoningDelta != "" {
				emitReason(chunk.ReasoningDelta, now)
				if _, tripped := reasonLoop.Add(chunk.ReasoningDelta); tripped && loopAbort == loopAbortNone {
					loopAbort = loopAbortReasoning
					cancelStream(errLoopGuardCut)
					return
				}
			}
			if chunk.TextDelta != "" {
				if _, tripped := textLoop.Add(chunk.TextDelta); tripped && loopAbort == loopAbortNone {
					loopAbort = loopAbortText
					// Emit this last delta with the fork's own markReasonEnd rule so a
					// whitespace-only chunk does not close the reasoning clock.
					emitText(chunk.TextDelta, now, strings.TrimSpace(chunk.TextDelta) != "")
					cancelStream(errLoopGuardCut)
					return
				}
			}
			if chunk.TextDelta != "" && strings.TrimSpace(chunk.TextDelta) != "" {
				emitText(chunk.TextDelta, now, true)
			} else if chunk.TextDelta != "" {
				emitText(chunk.TextDelta, now, false)
			}
			// A call the model has only named yet announces the same pending row:
			// the arguments can take seconds to stream, and without the row the
			// transcript stands still with nothing but a Stop button. The row is
			// keyed by the call id, so the complete call updates it in place.
			if tc := chunk.ToolCallDelta; tc != nil {
				streamedAny = true
				stopFirstTokenTimer()
				p := inputProgress[tc.ID]
				if p == nil {
					p = newToolInputProgress(tc.Name)
					inputProgress[tc.ID] = p
				}
				p.add(tc.InputJSON)
				a.progress.streamed(tc.InputJSON)
				if u := p.update(tc.ID, now, false); u != nil {
					_ = a.server.SendSessionUpdate(sessionID, *u)
				}
			}
			if tc := chunk.ToolCall; tc != nil {
				// Providers without argument deltas still contribute the full count.
				// Reconcile the fallback estimate once, never count fragments twice.
				p := inputProgress[tc.ID]
				if p == nil {
					a.progress.streamed(tc.Name + tc.InputJSON)
				} else {
					missing := utf8.RuneCountInString(tc.InputJSON) - p.argumentRunes
					if missing > 0 {
						a.progress.streamedRunes(missing)
					}
					a.progress.streamed(tc.Name)
					if u := p.update(tc.ID, now, true); u != nil {
						_ = a.server.SendSessionUpdate(sessionID, *u)
					}
				}
			}
			announce := chunk.ToolCall
			if announce == nil {
				announce = chunk.ToolCallNamed
			}
			if announce != nil && announce.Name != "" {
				if chunk.ToolCall == nil {
					namedMu.Lock()
					namedOnly[announce.ID] = announce.Name
					namedMu.Unlock()
				}
				maybeMarkReasonEnd(now)
				if st := sessionStatePtr(a.state); st != nil {
					if sd := strings.TrimSpace(st.GetPersistedSessionDir()); sd != "" && strings.TrimSpace(announce.ID) != "" {
						_ = session.WriteToolCallMeta(sd, announce.ID, session.ToolCallMeta{
							ToolCallID: strings.TrimSpace(announce.ID),
							Name:       announce.Name,
							Kind:       toolKind(announce.Name),
							Status:     "pending",
						})
					}
				}
				_ = a.server.SendSessionUpdate(sessionID, acp.ToolCallUpdate{
					SessionUpdate: acp.UpdateTypeToolCall,
					ToolCallID:    announce.ID,
					Title:         announce.Name, // plain name, no "Calling: " prefix
					Kind:          toolKind(announce.Name),
					Status:        "pending",
				})
				noteProgress(now)
			}
		})
		if streamErr != nil {
			status := "failed"
			if streamCtx.Err() != nil {
				status = "cancelled"
			}
			// Every call that streamed arguments announced a pending row; the
			// stream died before any of them could execute, so none may stay
			// pending. Calls that only announced a name keep their row - the
			// same gap as before argument deltas existed.
			for id := range inputProgress {
				_ = a.server.SendSessionUpdate(sessionID, acp.ToolCallStatusUpdate{
					SessionUpdate: acp.UpdateTypeToolCallUpdate, ToolCallID: id, Status: status,
				})
			}
		}
		stopFirstTokenTimer()
		streamCancel()
		namedMu.Lock()
		a.retractUnarrivedToolCalls(sessionID, namedOnly, response)
		namedMu.Unlock()

		// One auditable copy of the emitted contract every branch below depends on:
		// a call that delivered nothing may be replayed, a call that delivered
		// anything never may.
		//
		// sawDelta is load-bearing and not redundant with response: a transport
		// failure mid-stream returns no response at all, yet the deltas it managed
		// to send are already on the user's screen. Judging by response alone would
		// replay those and show the same text twice.
		hasAnyOutput := sawDelta.Load() || (response != nil && (strings.TrimSpace(response.Content) != "" ||
			len(response.ToolCalls) > 0 || strings.TrimSpace(reasoningBuf.String()) != ""))
		a.logLLMCallFinished(sessionID, turn, llmCalls, callStart, firstProgress.Load(), hasAnyOutput,
			context.Cause(streamCtx), streamErr, response,
			"call_reason", callReason, "provider_attempts", retryAllowance.Snapshot().Attempts-attemptsBefore.Attempts,
			"transport_retries", retryAllowance.Snapshot().TransportRetries-attemptsBefore.TransportRetries, "retries_remaining", retryAllowance.Snapshot().Remaining)
		// The provider's stall guard cut this stream (agent.llm_stream_idle_timeout_ms).
		stalled := llm.IsStreamStalled(streamErr)
		stallIdle := llm.StreamStalledIdle(streamErr)

		// The loop guard cancelled this stream: keep the useful part of the answer,
		// drop the repeated run so it is never replayed to the model, and either nudge
		// the model back on track or stop the turn with a notice. Checked before the
		// generic cancellation handling below, which cannot tell a guard abort from a
		// user Stop. A real cancellation racing the guard wins: the user asked to stop,
		// so the turn must not be re-prompted.
		if loopAbort != loopAbortNone && ctx.Err() == nil && !a.state.IsUserCancelledTurn() {
			a.persistLoopAbortedMessage(response, &reasoningBuf, reasonClockStart, reasonClockEnd, streamRepeatCycles)
			if loopNudges >= loopNudgeBudget {
				return string(acp.StopReasonRefused), loopAbortError(loopAbort)
			}
			loopNudges++
			messages = a.buildMessages(sys.Content)
			nudge := streamLoopNudge
			if loopAbort == loopAbortReasoning {
				nudge = reasoningLoopNudge
			}
			// LLM-facing only; never persisted to the transcript.
			messages = append(messages, llm.Message{Role: llm.RoleUser, Content: nudge})
			a.log.Warn("loop guard cut a degenerating response",
				"channel", loopAbortChannelName(loopAbort), "nudge", loopNudges)
			resetRetries("loop_guard")
			continue
		}

		// The stall guard cut this stream: the provider stopped sending data
		// mid-answer. Keep everything the user already watched arrive, then ask the
		// model to carry on from it. Checked after the loop-guard branch above,
		// because a stream that degenerates and then stalls must take that path -
		// only it strips the repeated tail before persisting, and a loop left in
		// the transcript re-seeds itself on the very next request.
		//
		// A stall that produced nothing visible falls through instead: there is
		// nothing to continue from, so it is handled as a silent call below and the
		// identical request is replayed.
		if stalled && loopAbort == loopAbortNone && ctx.Err() == nil && !a.state.IsUserCancelledTurn() {
			if a.persistStalledMessage(response, &reasoningBuf, reasonClockStart, reasonClockEnd) {
				// fork(continue-path): the partial answer is kept either way; with
				// agent.llm_continue off, or its budget at 0, the turn ends here and
				// says why, which is what upstream 1.1.47 always does.
				continueMax := a.cfg.Agent.EffectiveLLMContinueMax()
				if !a.cfg.Agent.LLMContinueEnabled() || continueMax == 0 {
					return string(acp.StopReasonRefused), continueOffError(stallIdle)
				}
				// Has this attempt been here before? A hub that keeps dropping the same
				// answer otherwise gets the same polite "carry on" every time, and the
				// model answers it by starting over - which is the loop the operator
				// watches: one identical opening paragraph per attempt.
				partialContent := ""
				var partialCalls []llm.ToolCall
				if response != nil {
					partialContent = response.Content
					partialCalls = response.ToolCalls
				}
				seen, repeated := attemptRepeats.Observe(
					attemptFingerprint(reasoningBuf.String(), partialContent, partialCalls))
				if repeated {
					attemptRestarts++
				}
				a.emitDebug(turn, "stream_stall", "", "", map[string]interface{}{
					"idle":         stallIdle.String(),
					"continuation": stallContinues + 1,
					"restarts":     attemptRestarts,
				})
				// fork(continue-budget): one budget per turn, agent.llm_continue_max.
				if stallContinues >= continueMax {
					return string(acp.StopReasonRefused), stallAbortError(stallIdle, stallContinues, attemptRestarts)
				}
				stallContinues++
				retryAllowance = nil
				nextCallReason = "continuation"
				recoveryTurns++
				// Which nudge: "carry on from the message above" is only honest the first
				// time, and only when there is visible text above to carry on from.
				nudge := streamStallNudge
				switch {
				case repeated:
					nudge = repeatedAttemptNudge(alreadyRanSteps(a.state.GetMessages(), alreadyRanStepsMax), seen)
					if attemptRestarts >= attemptRestartsBeforeAnswer {
						restartForceAnswer = true
					}
				case strings.TrimSpace(partialContent) == "":
					nudge = stallNoTextNudge
				}
				pause := continueDelay(a.cfg.Agent.EffectiveLLMContinueStallDelays(), stallContinues)
				a.log.Warn("provider stopped sending data mid-answer; continuing",
					"idle", stallIdle, "continuation", stallContinues, "restarts", attemptRestarts, "pause", pause)
				// Tell the client the turn is parked. It is still showing the half-written
				// answer as if it were arriving, and the row it renders the live status in
				// is hidden for as long as a bubble streams - so without this the wait
				// looks exactly like a turn that died.
				announcedPark.Store(true)
				_ = a.server.SendSessionUpdate(sessionID, acp.LLMRetryUpdate{
					SessionUpdate: acp.UpdateTypeLLMRetry,
					Phase:         acp.LLMRetryPhaseContinuing,
					Attempt:       stallContinues,
					DelayMS:       pause.Milliseconds(),
				})
				if err := sleepCtx(ctx, pause); err != nil {
					if a.state.IsUserCancelledTurn() {
						return string(acp.StopReasonCancelled), nil
					}
					return string(acp.StopReasonRefused), fmt.Errorf("%w (the pause before carrying on the answer was interrupted: %v)", stallAbortError(stallIdle, stallContinues-1, attemptRestarts), err)
				}
				messages = a.buildMessages(sys.Content)
				// LLM-facing only; never persisted to the transcript.
				messages = append(messages, llm.Message{Role: llm.RoleUser, Content: nudge})
				replaying = true
				continue
			}
		}

		// If the stream was cancelled by the first-token timer (no output produced, no user cancel),
		// surface a timeout error instead of a silent failure. The timer itself reports
		// that it fired, so a cancellation from anywhere else is never mislabelled.
		if ((firstTokenTimedOut.Load() && streamErr != nil && streamCtx.Err() != nil) || stalled) &&
			ctx.Err() == nil && !a.state.IsUserCancelledTurn() {
			if !hasAnyOutput {
				// Nothing reached the caller, so re-issuing the identical request
				// cannot duplicate anything.
				//
				// The cheap recovery goes first: behind one model name there is
				// usually a group of deployments, and a member that sends no first
				// byte fails that way per attempt, so the very next draw often
				// answers. Only when that has not helped does the stall ladder
				// below start waiting, which is the recovery a saturated gateway
				// needs. The iteration is repeated, not counted, either way.
				if stalls.enabled && firstTokenReissues < maxFirstTokenReissues && retryAllowance.TakeRetry() {
					firstTokenReissues++
					nextCallReason = "first_token_retry"
					a.log.Warn("no first token from the model; re-issuing the same request",
						"timeout", firstTokenTimeout, "attempt", firstTokenReissues)
					recoveryTurns++
					replaying = true
					continue
				}
				if retryAllowance.Snapshot().Remaining == 0 {
					return string(acp.StopReasonRefused), fmt.Errorf("model produced no reply: retry allowance exhausted")
				}
				// A saturated gateway is out for minutes, which is why this waits on
				// its own schedule rather than leaning on the seconds-scale retries
				// inside internal/llm.
				switch retry, stopped := a.waitForStalledProvider(ctx, &stalls, sessionID, "no output"); {
				case retry:
					recoveryTurns++
					replaying = true
					continue
				case stopped:
					return string(acp.StopReasonCancelled), nil
				}
				return string(acp.StopReasonRefused), fmt.Errorf("model did not respond (no output within %v)%s",
					firstTokenTimeout, stallGaveUpSuffix(&stalls))
			}
		}

		if streamErr != nil {
			// The provider named the moment its limit lifts and the operator
			// asked the turn to wait for it: the countdown reaches the client,
			// then the same call runs again (nothing was persisted for the
			// failed one, so nothing repeats). A cancel during the wait ends
			// the turn as a stop. The iteration is repeated, not counted.
			if reset, ok := a.limitResetToWaitFor(streamErr, response, reasoningBuf.String(), streamedAny); ok {
				waitStart := time.Now()
				err := a.waitForLimitReset(ctx, sessionID, reset)
				limitWait.Charge(time.Since(waitStart))
				if err != nil {
					if a.state.IsUserCancelledTurn() {
						return string(acp.StopReasonCancelled), nil
					}
					// A shutdown or a deadline, not the user: the turn ends with
					// the limit it was waiting on and says what cut the wait.
					return string(acp.StopReasonRefused), fmt.Errorf("LLM error: %w (the wait for the reset was interrupted: %v)", reset, err)
				}
				resetRetries("quota_reset_wait")
				turn--
				replaying = true
				continue
			}
			// A failure of the provider's lane after text was shown - a 5xx the
			// resilient wrapper could not ride out, a stream cut mid-answer -
			// carries the answer on (upstream 1.2.9, issue #246; see
			// provider_recovery.go for where the fork's version differs). A
			// stall with text shown took the branch above; a failure before any
			// text is the stall ladder's below; a limit (429) is the wrapper's
			// and the limit wait's.
			if hasAnyOutput && !stalled && ctx.Err() == nil && !a.state.IsUserCancelledTurn() &&
				a.cfg.Agent.LLMContinueEnabled() && llm.IsTransientProviderError(streamErr) {
				kept := a.keepInterruptedAnswer(answerBuf.String(), reasoningBuf.String(), reasonClockStart, reasonClockEnd)
				if stallContinues >= a.cfg.Agent.EffectiveLLMContinueMax() {
					return string(acp.StopReasonRefused), providerGaveUpError(streamErr, stallContinues)
				}
				seen, repeated := attemptRepeats.Observe(attemptFingerprint(reasoningBuf.String(), answerBuf.String(), nil))
				if repeated {
					attemptRestarts++
				}
				stallContinues++
				retryAllowance = nil
				nextCallReason = "continuation"
				recoveryTurns++
				pause := errorContinueDelay(&a.cfg.Agent, stallContinues-1, streamErr)
				a.log.Warn("provider failed mid-answer; carrying the answer on after a pause",
					"error", streamErr, "pause", pause, "continuation", stallContinues,
					"kept_partial_answer", kept, "restarts", attemptRestarts)
				// The same park the stall continuation announces: the client is
				// still showing the half-written answer as if it were arriving.
				announcedPark.Store(true)
				_ = a.server.SendSessionUpdate(sessionID, acp.LLMRetryUpdate{
					SessionUpdate: acp.UpdateTypeLLMRetry,
					Phase:         acp.LLMRetryPhaseContinuing,
					Attempt:       stallContinues,
					DelayMS:       pause.Milliseconds(),
				})
				if err := sleepCtx(ctx, pause); err != nil {
					if a.state.IsUserCancelledTurn() {
						return string(acp.StopReasonCancelled), nil
					}
					// A deadline or a shutdown, not the user: the turn ends
					// with the failure it was recovering from.
					return string(acp.StopReasonRefused), fmt.Errorf("LLM error: %w (the pause before carrying on the answer was interrupted: %v)", streamErr, err)
				}
				messages = a.buildMessages(sys.Content)
				nudge := providerRecoveryNudge
				switch {
				case repeated:
					nudge = repeatedAttemptNudge(alreadyRanSteps(a.state.GetMessages(), alreadyRanStepsMax), seen)
					if attemptRestarts >= attemptRestartsBeforeAnswer {
						restartForceAnswer = true
					}
				case !kept:
					nudge = stallNoTextNudge
				}
				// LLM-facing only; never persisted to the transcript.
				messages = append(messages, llm.Message{Role: llm.RoleUser, Content: nudge})
				replaying = true
				continue
			}
			// A mid-generation truncation keeps its partial answer like a user
			// stop: the user already watched the text stream in, so it must
			// survive in the transcript next to the honest error below.
			if (errors.Is(streamErr, context.Canceled) || llm.IsStreamTruncated(streamErr) || llm.IsStreamStalled(streamErr)) && response != nil {
				reasonTrim := strings.TrimSpace(reasoningBuf.String())
				hasText := strings.TrimSpace(response.Content) != ""
				// Tool calls are deliberately absent from what follows. A cancelled
				// stream is finalized mid-arguments, so the readers hand back calls
				// whose JSON was cut; and every tool_call_id an assistant message
				// announces must get a result, or the next request in this
				// conversation is rejected. Replaying an invalid call is worse than
				// losing it - the same choice openai_stream.go makes for a truncated
				// stream and persistStalledMessage makes for a stall. A response that
				// produced only a cut call therefore persists nothing at all.
				if hasText || reasonTrim != "" {
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
						ReasoningDurationMs: reasoningMs,
						Model:               a.state.EffectiveModelID(a.cfg),
						CreatedAt:           time.Now().UTC().Format(time.RFC3339),
					}
					a.state.AddMessage(assistantMsg)
					a.refreshConversationContextUsage(true)
				}
			}
			if errors.Is(streamErr, context.Canceled) {
				// A stopped turn is still a conversation the user opened, and the title is
				// built from their first message rather than from the answer - so it must
				// not depend on the turn running to completion. Launched before the returns
				// below, which are the paths a Stop actually takes.
				if allowTitleGen {
					a.startTitleGeneration(transport.provider)
				}
				// If output was already streamed, treat as a clean user-stop regardless.
				if hasAnyOutput || a.state.IsUserCancelledTurn() {
					return string(acp.StopReasonCancelled), nil
				}
				// Stream was interrupted before producing any output and the user did not stop it
				// (a signal, a shutdown): surface an error so the UI can show feedback instead of
				// silently completing, and say how long the model had been silent, because a
				// process killed after a long empty wait is a different story from one interrupted
				// at once.
				return string(acp.StopReasonRefused), fmt.Errorf("generation was interrupted before producing a response (the model had been silent for %s)", humanDuration(time.Since(callStart)))
			}
			if ctx.Err() != nil {
				// Context cancelled for non-context-Canceled stream error: still propagate the real error.
				return string(acp.StopReasonRefused), fmt.Errorf("LLM error: %w", streamErr)
			}
			// A failing endpoint that delivered nothing gets the same long schedule
			// as a silent one. internal/llm has already spent its own retries by
			// now, but those are seconds against an outage measured in minutes.
			if !hasAnyOutput && !a.state.IsUserCancelledTurn() && stallRetryableError(streamErr) {
				if !retryAllowance.TakeRetry() {
					return string(acp.StopReasonRefused), fmt.Errorf("LLM error: %w (retry allowance exhausted)", streamErr)
				}
				switch retry, stopped := a.waitForStalledProvider(ctx, &stalls, sessionID, "provider error"); {
				case retry:
					recoveryTurns++
					replaying = true
					continue
				case stopped:
					return string(acp.StopReasonCancelled), nil
				}
			}
			return string(acp.StopReasonRefused), fmt.Errorf("LLM error: %w%s", streamErr, stallGaveUpSuffix(&stalls))
		}

		// What the provider served from its prompt cache. A long conversation
		// only stays affordable while this is most of the input, which is what
		// the frozen system prompt and the trailing turn context block are for
		// (turn_context.go); a provider that reports nothing leaves it at zero.
		a.log.Debug("llm call usage",
			"input_tokens", response.InputTokens,
			"cached_input_tokens", response.CachedInputTokens,
			"output_tokens", response.OutputTokens)

		// Accumulate and broadcast token usage after each LLM call.
		totalInputTokens += response.InputTokens
		totalOutputTokens += response.OutputTokens
		lastInputTokens = response.InputTokens
		// All three counters are cumulative for this turn. Sending the last call's
		// input/output next to a cumulative total would make Input + Output != Total in
		// every client that renders the three together.
		_ = a.server.SendSessionUpdate(sessionID, acp.TokenUsageUpdate{
			SessionUpdate: acp.UpdateTypeTokenUsage,
			InputTokens:   totalInputTokens,
			OutputTokens:  totalOutputTokens,
			TotalTokens:   totalInputTokens + totalOutputTokens,
		})
		a.progress.finishCall(response.OutputTokens)

		if sd != "" {
			now := time.Now().UTC()
			if lastStatsWrite.IsZero() || now.Sub(lastStatsWrite) > 750*time.Millisecond {
				lastStatsWrite = now
				// Re-read rather than reuse a turn-start snapshot: compaction writes the
				// context breakdown into this same file while the turn runs, and a write
				// built on a stale document would revert it.
				prior, err := session.ReadSessionStats(sd)
				if err != nil {
					prior = nil
				}
				stats := session.ApplyTurnUsage(prior, sessionTurnIndex, session.TokenUsageTotals{
					InputTokens:  totalInputTokens,
					OutputTokens: totalOutputTokens,
				}, now)
				if rs, ok := a.state.(rulesState); ok {
					if b := rs.GetLastContextBreakdown(); b != nil {
						cp := *b
						stats.ContextBreakdown = &cp
					}
				}
				_ = session.WriteSessionStats(sd, stats)
			}
		}

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
		// response is non-nil on this path: the streamErr branch above returned, and the
		// token accounting plus assistantMsg below already dereference it unconditionally.
		a.emitDebug(turn, "llm_response", "", "", map[string]interface{}{
			"stop_reason":   response.StopReason,
			"input_tokens":  response.InputTokens,
			"output_tokens": response.OutputTokens,
			"tool_calls":    len(response.ToolCalls),
		})
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
		a.refreshConversationContextUsage(true)

		// After the first assistant response, generate a short session title off the hot path.
		// Only the fresh-prompt path titles; resume/continue turns never do.
		// Not gated on the loop index: a stall continuation, a loop-guard nudge or an
		// empty-answer re-prompt all push the first real answer past index 0, and
		// gating on it left those sessions untitled. titleOnce dedupes within the
		// turn and maybeGenerateTitleForConfig refuses to overwrite an existing
		// title, so calling it on every answer is both cheap and correct.
		if allowTitleGen {
			a.startTitleGeneration(transport.provider)
		}

		// If no tool calls, we're done — unless the model produced no visible answer at
		// all (empty content). Some models (notably gpt-oss / harmony endpoints) sometimes
		// end a turn with only internal reasoning — occasionally leaking a tool call into
		// the reasoning channel — emitting neither final content nor a tool_calls array.
		// Returning here would dead-end the conversation on a lone "thinking" bubble, so
		// re-prompt the model a bounded number of times before giving up.
		if len(response.ToolCalls) == 0 {
			// The recovery backstop also watches a resuming turn's answers: a
			// new answer resets the persisted repeat window, an unchanged one
			// earns a single loop correction and then stops the turn. A fresh
			// in-process turn stays with the loop guard above.
			if guardOn && recovering && strings.TrimSpace(response.Content) != "" {
				checkpoint.Observe(session.ObservationHash("assistant", strings.Join(strings.Fields(response.Content), " ")))
				if err := checkpoint.Save(sd); err != nil {
					return string(acp.StopReasonRefused), err
				}
				if checkpoint.Repeats >= 3 {
					checkpoint.Status = "no_progress"
					if err := checkpoint.Save(sd); err != nil {
						return string(acp.StopReasonRefused), err
					}
					return string(acp.StopReasonRefused), fmt.Errorf("no progress: repeated unchanged responses after a loop correction")
				}
				if checkpoint.Repeats == 2 {
					continue
				}
			}
			if response.StopReason == "max_tokens" {
				a.noteStopNotice(maxTokensNotice(a.effectiveMaxTokens(), response.OutputTokens))
				return string(acp.StopReasonMaxTokens), nil
			}
			if ctx.Err() != nil || a.state.IsUserCancelledTurn() {
				return string(acp.StopReasonCancelled), nil
			}
			// First recovery is the plain replay: drop the empty turn from the
			// LLM-facing slice so the request going out is byte for byte the one
			// that failed, and let the proxy hand it to another deployment. The
			// transcript keeps that turn, because the user watched its reasoning
			// stream in. Words come next, once a replay has not helped.
			if strings.TrimSpace(response.Content) == "" && emptyReissues < maxEmptyAssistantReissues &&
				len(messages) > 0 && messages[len(messages)-1].Role == llm.RoleAssistant && retryAllowance.TakeRetry() {

				emptyReissues++
				messages = emptyRecoveryProjection(messages, 0)
				nextCallReason = "empty_reissue"
				a.log.Warn("model answered with no text and no tool call; re-issuing the same request",
					"recovery", nextCallReason, "retries_remaining", retryAllowance.Snapshot().Remaining)
				replaying = true
				recoveryTurns++
				continue
			}
			if strings.TrimSpace(response.Content) == "" && emptyContinuations < maxEmptyAssistantContinuations && retryAllowance.TakeRetry() {

				emptyContinuations++
				messages = emptyRecoveryProjection(messages, 1)
				nextCallReason = "empty_nudge"
				recoveryTurns++
				a.log.Warn("model answered with no text and no tool call; nudging for an answer",
					"recovery", nextCallReason, "retries_remaining", retryAllowance.Snapshot().Remaining)
				continue
			}
			// The turn produced no visible answer at all (only reasoning / empty
			// content) and the model never recovered after the continuation nudges.
			// Surface a clear notice (rendered as a system message with a Retry
			// control in the UI) instead of dead-ending silently on a thinking-only
			// turn — otherwise the user sees no assistant reply. Seen with gpt-oss /
			// harmony endpoints that route the tool call through the reasoning channel.
			if strings.TrimSpace(response.Content) == "" && !turnHadVisibleText {
				return string(acp.StopReasonRefused), fmt.Errorf("model produced no reply: only internal reasoning, with no answer text or tool call")
			}
			// A Stop hook may send the agent back to work with a follow-up that
			// is submitted as the next user message (persisted, so the transcript
			// explains the continuation), bounded by hooks.stop_loop_limit.
			if followUp, again := a.runStopHooks(ctx, mode, response.Content, stopHookActive); again {
				limit := a.cfg.Hooks.EffectiveStopLoopLimit()
				if stopBlocks >= limit {
					a.log.Warn("stop hook loop limit reached; ending the turn", "limit", limit)
					return string(acp.StopReasonEndTurn), nil
				}
				// The follow-up needs an iteration to be read in; on the last one
				// it would only leave a dangling user message behind.
				if reactTurn+1 >= maxTurns {
					a.log.Warn("stop hook follow-up dropped: the turn cap is reached", "max_turns", maxTurns)
					return string(acp.StopReasonEndTurn), nil
				}
				stopBlocks++
				stopHookActive = true
				follow := llm.Message{
					Role:      llm.RoleUser,
					Content:   stopHookPrefix + followUp,
					CreatedAt: time.Now().UTC().Format(time.RFC3339),
				}
				messages = append(messages, follow)
				a.state.AddMessage(follow)
				a.refreshConversationContextUsage(true)
				resetRetries("stop_hook")
				continue
			}

			// The turn is about to end with an answer, and the Stop hooks have
			// had their say. Anything the operator queued while that answer was
			// being written is read now, so it is answered by this turn rather
			// than waiting for the next prompt. One iteration is needed to read
			// it in; on the last one it would only leave a dangling user
			// message behind, and the manager's boundary drain takes it.
			//
			// The last one is counted in the model's own steps: iterations spent
			// recovering from a provider do not shrink max_turns (see recoveryTurns).
			if reactTurn+1 < maxTurns && a.readQueuedMessages(&messages) {
				resetRetries("queued_followup")
				continue
			}
			return string(acp.StopReasonEndTurn), nil
		}

		// Compare a complete round, ignoring generated call IDs and announcement text.
		var observations []string
		// Execute all tool calls.
		executedThisRound, quarantinedThisRound := 0, 0
		for i, tc := range response.ToolCalls {
			if ctx.Err() != nil {
				return string(acp.StopReasonCancelled), nil
			}

			// Already taken away earlier in this turn: answer the call without
			// running it. This gate has to sit on the execution path rather than in
			// the tool definitions, because executeToolCall never consults them - the
			// same reason toolCallRefusedByMode exists (see toolsets.go).
			if a.isQuarantined(canonicalToolCallKey(tc.Name, tc.InputJSON)) {
				quarantinedThisRound++
				a.recordSkippedToolCalls(&messages, response.ToolCalls[i:i+1], toolQuarantinedResult)
				continue
			}

			// A model stuck on the identical call (same name, same canonical arguments),
			// or rotating through a fixed sequence of them, would otherwise burn the
			// whole max_turns budget without an answer. Skip the execution and tell it
			// so; every tool_call_id still gets a result, because OpenAI-compatible
			// endpoints reject the next request otherwise.
			//
			// Both detectors observe every requested call before either branch runs, so
			// a call the repeat detector skips still lands in the cycle history. The
			// repeat detector goes first: period one is its shape, and it trips sooner.
			_, repeatTripped := toolRepeats.Observe(tc.Name, tc.InputJSON)
			cyclePeriod, cycleUnit, cycleTripped := toolCycles.Observe(tc.Name, tc.InputJSON)
			switch {
			case repeatTripped && loopNudges >= loopNudgeBudget:
				if stuckAction == config.AgentLoopStuckActionStop {
					a.recordSkippedToolCalls(&messages, response.ToolCalls[i:], toolLoopSkippedResult)
					return string(acp.StopReasonRefused), fmt.Errorf(
						"stopped: the model kept requesting the same %s call with identical arguments", tc.Name)
				}
				a.log.Warn("loop guard quarantined a repeated tool call", "tool", tc.Name)
				if !a.quarantineLoop(tc, nil) {
					quarantinedThisRound++
					a.recordSkippedToolCalls(&messages, response.ToolCalls[i:i+1], toolQuarantinedResult)
					continue
				}
				// read/grep fall through: one last execution, pinned, so the model
				// finally holds stable content instead of being told no.
			case repeatTripped:
				loopNudges++
				// The counter deliberately keeps running: clearing it here (as Roo does,
				// where the trip is a blocking question to the user) would let the model
				// execute the same call limit-1 more times per nudge. A genuinely
				// different call resets the counter on its own.
				a.log.Warn("loop guard blocked a repeated tool call", "tool", tc.Name, "nudge", loopNudges)
				a.recordSkippedToolCalls(&messages, response.ToolCalls[i:i+1], toolLoopNudge)
				continue
			case cycleTripped && loopNudges >= loopNudgeBudget:
				a.emitDebug(turn, "loop_guard", "tool_cycle", tc.Name, map[string]interface{}{
					"tool": tc.Name, "period": cyclePeriod, "action": stuckAction,
				})
				if stuckAction == config.AgentLoopStuckActionStop {
					a.recordSkippedToolCalls(&messages, response.ToolCalls[i:], toolCycleSkippedResult)
					return string(acp.StopReasonRefused), errors.New(toolCycleStopNotice)
				}
				a.log.Warn("loop guard quarantined a tool-call cycle",
					"tool", tc.Name, "period", cyclePeriod, "calls", len(cycleUnit))
				if !a.quarantineLoop(tc, cycleUnit) {
					quarantinedThisRound++
					a.recordSkippedToolCalls(&messages, response.ToolCalls[i:i+1], toolQuarantinedResult)
					continue
				}
			case cycleTripped:
				a.emitDebug(turn, "loop_guard", "tool_cycle", tc.Name, map[string]interface{}{
					"tool": tc.Name, "period": cyclePeriod, "nudge": loopNudges,
				})
				loopNudges++
				a.log.Warn("loop guard blocked a repeating tool-call cycle",
					"tool", tc.Name, "period", cyclePeriod, "nudge", loopNudges)
				a.recordSkippedToolCalls(&messages, response.ToolCalls[i:i+1], toolCycleNudge)
				continue
			}
			executedThisRound++

			result, execErr := a.executeToolCall(ctx, tc, toolEnv, mode, a.state.GetID(), false, turn)

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
			a.refreshConversationContextUsage(true)
			checkpoint.LastTool = tc.Name
			observations = append(observations, session.ObservationHash(tc.Name, tc.InputJSON, toolResultMsg.Content))
		}
		checkpoint.Observe(session.ObservationHash(observations...))
		if err := checkpoint.Save(sd); err != nil {
			return string(acp.StopReasonRefused), err
		}
		// The deterministic repetition verdict is a recovery backstop only: it
		// judges a turn that resumed an interrupted or limited execution, whose
		// persisted window already shows the repeats. A fresh in-process turn is
		// the loop guard's jurisdiction (nudges, quarantine, the cycle notice).
		if guardOn && recovering && checkpoint.Repeats >= 3 {
			checkpoint.Status = "no_progress"
			if err := checkpoint.Save(sd); err != nil {
				return string(acp.StopReasonRefused), err
			}
			return string(acp.StopReasonRefused), fmt.Errorf("no progress: repeated unchanged tool results after a loop correction")
		}
		// The model folded its own history: the transcript the loop replays is
		// shorter now, so the outgoing slice is rebuilt from it before the next
		// call, the way an automatic compaction between steps rebuilds it. Done
		// after the whole batch, so a tool result already appended is picked up
		// from the transcript rather than dropped.
		if toolEnv.ContextCompacted {
			toolEnv.ContextCompacted = false
			messages = a.buildMessages(sys.Content)
			turnCtx = a.buildTurnContext(sys)
			a.refreshContextBreakdown(sys, turnCtx)
		}
		if toolEnv.ConfigReloaded {
			// config_commit or config_rollback replaced the live configuration.
			// Refresh everything the next model call depends on, at the safe
			// boundary between two calls rather than mid-flight.
			activeSkills = FilterSkillsForContext(a.state.GetSkills(), contextFiles)
			toolDefs = a.currentToolDefinitions(mode)
			toolEnv.PermissionMode = effectivePermMode(a.state, a.cfg)
			toolEnv.CommandAllowlist = append([]string(nil), a.cfg.Tools.CommandAllowlist...)
			toolEnv.HTTPAllowlist = append([]string(nil), a.cfg.Tools.HTTPRequest.Allowlist...)
			toolEnv.SSHConnectTimeout = a.cfg.Tools.SSHConnectTimeout
			toolEnv.OutputLineLimits = a.cfg.Tools.OutputLimits.AsMap()
			toolEnv.Background = a.backgroundPool(sd)
			toolEnv.BackgroundEnabled = a.cfg.Tools.Background.ResolvedEnabled()
			toolEnv.WebSearch = webSearchSettings(a.cfg)
			toolEnv.PreviewServer = previewServerSettings(a.cfg)
			toolEnv.ConfigReloaded = false
			// The frozen system message described the configuration that was just
			// replaced: its tool section, the skills catalogue, the response
			// language. Upstream leaves it as the turn rendered it; here it is
			// rendered again, because a model told about tools it no longer has
			// (or not told about the ones it got) is worse than one cache miss on
			// a step that happens once per configuration change.
			sys = a.buildSystemPromptParts(mode, activeSkills, toolDefs, contextFiles)
			if len(messages) > 0 && messages[0].Role == llm.RoleSystem {
				messages[0].Content = sys.Content
			}
		}
		// A round in which the model asked only for quarantined calls and executed
		// nothing means it has nothing left but the loop. Counted consecutively, the
		// same way emptyContinuations is: one executed call is progress and clears it.
		if quarantinedThisRound > 0 && executedThisRound == 0 {
			blockedRounds++
		} else {
			blockedRounds = 0
		}

		// The model made progress (executed tool calls), so reset the empty-turn counter. The
		// give-up notice is for CONSECUTIVE stalls (no answer and no tool call), not for a slow
		// multi-step task that keeps acting between reasoning-only thoughts — otherwise a model
		// that alternates thinking and tool calls (gpt-oss / harmony) is abandoned mid-task.
		// The replay budgets follow the same rule: a lane that answered once earns a
		// fresh one.
		resetRetries("step")

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

	// The step limit ends the turn like a finished answer would, so say it
	// (upstream 1.2.9, issue #255).
	a.noteStopNotice(a.maxTurnsNotice(maxTurns))
	return string(acp.StopReasonMaxTurns), nil
}

// persistLoopAbortedMessage stores the partial assistant message from a stream the
// loop guard cut, with the repeated run removed from both the answer text and the
// reasoning. Trimming here is what keeps the loop out of the context: buildMessages
// replays the transcript from session state, so a looped passage left in place would
// be fed straight back to the model on the nudge call (and to the compaction
// summarizer, and to every later turn) and would immediately re-seed the loop.
func (a *Agent) persistLoopAbortedMessage(
	response *llm.Response,
	reasoningBuf *strings.Builder,
	reasonClockStart, reasonClockEnd time.Time,
	minCycles int,
) {
	content := ""
	if response != nil {
		content = response.Content
	}
	content, _ = trimRepeatedTail(content, minCycles)

	// Trim the raw buffer before trimming whitespace: dropping a trailing space
	// first would truncate the last cycle and misalign the repeat detection.
	reasonRaw := reasoningBuf.String()
	reasonRaw, reasonCut := trimRepeatedTail(reasonRaw, minCycles)
	reasonTrim := strings.TrimSpace(reasonRaw)

	var reasonStore, reasonSig string
	if reasonCut {
		// The Anthropic signature only validates against the exact reasoning text,
		// so a trimmed block must be replayed unsigned.
		reasonStore, reasonSig = reasonTrim, ""
	} else {
		reasonStore, reasonSig = reasoningForStorage(reasonTrim, reasonRaw, response)
	}

	// Tool calls are dropped rather than persisted. The guard cancelled this stream,
	// so the readers finalized it mid-arguments and the calls may carry cut JSON;
	// worse, the caller rebuilds the payload from session state and re-prompts
	// immediately, so an unanswered tool_call_id here breaks the very next request
	// of this same turn. Same rule as persistStalledMessage and the truncation path
	// in openai_stream.go.
	if strings.TrimSpace(content) == "" && strings.TrimSpace(reasonStore) == "" {
		return
	}

	var reasoningMs int64
	if reasonTrim != "" && !reasonClockStart.IsZero() {
		end := reasonClockEnd
		if end.IsZero() {
			end = time.Now()
		}
		if d := end.Sub(reasonClockStart); d > 0 {
			reasoningMs = d.Milliseconds()
		}
	}

	a.state.AddMessage(llm.Message{
		Role:                llm.RoleAssistant,
		Content:             content,
		Reasoning:           reasonStore,
		ReasoningSignature:  reasonSig,
		ReasoningDurationMs: reasoningMs,
		Model:               a.state.EffectiveModelID(a.cfg),
		CreatedAt:           time.Now().UTC().Format(time.RFC3339),
	})
	a.refreshConversationContextUsage(true)
}

// recordSkippedToolCalls answers tool calls the loop guard refused to execute.
// Every tool_call_id an assistant message announced must get a result, otherwise
// OpenAI-compatible endpoints reject the next request in the conversation.
func (a *Agent) recordSkippedToolCalls(messages *[]llm.Message, calls []llm.ToolCall, reason string) {
	for _, tc := range calls {
		msg := llm.Message{
			Role:       llm.RoleTool,
			Content:    reason,
			ToolCallID: tc.ID,
		}
		*messages = append(*messages, msg)
		a.state.AddMessage(msg)
		_ = a.server.SendSessionUpdate(a.state.GetID(), acp.ToolCallStatusUpdate{
			SessionUpdate: acp.UpdateTypeToolCallUpdate,
			ToolCallID:    tc.ID,
			Status:        "cancelled",
			Content: []acp.ToolCallResultItem{
				{Type: "content", Content: acp.ContentBlock{Type: "text", Text: reason}},
			},
		})
	}
	a.refreshConversationContextUsage(true)
}

// retractUnarrivedToolCalls closes the pending rows of calls that were announced
// by name while their arguments streamed and then never arrived: the stream was
// cut, or the provider replayed it and the model named the call under another
// id. Nothing was executed or persisted for them, so the row is all there is to
// take back.
func (a *Agent) retractUnarrivedToolCalls(sessionID string, named map[string]string, response *llm.Response) {
	if len(named) == 0 {
		return
	}
	arrived := map[string]bool{}
	if response != nil {
		for _, tc := range response.ToolCalls {
			arrived[tc.ID] = true
		}
	}
	for id, name := range named {
		if arrived[id] {
			continue
		}
		if st := sessionStatePtr(a.state); st != nil {
			if sd := strings.TrimSpace(st.GetPersistedSessionDir()); sd != "" && strings.TrimSpace(id) != "" {
				_ = session.WriteToolCallMeta(sd, id, session.ToolCallMeta{
					ToolCallID: strings.TrimSpace(id),
					Name:       name,
					Kind:       toolKind(name),
					Status:     "cancelled",
				})
			}
		}
		_ = a.server.SendSessionUpdate(sessionID, acp.ToolCallStatusUpdate{
			SessionUpdate: acp.UpdateTypeToolCallUpdate,
			ToolCallID:    id,
			Status:        "cancelled",
		})
	}
}

// loopAbortChannelName labels the streamed channel that looped, for logs.
func loopAbortChannelName(c loopAbortChannel) string {
	if c == loopAbortReasoning {
		return "reasoning"
	}
	return "text"
}

// loopAbortError is the notice surfaced when a turn keeps looping after every
// nudge. The session manager records it as a UI log entry with a Retry control.
func loopAbortError(c loopAbortChannel) error {
	if c == loopAbortReasoning {
		return fmt.Errorf("stopped: the model kept repeating the same reasoning without reaching an answer")
	}
	return fmt.Errorf("stopped: the model kept repeating the same output instead of finishing the task")
}

// executeToolCall runs a single tool call and reports updates to the client.
func (a *Agent) executeToolCall(ctx context.Context, tc llm.ToolCall, env *tools.Env, mode, sessionID string, skipPermission bool, turn int) (string, error) {
	env.ToolCallID = strings.TrimSpace(tc.ID)
	a.currentToolCallID = env.ToolCallID
	defer func() {
		env.ToolCallID = ""
		a.currentToolCallID = ""
	}()

	// Touching a directory pulls its nested AGENTS.md into the prompt. Done up
	// front so it holds regardless of the outcome below (permission denial,
	// tool error), and so both callers — the ReAct loop and the resume-after-
	// permission path — are covered without threading state through.
	a.activateScopedRulesForToolCall(tc.Name, tc.InputJSON, env.CWD)

	sessionDir := ""
	if st := sessionStatePtr(a.state); st != nil {
		sessionDir = strings.TrimSpace(st.GetPersistedSessionDir())
	}

	// The persisted arguments are what a permission answered later resumes
	// on, so a write that fails is remembered and cancels the call before
	// any prompt instead of leaving a resume nothing trustworthy to run.
	var argsPersistErr error
	if sessionDir != "" && strings.TrimSpace(tc.ID) != "" {
		_ = session.MarkToolCallStarted(sessionDir, tc.ID, tc.Name, toolKind(tc.Name), "in_progress")
		argsPersistErr = session.WriteToolCallArgs(sessionDir, tc.ID, tc.InputJSON)
	}
	a.emitDebug(turn, "tool_start", tc.Name, "", map[string]interface{}{"tool_call_id": tc.ID, "kind": toolKind(tc.Name)})

	// Mark as in_progress, include raw InputJSON so connected clients can show args.
	_ = a.server.SendSessionUpdate(sessionID, acp.ToolCallStatusUpdate{
		SessionUpdate: acp.UpdateTypeToolCallUpdate,
		ToolCallID:    tc.ID,
		Status:        "in_progress",
		Content: []acp.ToolCallResultItem{
			{Type: "content", Content: acp.ContentBlock{Type: "text", Text: tc.InputJSON}},
		},
	})

	// A child may only call what its effective tool set admits: the
	// advertised definitions are filtered the same way, so this catches a
	// hallucinated or replayed call, MCP tools included, before anything runs
	// or asks for permission. The refusal goes through the same bookkeeping
	// as every other outcome, so the child's transcript records it as failed.
	if a.subagent != nil && !a.subagentAllows(tc.Name) {
		reason := fmt.Sprintf("tool %s is not available to this subagent", tc.Name)
		a.finishToolCall(sessionDir, sessionID, tc, reason, nil, "failed")
		return "", fmt.Errorf("%s", reason)
	}

	// A restricted mode filters tool definitions before the LLM sees them, but a
	// call replayed from history can still name a hidden tool; refuse it here so
	// the mode boundary holds at execution time too.
	if refusal, refused := toolCallRefusedByMode(mode, tc.Name, a.cfg.Tools.PlanNoSelfRunEnabled()); refused {
		a.finishToolCall(sessionDir, sessionID, tc, refusal, nil, "cancelled")
		return refusal, nil
	}

	// Operator hooks see the call before the permission gate, whatever the
	// permission mode: a hook can deny it, approve it past the prompt, force
	// the prompt, rewrite its arguments or add context to its result. They run
	// on the permission resume path as well (the history holds the model's
	// original arguments, so a rewrite must be applied again to what runs);
	// there allow and ask are moot, because the user already answered.
	var hookRes preToolUseOutcome
	{
		original := tc.InputJSON
		var ran bool
		hookRes, ran = a.runPreToolUseHooks(ctx, &tc, mode)
		if ran && hookRes.blocked {
			result := "blocked by hook: " + hookRes.reason
			a.finishToolCall(sessionDir, sessionID, tc, result, nil, "cancelled")
			return result, nil
		}
		// The turn was cancelled while a hook was running: the hooks answered
		// nothing, and the call must not run on the strength of that silence.
		if ran && ctx.Err() != nil {
			result := "cancelled before the tool ran"
			a.finishToolCall(sessionDir, sessionID, tc, result, nil, "cancelled")
			return result, nil
		}
		if ran && skipPermission && !sameToolArgs(tc.InputJSON, original) {
			// The user approved the arguments the prompt showed; a hook that
			// changes them again on the resume is not covered by that answer.
			result := "cancelled: a hook changed the approved arguments after the approval; run the call again"
			a.finishToolCall(sessionDir, sessionID, tc, result, nil, "cancelled")
			return result, nil
		}
		if ran && !sameToolArgs(tc.InputJSON, original) {
			// The rewritten arguments are what runs and what the operator must
			// see on the tool call card; the model's own message keeps the
			// original call, as it must for the transcript to replay. A
			// permission answered later resumes on this persisted value, so a
			// write that fails cancels the call before any prompt instead of
			// letting the resume fall back to arguments nobody saw.
			if sessionDir != "" && strings.TrimSpace(tc.ID) != "" {
				argsPersistErr = session.WriteToolCallArgs(sessionDir, tc.ID, tc.InputJSON)
			}
			_ = a.server.SendSessionUpdate(sessionID, acp.ToolCallStatusUpdate{
				SessionUpdate: acp.UpdateTypeToolCallUpdate,
				ToolCallID:    tc.ID,
				Status:        "in_progress",
				Content: []acp.ToolCallResultItem{
					{Type: "content", Content: acp.ContentBlock{Type: "text", Text: tc.InputJSON}},
				},
			})
		}
	}

	// Check if tool requires permission.
	tool, ok := a.registry.Get(tc.Name)
	var sessCmdGrants, sessWriteGrants, sessHTTPGrants []string
	if st := sessionStatePtr(a.state); st != nil {
		sessCmdGrants = st.GetPermissionCommandGrants()
		sessWriteGrants = st.GetPermissionWriteGrants()
		sessHTTPGrants = st.GetPermissionHTTPGrants()
	}
	requiresPerm := permissionRequired(ok && tool.RequiresPermission, tc, env, sessCmdGrants, sessWriteGrants, sessHTTPGrants)

	// A hook's allow skips the prompt; its ask forces one even in a mode that
	// would auto-approve.
	if hookRes.allow {
		requiresPerm = false
	}
	if hookRes.ask {
		requiresPerm = true
	}

	if requiresPerm && !skipPermission {
		promptBody := permission.PromptBody(tc.Name, tc.InputJSON)
		if tc.Name == toolweb.ToolHTTPRequest {
			// Raw arguments would bury the address and the files in JSON;
			// the prompt shows the request as it would go out.
			promptBody = permission.HTTPRequestPromptBody(tc.InputJSON, env.CWD)
		}
		if tc.Name == "config_commit" {
			// The commit call itself carries no arguments, so the dialog must
			// show the staged commands it would apply (secrets redacted) -
			// otherwise the operator confirms blindly.
			if pending := tools.PendingConfigSummary(env); len(pending) > 0 {
				promptBody += "\n\nStaged config commands to be committed:\n" + strings.Join(pending, "\n")
			}
		}
		if tc.Name == "config_rollback" {
			promptBody += "\n\nRestores the pre-commit snapshot (config.yaml.prev) over the active configuration; " +
				"changes committed after that snapshot leave the active file."
		}
		if argsPersistErr != nil {
			result := "cancelled: the arguments could not be persisted before the permission prompt: " + argsPersistErr.Error()
			a.finishToolCall(sessionDir, sessionID, tc, result, nil, "cancelled")
			return result, nil
		}
		// Notification hooks learn that a prompt is about to wait for the
		// operator (a chat ping, a desktop notification); they cannot answer it.
		a.runNotificationHooks(ctx, mode, hookNotificationPermissionPrompt, tc, promptBody)
		// tools.permission_timeout_seconds bounds the wait so a connected but
		// unresponsive client cannot hold the session turn lock forever; the
		// default (0) keeps waiting, which is the interactive contract. The
		// notification above goes out first: it announces the wait this bounds.
		permCtx := ctx
		var cancelPerm context.CancelFunc
		if d := a.cfg.Tools.ResolvedPermissionTimeout(); d > 0 {
			permCtx, cancelPerm = context.WithTimeout(ctx, d)
		}
		// The mode this prompt is asked under: the session's, or ask when a
		// hook forced the prompt - a sender must not wave that one through.
		askedUnder := env.PermissionMode
		if hookRes.ask {
			askedUnder = config.PermModeAsk
		}
		permResult, err := a.server.RequestPermission(permCtx, acp.PermissionRequestParams{
			SessionID: sessionID,
			ToolCall: acp.PermissionToolCall{
				ToolCallID: tc.ID,
				Title:      fmt.Sprintf("Run: %s", tc.Name),
				Kind:       toolKind(tc.Name),
				Status:     "pending",
				Content: []acp.ToolCallResultItem{
					{Type: "content", Content: acp.ContentBlock{Type: "text", Text: promptBody}},
				},
			},
			Options: permission.OptionsFor(tc.Name, tc.InputJSON, permission.OptionContext{
				Mode:          env.PermissionMode,
				SessionSwitch: a.subagent == nil && !hookRes.ask,
			}),
			SessionPermissionMode: askedUnder,
		})
		timedOut := permCtx.Err() == context.DeadlineExceeded
		if cancelPerm != nil {
			cancelPerm()
		}
		if timedOut {
			a.log.Warn("permission prompt timed out; cancelling tool call",
				"tool", tc.Name, "session", sessionID, "timeout", a.cfg.Tools.ResolvedPermissionTimeout())
			if permResult == nil {
				permResult = &acp.PermissionResult{Outcome: "cancelled", OptionID: "reject"}
			}
			permResult.Reason = permissionTimeoutReason
		}

		if err != nil || !permission.Approved(permResult) {
			_ = a.server.SendSessionUpdate(sessionID, acp.ToolCallStatusUpdate{
				SessionUpdate: acp.UpdateTypeToolCallUpdate,
				ToolCallID:    tc.ID,
				Status:        "cancelled",
			})
			return permissionDeniedResult(permResult), nil
		}
		if st := sessionStatePtr(a.state); st != nil {
			permission.RecordAllowAlways(st, tc.Name, tc.InputJSON, env.CWD, permResult)
		}
		a.switchPermissionModeFromDialog(ctx, env, permResult)
	}

	// Execute the tool.
	var result string
	var execErr error
	started := time.Now()

	// Check if it's an MCP tool (name contains __).
	if idx := strings.Index(tc.Name, "__"); idx >= 0 {
		serverName := tc.Name[:idx]
		toolName := tc.Name[idx+2:]
		result, execErr = a.callMCPTool(ctx, serverName, toolName, tc.InputJSON)
		// MCP calls bypass the built-in registry, so apply the shared default
		// output limit here.
		if execErr == nil {
			result = tools.ApplyOutputLimit(result, tc.Name, env)
		} else {
			execErr = tools.ApplyOutputLimitError(execErr, tc.Name, env)
		}
	} else {
		result, execErr = a.registry.Execute(ctx, tc.Name, tc.InputJSON, env)
	}

	// PostToolUse (or PostToolUseFailure) feedback and the PreToolUse
	// context travel with the result, so the model reads them next to the
	// output they refer to.
	if feedback := a.runPostToolUseHooks(ctx, tc, result, execErr, time.Since(started), mode); feedback != "" {
		if execErr != nil {
			execErr = fmt.Errorf("%w\n\n%s", execErr, feedback)
		} else {
			result = joinHookText(result, feedback)
		}
	}
	if len(hookRes.context) > 0 {
		text := hookContextText(hookRes.context)
		if execErr != nil {
			execErr = fmt.Errorf("%w\n\n%s", execErr, text)
		} else {
			result = joinHookText(result, text)
		}
	}

	status := "completed"
	if execErr != nil {
		status = "failed"
	}
	a.emitDebug(turn, "tool_finish", tc.Name, "", map[string]interface{}{
		"tool_call_id": tc.ID,
		"kind":         toolKind(tc.Name),
		"status":       status,
		"ok":           execErr == nil,
	})
	a.finishToolCall(sessionDir, sessionID, tc, result, execErr, status)
	return result, execErr
}

// finishToolCall persists the outcome of one tool call and publishes the final
// tool_call_update: the normal completed/failed path and the mode refusal
// (status cancelled, result carrying the refusal text) share it so the
// transcript, the tool_calls store, and the preview stay consistent.
//
// The plan snapshot is written before the call is marked finished: a transcript
// reload that reads meta.json in between then sees an in_progress call that
// already carries its plan rows, never a completed call without them.
func (a *Agent) finishToolCall(sessionDir, sessionID string, tc llm.ToolCall, result string, execErr error, status string) {
	var todoPlanSnapshot []acp.PlanEntry
	if status == "completed" {
		todoPlanSnapshot = todoPlanSnapshotAfterToolCall(tc.Name, a.state, execErr)
	}

	if sessionDir != "" && strings.TrimSpace(tc.ID) != "" {
		finalText := result
		if execErr != nil {
			finalText = fmt.Sprintf("error: %v", execErr)
		}
		_ = session.WriteToolCallResult(sessionDir, tc.ID, finalText)
		if len(todoPlanSnapshot) > 0 {
			_ = session.WriteToolCallPlanSnapshot(sessionDir, tc.ID, todoPlanSnapshot)
		}
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
	previewMeta = session.AttachTodoPlanMeta(previewMeta, todoPlanSnapshot)

	_ = a.server.SendSessionUpdate(sessionID, acp.ToolCallStatusUpdate{
		SessionUpdate: acp.UpdateTypeToolCallUpdate,
		ToolCallID:    tc.ID,
		Status:        status,
		Content:       content,
		Meta:          previewMeta,
	})
}

func todoPlanSnapshotAfterToolCall(toolName string, state SessionState, execErr error) []acp.PlanEntry {
	if execErr != nil || state == nil {
		return nil
	}
	switch toolName {
	case todo.ToolNameItemUpdate, todo.ToolNamePlanReplace:
		entries := state.GetPlan()
		if len(entries) == 0 {
			return nil
		}
		return append([]acp.PlanEntry(nil), entries...)
	default:
		return nil
	}
}

// currentToolDefinitions builds the definition list for mode, reflecting the
// configuration in force right now. It is called again after config_commit so a
// tool the reload enabled or disabled reaches the model in the same turn.
func (a *Agent) currentToolDefinitions(mode string) []llm.ToolDefinition {
	toolSet := ToolSetForMode(mode, a.cfg.Tools.PlanNoSelfRunEnabled())
	available := a.registry.AllToolDefinitions()
	if a.configReloader == nil {
		// Without a runtime reloader the staged config flow cannot commit, so
		// the whole editing family is hidden; config_get stays read-only.
		filtered := available[:0]
		for _, definition := range available {
			switch definition.Name {
			case "config_set", "config_changes", "config_commit", "config_revert", "config_rollback":
			default:
				filtered = append(filtered, definition)
			}
		}
		available = filtered
	}
	defs := FilterToolDefinitions(available, toolSet)
	if ModeAllowsMCPTools(mode) {
		defs = append(defs, a.mcpToolDefinitions()...)
	}
	if a.subagent != nil {
		// An empty effective set means no tools at all, not "unrestricted" as
		// the nil ToolSet would read; the spawn refuses such a set up front,
		// this keeps a replayed or restored child honest too.
		if len(a.subagent.Tools) == 0 {
			return nil
		}
		defs = FilterToolDefinitions(defs, ToolSet(a.subagent.Tools))
	} else if !a.canSpawnInMode(mode) {
		// spawn_agent is registered whenever the feature is on; a surface with no
		// runtime (a scheduled run), a session at the depth limit or a read-only
		// turn (ask, docs) must not advertise it.
		filtered := make([]llm.ToolDefinition, 0, len(defs))
		for _, d := range defs {
			if d.Name != tools.ToolSpawnAgent {
				filtered = append(filtered, d)
			}
		}
		defs = filtered
	}
	return defs
}

// mcpToolDefinitions converts the tools of connected MCP clients into LLM tool
// definitions, applying the configured enable/disable filter. Shared by the
// main prompt path and the permission-resume path so the two cannot drift.
//
// Callers are responsible for checking ModeAllowsMCPTools first: docs and ask
// never receive MCP definitions at all.
func (a *Agent) mcpToolDefinitions() []llm.ToolDefinition {
	allowed := a.state.GetMCPToolFilter()
	var defs []llm.ToolDefinition
	for _, client := range a.state.GetMCPClients() {
		for _, t := range client.Tools() {
			if !allowed(client.Name(), t.Name) {
				continue
			}
			defs = append(defs, t.ToLLMToolDefinition(client.Name()))
		}
	}
	return defs
}

// callMCPTool routes a tool call to the appropriate MCP client. Disabled
// tools are rejected here too so stale history cannot invoke them.
func (a *Agent) callMCPTool(ctx context.Context, serverName, toolName, argsJSON string) (string, error) {
	if allowed := a.state.GetMCPToolFilter(); !allowed(serverName, toolName) {
		return "", fmt.Errorf("MCP tool %s__%s is disabled", serverName, toolName)
	}
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
	// The foxxycode compaction engine replays only the window from the last summary onward; earlier
	// history stays in the transcript for the UI. The opencode engine keeps the full slice and
	// relies on isLLMHistoryMessage to drop messages flagged Compacted.
	if a.cfg.Compaction.EngineIsCoddy() {
		history = session.MessagesForLLM(history)
	}
	msgs := make([]llm.Message, 0, len(history)+1)
	msgs = append(msgs, llm.Message{Role: llm.RoleSystem, Content: systemPrompt})
	for _, m := range history {
		if !isLLMHistoryMessage(m) {
			continue
		}
		msgs = append(msgs, m)
	}
	return msgs
}

// invokedSkillBlocks returns an attachment carrying the body of every skill
// the typed text invokes as /name. It rides in the message that invoked it,
// written once, so the next turn replays the same bytes rather than a message
// that lost the body it was sent with.
func invokedSkillBlocks(text string, allSkills []*skills.Skill) []acp.ContentBlock {
	var out []acp.ContentBlock
	for _, inv := range invokedSkills(text, allSkills) {
		n, sk := inv.name, inv.skill
		body := strings.TrimSpace(sk.Content)
		if body == "" {
			continue
		}
		out = append(out, acp.ContentBlock{Type: acp.ContentTypeResource, Resource: &acp.Resource{
			URI:      "skill:" + n,
			MimeType: "text/markdown; charset=utf-8",
			Text:     body,
			Mention:  &acp.ResourceMention{Kind: mention.KindSkill, Name: n},
		}})
	}
	return out
}

// invokedSkill is one skill a prompt invokes, under the name it was invoked by.
type invokedSkill struct {
	name  string
	skill *skills.Skill
}

// invokedSkills resolves the /name tokens of the typed text to skills, in the
// order they appear.
func invokedSkills(text string, allSkills []*skills.Skill) []invokedSkill {
	if len(allSkills) == 0 {
		return nil
	}
	names := skills.ParseInvokedCommandNames(text)
	if len(names) == 0 {
		return nil
	}
	idx := skills.SkillBySlashName(allSkills)
	var out []invokedSkill
	for _, n := range names {
		if sk, ok := idx[n]; ok {
			out = append(out, invokedSkill{name: n, skill: sk})
		}
	}
	return out
}

// applySkillSettings runs the rest of the turn on the model and reasoning
// level a skill's frontmatter names, whether the operator invoked the skill
// or the model loaded it. A setting the turn already holds - the operator's
// --once, the model's own switch - is not overridden, and a value the
// configuration cannot honour is logged and skipped: a skill never fails the
// turn it helps.
func (a *Agent) applySkillSettings(ctx context.Context, name string, sk *skills.Skill) {
	if sk == nil || a.subagent != nil || (sk.Model == "" && sk.Reasoning == "") {
		return
	}
	ap := a.settings()
	if ap == nil {
		return
	}
	ch := session.SettingsChange{Source: "skill:" + name}
	if m := sk.Model; m != "" && a.state.TurnSetting(session.SettingModel) == "" {
		ch.Model = &m
	}
	if r := sk.Reasoning; r != "" && a.state.TurnSetting(session.SettingReasoning) == "" {
		ch.Reasoning = &r
	}
	if ch.Empty() {
		return
	}
	if _, err := ap.ApplyTurnSettings(ctx, a.state.GetID(), ch); err != nil {
		a.log.Warn("skill frontmatter settings skipped", "skill", name, "error", err)
	}
}

// typedText is what the user wrote: the text blocks of a prompt, without the
// attachments resolved for it, so a "/name" inside an attached file invokes
// nothing.
func typedText(blocks []acp.ContentBlock) string {
	var parts []string
	for _, b := range blocks {
		if b.Type == acp.ContentTypeText {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n\n")
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

// startTitleGeneration launches the session-title pass off the hot path, at most
// once per turn. Internal guards in maybeGenerateTitle make it a no-op when the
// title is pinned or already generated, and the detached context lets it outlive
// the turn that started it - including a turn the user stopped. Non-fatal by design.
func (a *Agent) startTitleGeneration(provider llm.Provider) {
	// config_commit can replace a.cfg while this goroutine is awaiting the title
	// completion. Config values are immutable after loading, so retaining the
	// current pointer gives this work a stable view without synchronizing the
	// whole ReAct loop.
	cfg := a.cfg
	a.titleOnce.Do(func() {
		titleCtx, titleCancel := context.WithTimeout(context.Background(), 30*time.Second)
		go func() {
			defer titleCancel()
			a.maybeGenerateTitleForConfig(titleCtx, provider, cfg)
		}()
	})
}

// llmTransport pairs a provider with the transport it was built for. The ReAct
// loop needs the distinction: a provider for a model with stream: false answers
// in one piece after the whole completion is generated, so guards that expect
// chunks to arrive progressively do not apply to it.
type llmTransport struct {
	provider  llm.Provider
	streaming bool
}

// getProvider creates the LLM provider for the given mode.
func (a *Agent) getProvider(mode string) (llmTransport, error) {
	modelID := a.state.EffectiveModelID(a.cfg)
	if modelID == "" {
		return llmTransport{}, fmt.Errorf("no model configured")
	}

	rm, err := a.cfg.ResolveLLM(modelID)
	if err != nil {
		return llmTransport{}, err
	}

	mk := a.providerFactory
	if mk == nil {
		mk = llm.NewProvider
	}
	in := a.childProviderInput(a.turnProviderInput(rm))
	in.ReasoningEffort = a.state.EffectiveReasoning(a.cfg)
	// The stall guard goes on here rather than inside NewProvider, so it wraps
	// whatever the factory built - a test double as much as a real provider -
	// and still sits outside the retry wrapper (llm.WithStreamIdleGuard).
	idle := in.StreamIdleTimeout
	in.StreamIdleTimeout = 0
	provider, err := mk(in)
	if err != nil {
		return llmTransport{}, err
	}
	provider = a.withChildFallbacks(provider, modelID, mk)
	return llmTransport{provider: llm.WithStreamIdleGuard(provider, idle), streaming: rm.Stream}, nil
}

func (a *Agent) llmProviderInput(rm *config.ResolvedLLM) llm.ProviderInput {
	return a.llmProviderInputForConfig(a.cfg, rm)
}

// llmProviderInputForConfig builds provider settings from an immutable configuration snapshot.
// It is used by detached work such as title generation, which must not race a config reload.
func (a *Agent) llmProviderInputForConfig(cfg *config.Config, rm *config.ResolvedLLM) llm.ProviderInput {
	in := llm.ProviderInput{
		Name:          rm.ProviderName,
		Type:          rm.ProviderType,
		Model:         rm.Model,
		APIKey:        rm.APIKey,
		BaseURL:       rm.BaseURL,
		ProxyURL:      rm.ProxyURL,
		AuthPath:      rm.AuthPath,
		MaxTokens:     rm.MaxTokens,
		Temperature:   rm.Temperature,
		DisableStream: !rm.Stream,
		Timeout:       time.Duration(rm.TimeoutMS) * time.Millisecond,
	}
	// The stall guard (agent.llm_stream_idle_timeout_ms) watches the gaps
	// between the chunks of a streamed answer; a blocking answer arrives in
	// one piece and has no gaps to watch.
	if rm.Stream {
		in.StreamIdleTimeout = cfg.Agent.EffectiveLLMStreamIdleTimeout()
	}
	return llm.WithAgentResilience(in, cfg.Agent.EffectiveLLMRetryMax(), cfg.Agent.LLMRetryBaseMS, cfg.Agent.LLMMinIntervalMS)
}

// settingsApplier is the manager's setter for session settings. The agent
// reaches it through its subagent runtime, which is the manager on every
// surface; without one (a bare agent in a test) the state is written directly.
type settingsApplier interface {
	ApplySessionSettings(ctx context.Context, sessionID string, ch session.SettingsChange) (acp.SessionSettings, error)
	ApplyTurnSettings(ctx context.Context, sessionID string, ch session.SettingsChange) (acp.SessionSettings, error)
}

// settings returns the manager's setter, or nil when the agent runs without one.
func (a *Agent) settings() settingsApplier {
	if ap, ok := a.subagentRuntime.(settingsApplier); ok {
		return ap
	}
	return nil
}

// switchModel backs the switch_model tool: the model's own choice of model
// and reasoning level, for the rest of the turn or for the session. It goes
// through the manager's setter like the operator's command, so it is checked
// against the configuration, logged and shown on every surface; the loop
// builds the new transport before its next request.
func (a *Agent) switchModel(ctx context.Context, req tooling.ModelSwitch) (string, error) {
	ap := a.settings()
	if ap == nil {
		return "", fmt.Errorf("switch_model is not available in this session")
	}
	ch := session.SettingsChange{Source: "model"}
	if req.Model != "" {
		ch.Model = &req.Model
	}
	if req.Reasoning != "" {
		ch.Reasoning = &req.Reasoning
	}
	scope := "for the rest of this turn"
	var err error
	if req.Session {
		scope = "for the rest of the session"
		_, err = ap.ApplySessionSettings(ctx, a.state.GetID(), ch)
	} else {
		_, err = ap.ApplyTurnSettings(ctx, a.state.GetID(), ch)
	}
	if err != nil {
		return "", err
	}
	model := a.state.EffectiveModelID(a.cfg)
	reasoning := a.state.EffectiveReasoning(a.cfg)
	if reasoning == "" {
		reasoning = "none offered"
	}
	return fmt.Sprintf("Switched %s: model %s, reasoning %s. It applies from your next request.", scope, model, reasoning), nil
}

// switchPermissionModeFromDialog applies a permission answer that also
// switches the session's permission mode ("bypass permissions for this
// session", "allow edits for this session", #292). The change goes through
// the manager's setter, so every surface shows it and the log records it,
// and the tool environment follows at once: the rest of this turn runs under
// the new mode.
func (a *Agent) switchPermissionModeFromDialog(ctx context.Context, env *tools.Env, res *acp.PermissionResult) {
	mode := permission.SessionModeOption(res)
	if mode == "" || a.subagent != nil {
		return
	}
	if ap := a.settings(); ap != nil {
		if _, err := ap.ApplySessionSettings(ctx, a.state.GetID(), session.SettingsChange{PermissionMode: &mode, Source: "permission_dialog"}); err != nil {
			a.log.Warn("permission dialog: the session's permission mode could not be switched", "mode", mode, "error", err)
			return
		}
	} else if st := sessionStatePtr(a.state); st != nil {
		st.SetPermissionMode(mode)
		st.ClearTurnOverride(session.SettingPermissionMode)
		a.log.Info("permission mode switched from the permission dialog", "session", a.state.GetID(), "mode", mode)
	}
	if env != nil {
		env.PermissionMode = effectivePermMode(a.state, a.cfg)
	}
}

// transportKey names what a model request is built for: the model and the
// reasoning level. The transport is rebuilt only when it changes.
func (a *Agent) transportKey() string {
	return a.state.EffectiveModelID(a.cfg) + "|" + a.state.EffectiveReasoning(a.cfg)
}

// childProviderInput applies what a system child's spec says about its
// model calls: the completion cap of memory.copilot_max_tokens, clamped the
// way the copilot pass clamped it.
func (a *Agent) childProviderInput(in llm.ProviderInput) llm.ProviderInput {
	if a.subagent == nil || a.subagent.MaxTokens <= 0 {
		return in
	}
	if in.MaxTokens <= 0 || in.MaxTokens > a.subagent.MaxTokens {
		in.MaxTokens = a.subagent.MaxTokens
	}
	return in
}

// withChildFallbacks wraps a child's provider in the fallback chain its spec
// names (memory.fallback_models, then the session's model): a call that
// fails before producing any output moves to the next model, a stream that
// broke after output does not. An entry that resolves to nothing is skipped
// and logged; an ordinary session, or a child without fallbacks, gets its
// provider back untouched.
func (a *Agent) withChildFallbacks(primary llm.Provider, modelID string, mk func(llm.ProviderInput) (llm.Provider, error)) llm.Provider {
	if a.subagent == nil || len(a.subagent.FallbackModels) == 0 {
		return primary
	}
	candidates := []llm.FallbackCandidate{{Provider: primary, Model: modelID}}
	seen := map[string]bool{modelID: true}
	for _, ref := range a.subagent.FallbackModels {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		rm, err := a.cfg.ResolveLLM(ref)
		if err != nil {
			a.log.Warn("fallback model unavailable; skipped", "model", ref, "error", err)
			continue
		}
		in := a.childProviderInput(a.turnProviderInput(rm))
		in.ReasoningEffort = a.state.EffectiveReasoning(a.cfg)
		provider, err := mk(in)
		if err != nil {
			a.log.Warn("fallback model unavailable; skipped", "model", ref, "error", err)
			continue
		}
		candidates = append(candidates, llm.FallbackCandidate{Provider: provider, Model: ref})
	}
	return llm.NewFallbackChain(candidates, func(from, to string, err error) {
		a.log.Warn("model failed before answering; falling back to the next one", "model", from, "next", to, "error", err)
	})
}

// turnProviderInput is llmProviderInput plus the bounds of a user turn: the
// first-token timer as the call's own budget, and with the wait on, the
// wait's maximum as the turn's budget and the turn's ledger. Helpers
// (compaction, the memory copilot, title generation) use llmProviderInput
// alone and never see the option.
func (a *Agent) turnProviderInput(rm *config.ResolvedLLM) llm.ProviderInput {
	in := a.llmProviderInput(rm)
	// The first-token timer cuts a streamed call that stays silent, retry
	// waits included: a server-requested pause the timer would cut anyway
	// is reported as a quota reset instead of being slept through in vain.
	if rm.Stream {
		if timeout := a.cfg.Agent.EffectiveLLMFirstTokenTimeout(); timeout > 0 {
			in.CallBudget = timeout
		}
	}
	// With the wait on, its maximum bounds every sleep the turn spends on a
	// limit, the wrapper's retries included: a pause beyond it comes back
	// as a quota reset and ends the turn at once instead of being slept
	// through by the retries first, and an explicit zero means no sleep on
	// a limit anywhere. The wrapper's sleeps count against the turn's
	// total, calls that succeed afterwards included.
	if a.cfg.Agent.WaitForLimitReset {
		in.RetryBudget, in.RetryBudgetSet = a.cfg.Agent.EffectiveWaitForLimitResetMax(), true
		in.LimitLedger = a.limitLedgerFor()
	}
	return in
}

// contentBlocksToText converts ACP content blocks to a plain text string.
// Attachments - a mentioned file, folder, session, rule or subagent, or a
// resource an editor sent - become <foxxycode_attachment ...> elements with the
// body in CDATA (resourceAttachmentXML), so the SPA and the console can
// collapse them for display while the model retains full context.
func contentBlocksToText(blocks []acp.ContentBlock) string {
	var parts []string
	for _, b := range blocks {
		switch b.Type {
		case acp.ContentTypeText:
			parts = append(parts, b.Text)
		case acp.ContentTypeResource:
			if b.Resource != nil {
				parts = append(parts, resourceAttachmentXML(b.Resource))
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

// wrapXMLCDATA wraps body in CDATA, split where the body itself holds the
// terminator sequence.
func wrapXMLCDATA(body string) string {
	escaped := strings.ReplaceAll(body, "]]>", "]]]]><![CDATA[>")
	return "<![CDATA[" + escaped + "]]>"
}

// extractContextFiles returns the local files and folders the prompt's
// attachments read: a file:// resource an editor sent, and every file or
// folder a mention resolved to. Path-scoped rules activate on them.
func extractContextFiles(blocks []acp.ContentBlock) []string {
	var files []string
	for _, b := range blocks {
		if b.Type != acp.ContentTypeResource || b.Resource == nil {
			continue
		}
		if m := b.Resource.Mention; m != nil && m.Path != "" &&
			(m.Kind == mention.KindFile || m.Kind == mention.KindDirectory) {
			files = append(files, m.Path)
			continue
		}
		if p := contextFilePath(b.Resource.URI); p != "" {
			files = append(files, p)
		}
	}
	return files
}

// contextFilePath turns one resource URI into a filesystem path, or "" when it
// names no file on disk.
//
// The two surfaces spell the same mention differently: an editor over ACP sends
// file:///C:/proj/src/sample.go, while the HTTP surface sends the
// workspace-relative path the user typed, "src/sample.go". Reading only the
// file:// form left every path-scoped rule and skill inactive off ACP. A URI
// carrying any other scheme (http://, data:) is not a path and is dropped.
func contextFilePath(uri string) string {
	uri = strings.TrimSpace(uri)
	switch {
	case uri == "":
		return ""
	case strings.HasPrefix(uri, "file://"):
		return fileURIPath(uri)
	case hasURIScheme(uri):
		return ""
	}
	return uri
}

// hasURIScheme reports whether s opens with a URI scheme. A Windows drive
// letter is deliberately not one: "C:/proj/x.go" is a path, and a scheme needs
// more than a single letter before the colon.
func hasURIScheme(s string) bool {
	colon := strings.IndexByte(s, ':')
	if colon < 2 {
		return false
	}
	for i := 0; i < colon; i++ {
		c := s[i]
		switch {
		case isASCIILetter(c):
		case c >= '0' && c <= '9', c == '+', c == '-', c == '.':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// fileURIPath turns a file:// URI into a filesystem path. On Windows the
// authority-less form is file:///C:/proj/x.go, whose leading slash must go —
// "/C:/proj/x.go" matches no rule scope and no glob. A POSIX path that merely
// contains a colon (/a:b) keeps its slash: the drive form requires a separator
// after the colon, or nothing at all.
func fileURIPath(uri string) string {
	p := strings.TrimPrefix(uri, "file://")
	if len(p) >= 3 && p[0] == '/' && isASCIILetter(p[1]) && p[2] == ':' &&
		(len(p) == 3 || p[3] == '/' || p[3] == '\\') {
		p = p[1:]
	}
	return p
}

func isASCIILetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// toolKind maps a tool name to an ACP tool call kind.
func toolKind(name string) string {
	switch name {
	case "read", "keep_result", "glob", "grep", "websearch", "webfetch", "config_get", "config_changes", "foxxycode_docs_search", "foxxycode_docs_read":
		return "read"
	case "write", "edit", "apply_patch", "mkdir", "rmdir", "touch", "rm", "mv",
		"svn_add", "svn_revert", "svn_resolve", "svn_update", "svn_commit",
		"svn_switch", "svn_merge", "svn_checkout",
		"config_commit", "config_rollback":
		return "write"
	case "run_command":
		return "run_command"
	default:
		return "other"
	}
}

func filesystemWriteTool(name string) bool {
	switch name {
	case "write", "edit", "apply_patch", "mkdir", "rmdir", "touch", "rm", "mv",
		"svn_add", "svn_revert", "svn_resolve", "svn_update", "svn_commit",
		"svn_switch", "svn_merge", "svn_checkout":
		return true
	default:
		return false
	}
}

// configWriteTool names the tools that write the agent's own configuration.
// They get a stricter permission policy than project file writes: accept_edits
// never auto-approves them (see executeToolCall).
func configWriteTool(name string) bool {
	switch name {
	case "config_commit", "config_rollback":
		return true
	default:
		return false
	}
}

// effectivePermMode returns the session-level permission mode override, falling back to the config default.
func effectivePermMode(state SessionState, cfg *config.Config) string {
	if m := state.EffectivePermissionMode(); m != "" {
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

// ideEnvMaxSelectionBytes caps the selection text included in the IDE context
// block. Kept short — a paste-to-chip attachment carries the full fragment on
// demand; this is ambient context only.
const ideEnvMaxSelectionBytes = 2 * 1024

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
	if snap.ActiveFile == "" && len(snap.OpenFiles) == 0 && snap.Selection == nil {
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
	if sel := snap.Selection; sel != nil {
		text := sel.Text
		if len(text) > ideEnvMaxSelectionBytes {
			text = text[len(text)-ideEnvMaxSelectionBytes:]
		}
		b.WriteString("\n\n# Selection\n")
		fmt.Fprintf(&b, "%s:%d-%d\n", rel(sel.File), sel.StartLine, sel.EndLine)
		b.WriteString(text)
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
		b.WriteString(html.EscapeString(tm.Name))
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
