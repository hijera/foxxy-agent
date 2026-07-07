import type { CSSProperties } from "react";
import {
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
import { ChatHeader } from "./ChatHeader";
import { Composer } from "./Composer";
import { MessageList } from "../messages/MessageList";
import { useT } from "../i18n/I18nProvider";
import {
  subscribeShellStack,
  snapshotShellStack,
  serverSnapshotShellStack,
} from "../shellBreakpoint";
import { transcriptItemsAffectAutoScroll } from "./transcriptAutoScroll";

export function ChatScreen(props: {
  title: string;
  sessionId: string;
  /** Accent verb for "What do you want to …?" on the empty hero (session-stable or home rotation). */
  heroAccentVerb: HeroAccentVerb;
  /** Bumps when the user starts a fresh home chat so the composer can refocus. */
  heroComposerFocusEpoch: number;
  onTitleSave: (title: string) => void;
  items: TranscriptItem[];
  draft: string;
  tokenUsage: TokenUsage | null;
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
  onContextRingOpen?: () => void;
  generating?: boolean;
  onStop?: () => void;
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
  /** Basename of the current project folder (project pill in the header). */
  projectName?: string;
  /** Full project path for the pill tooltip. */
  projectPath?: string;
  onOpenProject?: () => void;
}) {
  const { t } = useT();
  const messagesRef = useRef<HTMLDivElement | null>(null);
  const composerHostRef = useRef<HTMLDivElement | null>(null);
  const isEmpty = props.items.length === 0;
  const showSkeleton = isEmpty && !!props.sessionLoading;
  const stickToBottomRef = useRef(true);
  const prevItemsForScrollRef = useRef<TranscriptItem[]>([]);
  const [composerReserve, setComposerReserve] = useState(200);
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
    const apply = () => {
      const h = host.getBoundingClientRect().height;
      setComposerReserve(Math.max(140, Math.ceil(h) + extra));
    };
    apply();
    const ro =
      typeof ResizeObserver !== "undefined" ? new ResizeObserver(apply) : null;
    ro?.observe(host);
    return () => ro?.disconnect();
  }, [isEmpty, props.tokenUsage]);

  useEffect(() => {
    if (isEmpty) return;
    const prev = prevItemsForScrollRef.current;
    prevItemsForScrollRef.current = props.items;
    if (!transcriptItemsAffectAutoScroll(prev, props.items)) {
      return;
    }
    if (!stickToBottomRef.current) return;
    if (mobileDocScroll) {
      const run = () => {
        const top = Math.max(
          document.body.scrollHeight,
          document.documentElement.scrollHeight,
        );
        window.scrollTo({ top, left: 0, behavior: "auto" });
      };
      requestAnimationFrame(() => requestAnimationFrame(run));
      return;
    }
    const el = messagesRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [props.items, isEmpty, mobileDocScroll]);

  useEffect(() => {
    if (isEmpty) return;
    const onScroll = () => {
      if (mobileDocScroll) {
        const doc = document.documentElement;
        const dist = doc.scrollHeight - window.scrollY - window.innerHeight;
        stickToBottomRef.current = dist < 80;
      } else {
        const el = messagesRef.current;
        if (!el) return;
        const dist = el.scrollHeight - el.scrollTop - el.clientHeight;
        stickToBottomRef.current = dist < 80;
      }
    };
    if (mobileDocScroll) {
      window.addEventListener("scroll", onScroll, { passive: true });
      return () => window.removeEventListener("scroll", onScroll);
    }
    const el = messagesRef.current;
    el?.addEventListener("scroll", onScroll, { passive: true });
    return () => el?.removeEventListener("scroll", onScroll);
  }, [isEmpty, mobileDocScroll]);

  const mainClassName = [
    "main",
    isEmpty && !showSkeleton ? "is-empty" : "",
    props.sessionFadingOut ? "session-fading-out" : "",
  ].filter(Boolean).join(" ");

  return (
    <main className={mainClassName}>
      {showSkeleton ? (
        <div className="chat-skeleton" aria-hidden="true">
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
      ) : isEmpty ? (
        <div className="hero" id="hero">
          {props.projectName && props.onOpenProject ? (
            <button
              type="button"
              className="project-pill project-pill--hero"
              title={props.projectPath || props.projectName}
              aria-label={t("project.pillTitle")}
              data-testid="project-pill-hero"
              onClick={props.onOpenProject}
            >
              <span className="project-pill-icon" aria-hidden>
                📁
              </span>
              <span className="project-pill-name">{props.projectName}</span>
            </button>
          ) : null}
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
            <Composer
              value={props.draft}
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
              {...(props.onContextRingOpen ? { onContextRingOpen: props.onContextRingOpen } : {})}
              {...(props.generating === true && props.onStop !== undefined
                ? { generating: true, onStop: props.onStop }
                : {})}
              {...(props.knownSkillNames ? { knownSkillNames: props.knownSkillNames } : {})}
            />
          </div>
        </div>
      ) : (
        <div
          className="chat-stack"
          style={
            {
              "--chat-composer-reserve": `${composerReserve}px`,
            } as CSSProperties
          }
        >
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
                  {...(props.projectName && props.onOpenProject
                    ? {
                        projectName: props.projectName,
                        projectPath: props.projectPath,
                        onOpenProject: props.onOpenProject,
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
              />
            </div>
            <div className="chat-scroll-tail" aria-hidden />
          </div>

          {/* Composer here always renders composer-wrap-docked (isEmpty={false});
              the marker class replaces :has(), unsupported in JCEF Chromium 104 */}
          <div className="chat-bottom chat-bottom--docked">
            <div className="chat-bottom-inner" ref={composerHostRef}>
              <Composer
                value={props.draft}
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
                {...(props.onContextRingOpen ? { onContextRingOpen: props.onContextRingOpen } : {})}
                {...(props.generating === true && props.onStop !== undefined
                  ? { generating: true, onStop: props.onStop }
                  : {})}
                {...(props.knownSkillNames ? { knownSkillNames: props.knownSkillNames } : {})}
                {...(props.editingFiles && props.editingFiles.length > 0
                  ? { editingFiles: props.editingFiles }
                  : {})}
              />
            </div>
          </div>
        </div>
      )}
    </main>
  );
}
