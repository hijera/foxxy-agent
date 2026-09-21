import type { CSSProperties } from "react";
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import type { HeroAccentVerb } from "./heroTitleWords";
import type { PermissionResolvedState } from "./permissionTypes";
import type { QuestionResolvedState } from "./questionTypes";
import type { TokenUsage, TranscriptItem } from "./types";
import { UsageBanner } from "./UsageBanner";
import type { ProviderUsage } from "./providerUsage";
import { ChatHeader } from "./ChatHeader";
import { SessionExportMenu, type ExportFormat } from "./SessionExportMenu";
import { Composer } from "./Composer";
import { SubagentReadOnlyNotice } from "./SubagentReadOnlyNotice";
import type { SubagentTranscriptMeta } from "./subagentTranscript";
import type { QueuedMessage } from "./Composer";
import { MessageList } from "../messages/MessageList";
import type { BackgroundTask } from "../tasks/types";
import { BackgroundTasksChip } from "../tasks/BackgroundTasksChip";
import { useT } from "../i18n/I18nProvider";
import { ArchivedSessionNotice } from "./ArchivedSessionNotice";
import {
  subscribeShellStack,
  snapshotShellStack,
  serverSnapshotShellStack,
} from "../shellBreakpoint";
import { transcriptItemsAffectAutoScroll } from "./transcriptAutoScroll";
import {
  documentScrollBottom,
  documentTranscriptMetrics,
  easeTranscriptJump,
  elementScrollBottom,
  elementTranscriptMetrics,
  isTranscriptAtBottom,
  transcriptJumpDurationMs,
} from "./transcriptScrollPosition";
import { ScrollToBottomButton } from "./ScrollToBottomButton";

export function ChatScreen(props: {
  title: string;
  sessionId: string;
  /** Accent verb for "What do you want to …?" on the empty hero (session-stable or home rotation). */
  heroAccentVerb: HeroAccentVerb;
  /** Bumps when the user starts a fresh home chat so the composer can refocus. */
  heroComposerFocusEpoch: number;
  onTitleSave: (title: string) => void;
  items: TranscriptItem[];
  /** Export the session transcript as a document. Hidden until an assistant answer exists. */
  onExportSession?: (format: ExportFormat) => void;
  exportBusy?: boolean;
  draft: string;
  tokenUsage: TokenUsage | null;
  /** Account usage behind the selected model's provider: the pill and the banner. */
  providerUsage?: ProviderUsage | null;
  usageBannerDismissedKey?: string;
  onUsageBannerDismiss?: (key: string) => void;
  contextPct?: number;
  maxContextTokens?: number;
  contextBreakdown?: import("./ContextBreakdownPopover").ContextBreakdown | null;
  mode: string;
  modes: string[];
  llmModels?: string[];
  llmModel?: string;
  onLlmModelChange?: (modelId: string) => void;
  /** Whether the currently selected model accepts image/file inputs. */
  llmModelMultimodal?: boolean;
  /** Reasoning levels offered by the current model (empty hides the selector). */
  llmReasoningLevels?: string[];
  llmReasoning?: string;
  onLlmReasoningChange?: (level: string) => void;
  onModeChange: (mode: string) => void;
  onDraftChange: (v: string) => void;
  onSend: (text: string, files?: File[]) => void;
  /** Pass-through to Composer: captured paste-chip literals (see Composer). */
  onPasteChipCaptured?: (key: string, literal: string) => void;
  onContextRingOpen?: () => void;
  generating?: boolean;
  onStop?: () => void;
  /** Follow-ups waiting for the running turn to read them (the message queue). */
  queuedMessages?: QueuedMessage[];
  /** Add the draft to that queue instead of starting a turn. */
  onQueue?: (text: string) => void;
  /** Take one queued follow-up back before the agent reads it. */
  onCancelQueued?: (id: string) => void;
  /** Fetch persisted full tool output; UI keeps preview in resultText. */
  onFetchToolCallFull?: (toolCallId: string) => Promise<void>;
  onQuestionPromptResolved?: (
    sessionId: string,
    itemId: string,
    resolved: QuestionResolvedState,
  ) => void;
  onPermissionPromptResolved?: (
    sessionId: string,
    itemId: string,
    resolved: PermissionResolvedState,
  ) => void;
  onPlanDocumentExpanded?: (itemId: string, expanded: boolean) => void;
  onPlanDocumentRun?: (slug: string) => void;
  onPlanDocumentDiscard?: (itemId: string, slug: string) => void;
  onEdit?: (content: string, userMsgIdx: number) => void;
  editingFiles?: { name: string; mimeType: string }[];
  onBranchSwitch?: (sessionId: string) => void;
  sessionLoading?: boolean;
  sessionFadingOut?: boolean;
  knownSkillNames?: Set<string>;
  /** Background tasks of this session keyed by the tool call that started them. */
  backgroundTasksByToolCallId?: Map<string, BackgroundTask>;
  backgroundNowMs?: number;
  /** Every background task of this chat, for the opener under the transcript. */
  backgroundTasks?: BackgroundTask[];
  onOpenBackgroundTasks?: () => void;
  onOpenBackgroundTask?: (taskId: string) => void;
  onStopBackgroundTask?: (taskId: string) => void;
  /** Roots this session works in - its own directory, then its worktrees -
   *  which tool rows spell paths against. */
  pathRoots?: readonly string[];
  /** Workspace context chips (folder / branch / worktree) above the composer field. */
  workspaceCtx?: import("./workspaceContext").WorkspaceContext | null;
  worktreePref?: boolean;
  svnFolderPref?: boolean;
  /** The workspace is chosen once: locked as soon as the conversation starts. */
  workspaceLocked?: boolean;
  onWorkspacePickFolder?: (path: string) => void;
  onWorkspacePickBranch?: (branch: string, worktree: boolean) => void;
  onWorktreeToggle?: () => void;
  onWorkspacePickSvnBranch?: (branch: string, separateFolder: boolean) => void;
  onSvnFolderToggle?: () => void;
  /** Set when this session is a subagent's transcript: the composer gives way to a read-only notice. */
  subagentTranscript?: SubagentTranscriptMeta | null;
  /** True when the conversation on screen is archived: the composer gives way to the notice that offers to take it back out. */
  sessionArchived?: boolean;
  onUnarchiveSession?: () => void;
  /** True while that request is in flight. */
  unarchiving?: boolean;
  /** Opens another session in this tab (the parent chat from the notice). */
  onOpenSession?: (sessionId: string) => void;
}) {
  const { t } = useT();
  const messagesRef = useRef<HTMLDivElement | null>(null);
  // Hero and docked are two branches of one ternary, so the composer unmounts on
  // the transition. Attachments live here to survive it.
  const [attachedFiles, setAttachedFiles] = useState<File[]>([]);
  const composerHostRef = useRef<HTMLDivElement | null>(null);
  const isEmpty = props.items.length === 0;
  const showSkeleton = isEmpty && !!props.sessionLoading;
  const stickToBottomRef = useRef(true);
  const prevItemsForScrollRef = useRef<TranscriptItem[]>([]);
  // The scroll-tail reserve is written straight to this element, never held as
  // state: see the effect below for why React must stay out of this loop.
  const chatStackRef = useRef<HTMLDivElement | null>(null);
  const composerReserveRef = useRef(0);
  // The jump owns the scroll position while it travels; a frame id says so.
  const jumpFrameRef = useRef<number | null>(null);
  const [showScrollToBottom, setShowScrollToBottom] = useState(false);
  const mobileDocScroll = useSyncExternalStore(
    subscribeShellStack,
    snapshotShellStack,
    serverSnapshotShellStack,
  );

  useLayoutEffect(() => {
    if (isEmpty) return;
    const host = composerHostRef.current;
    if (!host) return;
    const extra = 10;
    const measure = () => Math.max(140, Math.ceil(host.getBoundingClientRect().height) + extra);
    // The reserve is the height of `.chat-scroll-tail` inside the scroll
    // container. Writing it from inside the ResizeObserver callback relayouts
    // the transcript in the same delivery loop, and with content-visibility
    // rows that resizes the observed host again before the loop settles:
    // JCEF (Chromium 104) then raises "ResizeObserver loop limit exceeded"
    // on every transcript open. So the write waits for the next frame and an
    // unchanged value is skipped, and the loop cannot feed itself.
    //
    // It is written to the element rather than held as state, which is the
    // other half of the same problem. A setState here does not write the DOM
    // where it is called: React commits it on its own schedule, and that commit
    // can land after the frame's resize observations have already been
    // delivered - a write inside the delivery loop again, one frame along, which
    // is what the panel resize scenario catches as a reserve write landing in
    // the frame that measured it. A direct write happens exactly where it is
    // scheduled, and re-renders nothing, so nothing cascades from it.
    const write = (next: number) => {
      const stack = chatStackRef.current;
      if (!stack || composerReserveRef.current === next) return;
      composerReserveRef.current = next;
      stack.style.setProperty("--chat-composer-reserve", `${next}px`);
    };
    write(measure());
    let frame = 0;
    const onResize = () => {
      if (frame) return;
      frame = requestAnimationFrame(() => {
        frame = 0;
        write(measure());
      });
    };
    const ro =
      typeof ResizeObserver !== "undefined" ? new ResizeObserver(onResize) : null;
    ro?.observe(host);
    return () => {
      if (frame) cancelAnimationFrame(frame);
      ro?.disconnect();
    };
    // Only isEmpty. tokenUsage was a dependency back when the observer callback
    // wrote the reserve synchronously and restarting the effect was how a
    // changed token line got re-measured; with the write deferred above, it is
    // the one synchronous writer left. A turn updates tokenUsage repeatedly, so
    // React re-ran this effect mid-turn and it measured and wrote before paint -
    // landing in the same frame as an observation, which is the reserve write
    // the panel resize scenario catches. The observer sees every height change
    // the token line can cause, so there is nothing to restart for.
  }, [isEmpty]);

  // Whichever surface scrolls, it is read and written through these three, so
  // the follow, the button and the jump never disagree about where the end is.
  const transcriptScrollBottom = useCallback((): number => {
    if (mobileDocScroll) return documentScrollBottom(window);
    const el = messagesRef.current;
    return el ? elementScrollBottom(el) : 0;
  }, [mobileDocScroll]);

  const readTranscriptScrollTop = useCallback((): number => {
    if (mobileDocScroll) return window.scrollY;
    return messagesRef.current?.scrollTop ?? 0;
  }, [mobileDocScroll]);

  const writeTranscriptScrollTop = useCallback(
    (top: number) => {
      if (mobileDocScroll) {
        window.scrollTo({ top, left: 0, behavior: "auto" });
        return;
      }
      const el = messagesRef.current;
      if (el) el.scrollTop = top;
    },
    [mobileDocScroll],
  );

  const cancelTranscriptJump = useCallback((): boolean => {
    if (jumpFrameRef.current === null) return false;
    cancelAnimationFrame(jumpFrameRef.current);
    jumpFrameRef.current = null;
    return true;
  }, []);

  // One reading of the scrollport drives both behaviours: the transcript
  // follows new output while it sits in the bottom band, and the jump button
  // appears exactly when it stops following.
  const syncTranscriptPosition = useCallback(() => {
    // A jump owns the position while it travels. Reading it mid-flight would
    // put the button back on screen for every frame above the band.
    if (jumpFrameRef.current !== null) return;
    let atBottom: boolean;
    if (mobileDocScroll) {
      atBottom = isTranscriptAtBottom(documentTranscriptMetrics(window));
    } else {
      const el = messagesRef.current;
      if (!el) return;
      atBottom = isTranscriptAtBottom(elementTranscriptMetrics(el));
    }
    stickToBottomRef.current = atBottom;
    setShowScrollToBottom(!atBottom);
  }, [mobileDocScroll]);

  const jumpToNewestMessage = useCallback(() => {
    cancelTranscriptJump();
    stickToBottomRef.current = true;
    setShowScrollToBottom(false);
    const from = readTranscriptScrollTop();
    const to = transcriptScrollBottom();
    const reduceMotion =
      typeof window.matchMedia === "function" &&
      window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    if (to <= from || reduceMotion) {
      writeTranscriptScrollTop(to);
      return;
    }
    const duration = transcriptJumpDurationMs(to - from);
    const started = performance.now();
    const step = (now: number) => {
      const progress = (now - started) / duration;
      // The end is re-read every frame: a streaming turn keeps moving it down,
      // and the travel should land on where the transcript is now.
      const end = transcriptScrollBottom();
      writeTranscriptScrollTop(
        from + (end - from) * easeTranscriptJump(progress),
      );
      if (progress < 1) {
        jumpFrameRef.current = requestAnimationFrame(step);
        return;
      }
      jumpFrameRef.current = null;
      syncTranscriptPosition();
    };
    jumpFrameRef.current = requestAnimationFrame(step);
  }, [
    cancelTranscriptJump,
    readTranscriptScrollTop,
    syncTranscriptPosition,
    transcriptScrollBottom,
    writeTranscriptScrollTop,
  ]);

  // The reader reaching for the wheel, a finger or the scrollbar always wins
  // over a jump still in the air.
  useEffect(() => {
    const takeOver = () => {
      if (cancelTranscriptJump()) syncTranscriptPosition();
    };
    const passive = { passive: true } as const;
    window.addEventListener("wheel", takeOver, passive);
    window.addEventListener("touchstart", takeOver, passive);
    window.addEventListener("mousedown", takeOver, passive);
    return () => {
      window.removeEventListener("wheel", takeOver);
      window.removeEventListener("touchstart", takeOver);
      window.removeEventListener("mousedown", takeOver);
    };
  }, [cancelTranscriptJump, syncTranscriptPosition]);

  useEffect(() => {
    return () => {
      cancelTranscriptJump();
    };
  }, [cancelTranscriptJump]);

  useEffect(() => {
    if (isEmpty) return;
    const prev = prevItemsForScrollRef.current;
    prevItemsForScrollRef.current = props.items;
    if (!transcriptItemsAffectAutoScroll(prev, props.items)) {
      return;
    }
    // A jump already chases the end of a growing transcript; a hard scroll here
    // would fight its travel frame by frame.
    if (jumpFrameRef.current !== null) return;
    // Content grew under a reader who scrolled away: leave them where they are
    // and re-read the position, which is what reveals the button mid-stream.
    if (!stickToBottomRef.current) {
      syncTranscriptPosition();
      return;
    }
    const follow = () => {
      writeTranscriptScrollTop(transcriptScrollBottom());
      syncTranscriptPosition();
    };
    if (mobileDocScroll) {
      // The document takes its new height after layout, not on this tick.
      requestAnimationFrame(() => requestAnimationFrame(follow));
      return;
    }
    follow();
  }, [
    props.items,
    isEmpty,
    mobileDocScroll,
    syncTranscriptPosition,
    transcriptScrollBottom,
    writeTranscriptScrollTop,
  ]);

  useEffect(() => {
    if (isEmpty) return;
    const onScroll = () => syncTranscriptPosition();
    if (mobileDocScroll) {
      window.addEventListener("scroll", onScroll, { passive: true });
      return () => window.removeEventListener("scroll", onScroll);
    }
    const el = messagesRef.current;
    el?.addEventListener("scroll", onScroll, { passive: true });
    return () => el?.removeEventListener("scroll", onScroll);
  }, [isEmpty, mobileDocScroll, syncTranscriptPosition]);

  // A child session is read-only on the server (409 on any prompt), so the
  // notice takes the composer's slot in both the hero and the docked layout.
  // An archived conversation takes the same slot for a different reason: the
  // server would accept the prompt, and accepting it would quietly undo the
  // operator's own "not now".
  const readOnlyNotice = props.subagentTranscript ? (
    <SubagentReadOnlyNotice
      meta={props.subagentTranscript}
      {...(props.onOpenSession ? { onOpenSession: props.onOpenSession } : {})}
    />
  ) : props.sessionArchived ? (
    <ArchivedSessionNotice
      onUnarchive={() => props.onUnarchiveSession?.()}
      {...(props.unarchiving ? { busy: true } : {})}
    />
  ) : null;

  const mainClassName = [
    "main",
    isEmpty && !showSkeleton ? "is-empty" : "",
    props.sessionFadingOut ? "session-fading-out" : "",
  ].filter(Boolean).join(" ");

  return (
    <main className={mainClassName}>
      {showSkeleton ? (
        <div className="chat-skeleton">
          {/* Named state, not just shimmering bars: in a narrow IDE tool window the
              History drawer closes on pick, so this line is the only feedback that
              the conversation is on its way. */}
          <div
            className="chat-skeleton-label"
            role="status"
            data-testid="session-loading-label"
          >
            <span className="chat-skeleton-spinner" aria-hidden="true" />
            {t("sessions.loadingSession")}
          </div>
          <div className="chat-skeleton-bars" aria-hidden="true">
            <div className="chat-skeleton-header">
              <div className="chat-skeleton-bar" style={{ width: "180px", height: "18px", borderRadius: "6px" }} />
            </div>
            <div className="chat-skeleton-messages">
              <div className="chat-skeleton-row chat-skeleton-row--user">
                <div className="chat-skeleton-bar" style={{ width: "220px", height: "38px", borderRadius: "12px" }} />
              </div>
              <div className="chat-skeleton-row">
                <div className="chat-skeleton-bar" style={{ width: "78%", height: "14px", borderRadius: "6px" }} />
                <div className="chat-skeleton-bar" style={{ width: "62%", height: "14px", borderRadius: "6px" }} />
                <div className="chat-skeleton-bar" style={{ width: "70%", height: "14px", borderRadius: "6px" }} />
              </div>
              <div className="chat-skeleton-row chat-skeleton-row--user">
                <div className="chat-skeleton-bar" style={{ width: "160px", height: "38px", borderRadius: "12px" }} />
              </div>
              <div className="chat-skeleton-row">
                <div className="chat-skeleton-bar" style={{ width: "72%", height: "14px", borderRadius: "6px" }} />
                <div className="chat-skeleton-bar" style={{ width: "50%", height: "14px", borderRadius: "6px" }} />
              </div>
            </div>
          </div>
        </div>
      ) : isEmpty ? (
        <div className="hero" id="hero">
          <h1 className="hero-title">
            {(() => {
              const verb = t(`chat.heroVerb.${props.heroAccentVerb}`);
              const marker = "\u0000";
              const full = t("chat.heroTitle", { verb: marker });
              const i = full.indexOf(marker);
              const before = i >= 0 ? full.slice(0, i) : full;
              const after = i >= 0 ? full.slice(i + marker.length) : "";
              return (
                <span className="hero-title-muted">
                  {before}
                  <span
                    className="hero-title-accent"
                    data-testid="hero-title-accent"
                  >
                    {verb}
                  </span>
                  {after}
                </span>
              );
            })()}
          </h1>
          <div className="hero-composer">
            {readOnlyNotice ? null : (
              <UsageBanner
                usage={props.providerUsage}
                modelId={props.llmModel ?? ""}
                {...(props.usageBannerDismissedKey
                  ? { dismissedKey: props.usageBannerDismissedKey }
                  : {})}
                {...(props.onUsageBannerDismiss
                  ? { onDismiss: props.onUsageBannerDismiss }
                  : {})}
              />
            )}
            {readOnlyNotice ?? (
              <Composer
                value={props.draft}
                providerUsage={props.providerUsage ?? null}
                attachedFiles={attachedFiles}
                onAttachedFilesChange={setAttachedFiles}
                isEmpty={true}
                focusEpoch={props.heroComposerFocusEpoch}
                sessionId={props.sessionId}
                contextIdle={!props.sessionId}
                mode={props.mode}
                modes={props.modes}
                tokenUsage={props.tokenUsage}
                {...(props.contextPct !== undefined
                  ? { contextPct: props.contextPct }
                  : {})}
                {...(props.maxContextTokens !== undefined
                  ? { maxContextTokens: props.maxContextTokens }
                  : {})}
                {...(props.contextBreakdown !== undefined
                  ? { contextBreakdown: props.contextBreakdown }
                  : {})}
                {...(props.llmModels !== undefined &&
                props.llmModels.length > 0 &&
                props.onLlmModelChange !== undefined
                  ? {
                      llmModels: props.llmModels,
                      llmModel: props.llmModel,
                      onLlmModelChange: props.onLlmModelChange,
                      llmModelMultimodal: props.llmModelMultimodal,
                      ...(props.llmReasoningLevels !== undefined &&
                      props.llmReasoningLevels.length > 0 &&
                      props.onLlmReasoningChange !== undefined
                        ? {
                            llmReasoningLevels: props.llmReasoningLevels,
                            llmReasoning: props.llmReasoning,
                            onLlmReasoningChange: props.onLlmReasoningChange,
                          }
                        : {}),
                    }
                  : {})}
                onModeChange={props.onModeChange}
                onChange={props.onDraftChange}
                onSend={props.onSend}
                {...(props.onPasteChipCaptured
                  ? { onPasteChipCaptured: props.onPasteChipCaptured }
                  : {})}
                {...(props.onContextRingOpen ? { onContextRingOpen: props.onContextRingOpen } : {})}
                {...(props.generating === true && props.onStop !== undefined
                  ? { generating: true, onStop: props.onStop }
                  : {})}
                {...(props.onQueue
                  ? {
                      queuedMessages: props.queuedMessages ?? [],
                      onQueue: props.onQueue,
                      ...(props.onCancelQueued
                        ? { onCancelQueued: props.onCancelQueued }
                        : {}),
                    }
                  : {})}
                {...(props.knownSkillNames ? { knownSkillNames: props.knownSkillNames } : {})}
                {...(props.onWorkspacePickFolder
                  ? {
                      workspaceCtx: props.workspaceCtx ?? null,
                      worktreePref: props.worktreePref ?? false,
                      svnFolderPref: props.svnFolderPref ?? false,
                      workspaceLocked: props.workspaceLocked ?? false,
                      onWorkspacePickFolder: props.onWorkspacePickFolder,
                      onWorkspacePickBranch: props.onWorkspacePickBranch,
                      onWorktreeToggle: props.onWorktreeToggle,
                      onWorkspacePickSvnBranch: props.onWorkspacePickSvnBranch,
                      onSvnFolderToggle: props.onSvnFolderToggle,
                    }
                  : {})}
              />
            )}
          </div>
        </div>
      ) : (
        <div className="chat-stack" ref={chatStackRef}>
          <div
            id="messages"
            className="chat-scroll"
            aria-live="polite"
            ref={messagesRef}
          >
            <div className="chat-scroll-sticky-head">
              <div className="chat-title-column">
                <ChatHeader
                  title={props.title}
                  editable={true}
                  onTitleSave={props.onTitleSave}
                  {...(props.onExportSession &&
                  hasExportableAssistant(props.items)
                    ? {
                        actions: (
                          <SessionExportMenu
                            onExport={props.onExportSession}
                            {...(props.exportBusy !== undefined
                              ? { busy: props.exportBusy }
                              : {})}
                          />
                        ),
                      }
                    : {})}
                />
              </div>
            </div>
            <div className="messages-inner">
              <MessageList
                items={props.items}
                sessionId={props.sessionId}
                generating={props.generating === true}
                {...(props.pathRoots !== undefined
                  ? { pathRoots: props.pathRoots }
                  : {})}
                {...(props.workspaceCtx?.path
                  ? { workspacePath: props.workspaceCtx.path }
                  : {})}
                {...(props.onOpenSession
                  ? { onOpenSubagentTranscript: props.onOpenSession }
                  : {})}
                {...(props.onFetchToolCallFull
                  ? { onFetchToolCallFull: props.onFetchToolCallFull }
                  : {})}
                {...(props.onQuestionPromptResolved
                  ? { onQuestionPromptResolved: props.onQuestionPromptResolved }
                  : {})}
                {...(props.onPermissionPromptResolved
                  ? {
                      onPermissionPromptResolved:
                        props.onPermissionPromptResolved,
                    }
                  : {})}
                {...(props.onPlanDocumentExpanded
                  ? { onPlanDocumentExpanded: props.onPlanDocumentExpanded }
                  : {})}
                {...(props.onPlanDocumentRun
                  ? { onPlanDocumentRun: props.onPlanDocumentRun }
                  : {})}
                {...(props.onPlanDocumentDiscard
                  ? { onPlanDocumentDiscard: props.onPlanDocumentDiscard }
                  : {})}
                {...(props.onEdit ? { onEdit: props.onEdit } : {})}
                {...(props.onBranchSwitch
                  ? { onBranchSwitch: props.onBranchSwitch }
                  : {})}
                {...(props.knownSkillNames ? { knownSkillNames: props.knownSkillNames } : {})}
                {...(props.backgroundTasksByToolCallId
                  ? {
                      backgroundTasksByToolCallId:
                        props.backgroundTasksByToolCallId,
                    }
                  : {})}
                {...(props.backgroundNowMs !== undefined
                  ? { backgroundNowMs: props.backgroundNowMs }
                  : {})}
                {...(props.onOpenBackgroundTask
                  ? { onOpenBackgroundTask: props.onOpenBackgroundTask }
                  : {})}
                {...(props.onStopBackgroundTask
                  ? { onStopBackgroundTask: props.onStopBackgroundTask }
                  : {})}
              />
              {props.backgroundTasks && props.onOpenBackgroundTasks ? (
                <BackgroundTasksChip
                  tasks={props.backgroundTasks}
                  onOpen={props.onOpenBackgroundTasks}
                />
              ) : null}
            </div>
            <div className="chat-scroll-tail" aria-hidden />
          </div>

          {/* Composer here always renders composer-wrap-docked (isEmpty={false});
              the marker class replaces :has(), unsupported in JCEF Chromium 104 */}
          <div className="chat-bottom chat-bottom--docked">
            <div className="chat-bottom-inner" ref={composerHostRef}>
              <ScrollToBottomButton
                visible={showScrollToBottom}
                onClick={jumpToNewestMessage}
              />
              {readOnlyNotice ? null : (
                <UsageBanner
                  usage={props.providerUsage}
                  modelId={props.llmModel ?? ""}
                  {...(props.usageBannerDismissedKey
                    ? { dismissedKey: props.usageBannerDismissedKey }
                    : {})}
                  {...(props.onUsageBannerDismiss
                    ? { onDismiss: props.onUsageBannerDismiss }
                    : {})}
                />
              )}
              {readOnlyNotice ?? (
                <Composer
                  value={props.draft}
                  providerUsage={props.providerUsage ?? null}
                  attachedFiles={attachedFiles}
                  onAttachedFilesChange={setAttachedFiles}
                  isEmpty={false}
                  sessionId={props.sessionId}
                  contextIdle={false}
                  mode={props.mode}
                  modes={props.modes}
                  tokenUsage={props.tokenUsage}
                  {...(props.contextPct !== undefined
                    ? { contextPct: props.contextPct }
                    : {})}
                {...(props.maxContextTokens !== undefined
                  ? { maxContextTokens: props.maxContextTokens }
                  : {})}
                {...(props.contextBreakdown !== undefined
                  ? { contextBreakdown: props.contextBreakdown }
                  : {})}
                {...(props.llmModels !== undefined &&
                  props.llmModels.length > 0 &&
                  props.onLlmModelChange !== undefined
                    ? {
                        llmModels: props.llmModels,
                        llmModel: props.llmModel,
                        onLlmModelChange: props.onLlmModelChange,
                        llmModelMultimodal: props.llmModelMultimodal,
                        ...(props.llmReasoningLevels !== undefined &&
                        props.llmReasoningLevels.length > 0 &&
                        props.onLlmReasoningChange !== undefined
                          ? {
                              llmReasoningLevels: props.llmReasoningLevels,
                              llmReasoning: props.llmReasoning,
                              onLlmReasoningChange: props.onLlmReasoningChange,
                            }
                          : {}),
                      }
                    : {})}
                  onModeChange={props.onModeChange}
                  onChange={props.onDraftChange}
                  onSend={props.onSend}
                  {...(props.onPasteChipCaptured
                    ? { onPasteChipCaptured: props.onPasteChipCaptured }
                    : {})}
                  {...(props.onContextRingOpen ? { onContextRingOpen: props.onContextRingOpen } : {})}
                  {...(props.generating === true && props.onStop !== undefined
                    ? { generating: true, onStop: props.onStop }
                    : {})}
                  {...(props.onQueue
                    ? {
                        queuedMessages: props.queuedMessages ?? [],
                        onQueue: props.onQueue,
                        ...(props.onCancelQueued
                          ? { onCancelQueued: props.onCancelQueued }
                          : {}),
                      }
                    : {})}
                  {...(props.knownSkillNames ? { knownSkillNames: props.knownSkillNames } : {})}
                  {...(props.editingFiles && props.editingFiles.length > 0
                    ? { editingFiles: props.editingFiles }
                    : {})}
                  {...(props.onWorkspacePickFolder
                    ? {
                        workspaceCtx: props.workspaceCtx ?? null,
                        worktreePref: props.worktreePref ?? false,
                        svnFolderPref: props.svnFolderPref ?? false,
                        workspaceLocked: props.workspaceLocked ?? false,
                        onWorkspacePickFolder: props.onWorkspacePickFolder,
                        onWorkspacePickBranch: props.onWorkspacePickBranch,
                        onWorktreeToggle: props.onWorktreeToggle,
                        onWorkspacePickSvnBranch: props.onWorkspacePickSvnBranch,
                        onSvnFolderToggle: props.onSvnFolderToggle,
                      }
                    : {})}
                />
              )}
            </div>
          </div>
        </div>
      )}
    </main>
  );
}

/**
 * True when the transcript holds at least one completed assistant answer. The
 * export action is gated on this so it only appears once there is something to
 * download; matches the `type === "assistant_message"` discriminant used by
 * MessageList and the streaming-sync local check.
 */
function hasExportableAssistant(items: TranscriptItem[]): boolean {
  return items.some(
    (it) => it.type === "assistant_message" && it.content.trim() !== "",
  );
}
