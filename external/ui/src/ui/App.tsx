import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import type { CSSProperties } from "react";
import { ChatScreen } from "./chat/ChatScreen";
import { useStableHandler } from "./components/useStableHandler";
import { contextUsagePercent, withContextUsedTokens } from "./chat/contextUsage";
import { HERO_ACCENT_VERBS, pickHeroAccentVerb } from "./chat/heroTitleWords";
import { markConnected, markReconnecting } from "./chat/liveConnectionState";
import { setLlmRetrying } from "./chat/llmRetryState";
import { setMcpConnecting } from "./chat/mcpConnectingState";
import { openAIStreamErrorMessage } from "./chat/streamError";
import { parseSSEBlocks } from "./chat/sse";
import { optimisticUserFiles } from "./chat/optimisticUserFiles";
import { sessionMessageFiles } from "./chat/sessionMessageFiles";
import { subscribeServerEvents } from "./chat/serverEvents";
import {
  consumeComposerSseReader,
  type ContextUsageUpdate,
} from "./chat/consumeComposerSse";
import {
  parseFoxxyCodePermissionPayload,
  type PermissionResolvedState,
} from "./chat/permissionTypes";
import {
  parseFoxxyCodeQuestionPayload,
  type QuestionResolvedState,
} from "./chat/questionTypes";
import { createDebouncedSessionStatsRefresh } from "./chat/sessionStatsPoll";
import { planSessionStatsApply } from "./chat/sessionTokenTotals";
import { stripCompactionPreamble } from "./chat/compactionSummary";
import { EnvHealthBanner } from "./env/EnvHealthBanner";
import { fetchJSON } from "./env/fetchJSON";
import {
  preserveTranscriptItemIds,
  stablePermissionPromptItemId,
  stableThinkingItemId,
  stableToolCallItemId,
  stableUserItemId,
} from "./chat/transcriptItemIds";
import {
  appendDeferredAssistant,
  deferredAssistantItem,
  emptyDeferredAssistant,
} from "./chat/transcriptDeferredAssistant";
import {
  dedupeAdjacentDuplicateThinkingCompleted,
  keepLocalTranscriptIfServerEmpty,
  mergeTranscriptPreferLocalSuffix,
  preserveUserMessageFiles,
  revokeSupersededUserMessagePreviews,
} from "./chat/transcriptServerSnapshot";
import { pinPlanDocumentsToTurnEnd } from "./chat/planDocumentPlacement";
import { pickStreamMutationBase } from "./chat/streamMutationBase";
import { ShadowTranscriptCache } from "./chat/sessionTranscriptCache";
import {
  parseSubagentTranscriptMeta,
  type SubagentTranscriptMeta,
} from "./chat/subagentTranscript";
import { shouldApplyTranscriptSnapshot } from "./chat/transcriptSnapshotGuard";
import {
  shouldAdoptServerModeAfterTurn,
  shouldApplySessionSelection,
} from "./chat/sessionSelectionApply";
import {
  mergePermissionPromptsIntoTranscript,
  permissionPendingSessionIdsFromStorage,
  resolvedPermissionToolCallIds,
  upsertPermissionPromptRecord,
} from "./chat/permissionPromptSessionStore";
import { permissionPromptInsertIndex } from "./chat/permissionPromptPlacement";
import { createTurnReplayTrimGate } from "./chat/transcriptTurnTrim";
import {
  diskFallbackDelayMs,
  isNoLiveTurnRelayError,
  liveReconnectDelayMs,
  parseSessionBusyResponse,
  shouldKeepWatching,
  shouldReloadTranscript,
} from "./chat/liveTurnRecovery";
import {
  parseToolsPermissionPolicy,
  type ToolsPermissionPolicy,
} from "./chat/toolsPermissionPolicy";
import { reattachLocalQuestionPrompts } from "./chat/transcriptQuestionReattach";
import {
  clearQuestionPromptRecords,
  loadQuestionPromptRecords,
  mergeStoredQuestionPromptsIntoTranscript,
  patchQuestionToolArgsFromPromptRecords,
  pickRicherQuestionToolArgs,
  upsertQuestionPromptRecord,
} from "./chat/questionPromptSessionStore";
import { pickRicherToolArgs } from "./chat/toolCallArgs";
import { normalizeTodoPlanSnapshot } from "./chat/todoToolPreview";
import { transcriptHasFilledAssistant } from "./chat/streamSyncLocalAssistant";
import { stableMemoryCopilotItemId } from "./chat/memoryStableId";
import type { TokenUsage, TranscriptItem } from "./chat/types";
import type { ProviderUsage } from "./chat/providerUsage";
import { connectSwarmNode, getEnv, returnToSwarm } from "./env/remoteEnv";
import { EnvironmentChip } from "./chat/EnvironmentChip";
import { probeSwarm } from "./swarm/api";
import { SwarmView } from "./swarm/SwarmView";
import { useProviderUsage } from "./chat/useProviderUsage";
import type { WorkspaceContext } from "./chat/workspaceContext";
import {
  injectBranchNavItems,
  deduplicateBranchNavs,
  type BranchPointData,
} from "./chat/branchInject";
import { resolveLatestLeaf } from "./chat/resolveLatestLeaf";
import { NavRail } from "./nav/NavRail";
import {
  fetchOnboardingStatus,
  shouldShowOnboarding,
} from "./onboarding/onboardingStatus";
import { ProviderPickerDialog } from "./onboarding/ProviderPickerDialog";
import { GuidedTour } from "./onboarding/GuidedTour";
import { TOUR_STEPS } from "./onboarding/tourSteps";
import { isTourSeen, markTourSeen, resetTour } from "./onboarding/tourState";
import { isDesktopShell } from "./desktopShell";
import {
  DesktopNotifications,
  type DesktopNotification,
  type DesktopPermissionNotification,
  type DesktopPlanNotification,
} from "./desktop/DesktopNotifications";
import {
  armNotificationSoundUnlock,
  playNotificationSound,
} from "./desktop/desktopNotifySound";
import { permissionPromptDetail } from "./chat/permissionPromptDisplay";
import { buildPermissionToolPreview } from "./chat/permissionToolPreview";
import { submitPermissionChoice } from "./chat/permissionSubmit";
import { readNavRailCookie, writeNavRailCookie } from "./nav/navRailCookie";
import { readLlmModelCookie, writeLlmModelCookie } from "./chat/llmModelCookie";
import {
  pickDefaultLlmModelForNewChat,
  pickLlmModelForOpenSession,
} from "./chat/llmModelSelection";
import {
  readReasoningCookie,
  writeReasoningCookie,
} from "./chat/reasoningCookie";
import { pickReasoningLevel } from "./chat/reasoningSelection";
import { SessionsSidebar } from "./sessions/SessionsSidebar";
import { useConfirm } from "./components/useConfirm";
import type { SessionRow } from "./sessions/types";
import {
  isClientDraftSessionId,
  mergeSessionsWithDrafts,
  newClientDraftId,
  readClientDraftSessions,
  removeClientDraftSession,
  upsertClientDraftSession,
  type ClientDraftSession,
} from "./sessions/draftSessions";
import {
  isRedundantSessionPick,
  shouldCloseHistoryOnSessionPick,
} from "./sessions/pickSessionGuard";
import {
  readProjectOnlyPref,
  sessionsProjectCwdParam,
  writeProjectOnlyPref,
} from "./sessions/sessionsProjectFilter";
import {
  fetchLastProjectSession,
  recordLastProjectSession,
  shouldRestoreLastSession,
} from "./sessions/lastProjectSession";
import { isEditorEmbed } from "./embedShell";
import { scheduleSessionTitleRefresh } from "./sessionTitleSuggest";
import { extractAtFileAttachments } from "./skills/draftAt";
import {
  extractSessionAssetsXml,
  parseSessionAssetFiles,
  stripFoxxyCodeAttachmentsForUserDisplay,
} from "./skills/stripFoxxyCodeAttachments";
import {
  migrateWorkspaceAtRecents,
  recordWorkspaceAtRecent,
  WORKSPACE_AT_RECENTS_NO_SESSION_KEY,
} from "./skills/workspaceAtRecents";
import {
  schedulerCancelJob,
  schedulerListJobs,
  schedulerRunJob,
} from "./scheduler/api";
import {
  parseAppHash,
  setMiniAppsHash,
  setDraftHashInLocation,
  setHistoryHash,
  setSessionHashInLocation,
  schedulerEditorFromParsedHash,
  setSchedulerCreateHash,
  setSchedulerJobHash,
  setSchedulerListHash,
  setSessionTasksHash,
  setSettingsHash,
  stripHistorySidebarFromHash,
  appNavHrefSwarm,
} from "./scheduler/hashRoute";
import { MiniAppsPage } from "./miniapps/MiniAppsPage";
import { isMiniAppSessionEligible } from "./miniapps/sessionEligibility";
import { SchedulerJobEditorSheet } from "./scheduler/SchedulerJobEditorSheet";
import { SchedulerJobsDrawer } from "./scheduler/SchedulerJobsDrawer";
import { BackgroundTasksPanel } from "./tasks/BackgroundTasksPanel";
import {
  clearFinishedBackgroundTasks,
  getBackgroundTask,
  listBackgroundTasks,
  stopBackgroundTask,
} from "./tasks/api";
import { awaitingPermissionCount, tasksPollIntervalMs } from "./tasks/taskStatus";
import type { BackgroundTask } from "./tasks/types";
import type { SchedulerInfo, SchedulerJob } from "./scheduler/types";
import { Settings } from "./settings/Settings";
import { downloadBlob } from "./settings/transferIO";
import { t } from "./i18n/i18n";
import { useT } from "./i18n/I18nProvider";
import type { ExportFormat } from "./chat/SessionExportMenu";

const HDR = "X-FoxxyCode-Session-ID";

/**
 * How often the open chat is asked whether it is working, while this client is not
 * streaming it. Slow enough to be invisible on an idle chat, quick enough that a turn
 * started without us - a background task waking the agent - shows up as progress rather
 * than as a chat that sat still and then jumped.
 */
const VIEWED_ACTIVITY_POLL_MS = 4000;

async function markFoxxyCodeSessionActivityRead(id: string): Promise<void> {
  const t = id.trim();
  if (!t) {
    return;
  }
  try {
    await fetch(`/foxxycode/sessions/${encodeURIComponent(t)}`, {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
        [HDR]: t,
      },
      body: JSON.stringify({ markActivityRead: true }),
    });
  } catch {
    // ignore
  }
}

/** Poll job list while scheduler UI is open (running, next_run_utc, paused). */
const SCHEDULER_JOBS_POLL_MS = 12_000;

type SchedulerEditorState =
  | null
  | { mode: "create" }
  | { mode: "edit"; jobId: string };

type ToolCallUpdate = {
  toolCallId: string;
  title?: string;
  kind?: string;
  status?: string;
};

type ToolCallStatusUpdate = {
  toolCallId: string;
  status?: string;
  content?: Array<{ type: string; content: { type: string; text?: string } }>;
  _meta?: {
    foxxycode?: {
      toolResultPreview?: { truncated?: boolean; totalLines?: number };
    };
  };
};

type ToolCallListRow = {
  toolCallId: string;
  name?: string;
  kind?: string;
  status?: string;
  startedAt?: string;
  finishedAt?: string;
  argsPreview?: string;
  resultPreview?: string;
  resultPreviewTruncated?: boolean;
  planSnapshot?: unknown;
};

function readMessageCreatedAtUTC(
  m: Record<string, unknown>,
): string | undefined {
  const raw = m.created_at ?? m.createdAt;
  if (typeof raw !== "string") {
    return undefined;
  }
  const s = raw.trim();
  return s === "" ? undefined : s;
}

function toolSseShowsTruncatedPreview(u: ToolCallStatusUpdate): boolean {
  const p = u._meta?.foxxycode?.toolResultPreview;
  return !!(p && p.truncated === true);
}

type MemoryPhaseEvt = {
  memoryRowId: string;
  phase: string;
  status: string;
  userTurnIndex?: number;
  durationMs?: number;
  persistSaved?: boolean;
  persistRelativePath?: string;
  persistTitle?: string;
  persistSavedBody?: string;
  recallReadPaths?: string[];
};

type MemoryChunkEvt = {
  memoryRowId: string;
  phase: string;
  kind: string;
  delta: string;
};

type MemoryTurnApi = {
  userTurnIndex: number;
  memoryRowId?: string;
  memoryMode?: string;
  memoryDurationMs?: number;
  memoryContextText?: string;
  recallSkipped?: boolean;
  recallText?: string;
  recallReasoningText?: string;
  recallDurationMs?: number;
  persistJudgeText?: string;
  persistDurationMs?: number;
  persistSaved?: boolean;
  persistRelativePath?: string;
  persistTitle?: string;
  persistSavedBody?: string;
  recallReadPaths?: string[];
};

type ModelInfo = {
  id: string;
  ownedBy?: string;
  maxContextTokens?: number | undefined;
  multimodal?: boolean;
  reasoningLevels?: string[];
  reasoningDefault?: string;
};

const PROFILE_MODES = ["agent", "plan", "docs", "ask", "debug"] as const;

type SessionStats = {
  tokenUsageTotal?: {
    inputTokens: number;
    outputTokens: number;
    totalTokens: number;
  };
  contextBreakdown?: {
    systemPrompt: number;
    toolDefinitions: number;
    rules: number;
    skills: number;
    mcp: number;
    subagents: number;
    conversation: number;
    summary: number;
    estimatedTotal: number;
  };
};

function randomSessionId(): string {
  const hex = [...crypto.getRandomValues(new Uint8Array(18))]
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
  return `sess_${hex}`;
}

function newId(prefix: string): string {
  return `${prefix}_${Date.now().toString(36)}_${Math.random().toString(16).slice(2)}`;
}

function memoryTranscriptFromApi(
  row: MemoryTurnApi,
): Extract<TranscriptItem, { type: "memory_copilot" }> {
  const rowId = (row.memoryRowId || "").trim() || `mem-${row.userTurnIndex}`;
  const unifiedCtx = (row.memoryContextText || "").trim();
  const rt = (row.recallText || "").trim();
  const rr = (row.recallReasoningText || "").trim();
  const paths = Array.isArray(row.recallReadPaths)
    ? row.recallReadPaths.filter(
        (x) => typeof x === "string" && x.trim() !== "",
      )
    : [];
  const hasRecallTrail = !!(
    row.recallDurationMs ||
    rt ||
    rr ||
    paths.length > 0
  );
  const pt = (row.persistJudgeText || "").trim();
  const hasPersistTrail = !!(row.persistDurationMs || pt || row.persistSaved);
  const hasUnified = !!(row.memoryDurationMs || unifiedCtx);
  const sumMs =
    typeof row.memoryDurationMs === "number" && row.memoryDurationMs > 0
      ? row.memoryDurationMs
      : (row.recallDurationMs ?? 0) + (row.persistDurationMs ?? 0);
  const legacyCombined = [row.recallText, row.persistJudgeText]
    .filter((x) => typeof x === "string" && x.trim() !== "")
    .join("\n\n");
  const memoryTextOut = unifiedCtx || legacyCombined;
  return {
    id: stableMemoryCopilotItemId(rowId, row.userTurnIndex),
    type: "memory_copilot",
    memoryRowId: rowId,
    userTurnIndex: row.userTurnIndex,
    ...(hasUnified
      ? { memoryStatus: "completed" as const, memoryText: memoryTextOut }
      : {}),
    recallStatus: hasRecallTrail ? "completed" : "idle",
    persistStatus: hasPersistTrail ? "completed" : "idle",
    recallText: row.recallText || "",
    recallReasoning: row.recallReasoningText || "",
    persistText: row.persistJudgeText || "",
    persistReasoning: "",
    ...(typeof row.recallDurationMs === "number"
      ? { recallDurationMs: row.recallDurationMs }
      : {}),
    ...(typeof row.persistDurationMs === "number"
      ? { persistDurationMs: row.persistDurationMs }
      : {}),
    ...(sumMs > 0 ? { memoryWallDurationMs: sumMs } : {}),
    ...(typeof row.persistSaved === "boolean"
      ? { persistSaved: row.persistSaved }
      : {}),
    ...(row.persistRelativePath
      ? { persistRelativePath: row.persistRelativePath }
      : {}),
    ...(row.persistTitle ? { persistTitle: row.persistTitle } : {}),
    ...(row.persistSavedBody ? { persistSavedBody: row.persistSavedBody } : {}),
    ...(paths.length > 0 ? { recallReadPaths: paths } : {}),
  };
}

function applyMemoryPhaseToItems(
  prev: TranscriptItem[],
  p: MemoryPhaseEvt,
): TranscriptItem[] {
  const now = Date.now();
  let idx = prev.findIndex(
    (x) => x.type === "memory_copilot" && x.memoryRowId === p.memoryRowId,
  );
  const next = [...prev];
  let uidx = -1;
  for (let i = prev.length - 1; i >= 0; i--) {
    const it = prev[i];
    if (it && it.type === "user_message") {
      uidx = i;
      break;
    }
  }
  const insertAt = uidx >= 0 ? uidx + 1 : next.length;

  const baseMemory = (): Extract<
    TranscriptItem,
    { type: "memory_copilot" }
  > => ({
    id: stableMemoryCopilotItemId(
      p.memoryRowId,
      typeof p.userTurnIndex === "number" ? p.userTurnIndex : 0,
    ),
    type: "memory_copilot",
    memoryRowId: p.memoryRowId,
    userTurnIndex: typeof p.userTurnIndex === "number" ? p.userTurnIndex : 0,
    memoryStatus: "idle",
    memoryText: "",
    recallStatus: "idle",
    persistStatus: "idle",
    recallText: "",
    recallReasoning: "",
    persistText: "",
    persistReasoning: "",
  });

  if (idx < 0) {
    next.splice(insertAt, 0, baseMemory());
    idx = insertAt;
  }

  const cur = next[idx];
  if (!cur || cur.type !== "memory_copilot") {
    return prev;
  }

  let patch: Extract<TranscriptItem, { type: "memory_copilot" }> = { ...cur };
  const st = (p.status || "").trim();

  if (p.phase === "memory") {
    if (st === "started") {
      patch.memoryStatus = "in_progress";
      patch.recallStatus = "in_progress";
      patch.persistStatus = "idle";
      if (patch.memoryWallStartedAtMs == null)
        patch.memoryWallStartedAtMs = now;
    }
    if (st === "completed") {
      patch.memoryStatus = "completed";
      patch.recallStatus = "completed";
      patch.persistStatus = p.persistSaved ? "completed" : "idle";
      const rp = p.recallReadPaths;
      if (Array.isArray(rp) && rp.length > 0) {
        const cleaned = rp.map((x) => String(x).trim()).filter((x) => x !== "");
        if (cleaned.length > 0) patch.recallReadPaths = cleaned;
      }
      if (typeof p.persistSaved === "boolean") {
        patch.persistSaved = p.persistSaved;
      }
      const pr = (p.persistRelativePath || "").trim();
      if (pr) patch.persistRelativePath = pr;
      const tt = (p.persistTitle || "").trim();
      if (tt) patch.persistTitle = tt;
      const pb = (p.persistSavedBody || "").trim();
      if (pb) patch.persistSavedBody = pb;
      if (typeof patch.memoryWallStartedAtMs === "number") {
        patch.memoryWallDurationMs = Math.max(
          0,
          now - patch.memoryWallStartedAtMs,
        );
      }
    }
  }
  if (p.phase === "recall") {
    if (st === "started") {
      patch.recallStatus = "in_progress";
      if (patch.memoryWallStartedAtMs == null)
        patch.memoryWallStartedAtMs = now;
    }
    if (st === "completed") {
      patch.recallStatus = "completed";
      if (typeof p.durationMs === "number" && p.durationMs > 0)
        patch.recallDurationMs = p.durationMs;
      const rp = p.recallReadPaths;
      if (Array.isArray(rp) && rp.length > 0) {
        const cleaned = rp.map((x) => String(x).trim()).filter((x) => x !== "");
        if (cleaned.length > 0) patch.recallReadPaths = cleaned;
      }
    }
  }
  if (p.phase === "persist") {
    if (st === "started") {
      patch.persistStatus = "in_progress";
      if (patch.memoryWallStartedAtMs == null)
        patch.memoryWallStartedAtMs = now;
      const wallStart = patch.memoryWallStartedAtMs;
      const wallElapsed =
        typeof wallStart === "number" ? Math.max(0, now - wallStart) : 0;
      if (
        typeof patch.memoryWallLiveCapMs === "number" &&
        Number.isFinite(patch.memoryWallLiveCapMs)
      ) {
        patch.memoryWallLiveCapMs = Math.max(
          patch.memoryWallLiveCapMs,
          wallElapsed,
        );
      } else {
        patch.memoryWallLiveCapMs = wallElapsed;
      }
    }
    if (st === "completed") {
      patch.persistStatus = "completed";
      if (typeof p.durationMs === "number" && p.durationMs > 0)
        patch.persistDurationMs = p.durationMs;
      if (typeof p.persistSaved === "boolean") {
        patch.persistSaved = p.persistSaved;
      }
      const pr = (p.persistRelativePath || "").trim();
      if (pr) patch.persistRelativePath = pr;
      const tt = (p.persistTitle || "").trim();
      if (tt) patch.persistTitle = tt;
      const pb = (p.persistSavedBody || "").trim();
      if (pb) patch.persistSavedBody = pb;
      if (typeof patch.memoryWallStartedAtMs === "number") {
        patch.memoryWallDurationMs = Math.max(
          0,
          now - patch.memoryWallStartedAtMs,
        );
      }
    }
  }

  next[idx] = patch;
  return next;
}

function applyMemoryChunkToItems(
  prev: TranscriptItem[],
  c: MemoryChunkEvt,
): TranscriptItem[] {
  const idx = prev.findIndex(
    (x) => x.type === "memory_copilot" && x.memoryRowId === c.memoryRowId,
  );
  if (idx < 0) return prev;
  const cur = prev[idx];
  if (!cur || cur.type !== "memory_copilot") return prev;
  const next = [...prev];
  const patch: Extract<TranscriptItem, { type: "memory_copilot" }> = { ...cur };
  const ph = (c.phase || "").trim();
  const kd = (c.kind || "").trim();
  const d = typeof c.delta === "string" ? c.delta : "";
  if (!d) return prev;
  if (ph === "memory") {
    if (kd !== "reasoning") patch.memoryText = (patch.memoryText || "") + d;
  } else if (ph === "recall") {
    if (kd !== "reasoning") patch.recallText += d;
  } else if (ph === "persist") {
    if (kd !== "reasoning") patch.persistText += d;
  } else {
    return prev;
  }
  next[idx] = patch;
  return next;
}

/** Freeze the memory wall-clock label once main-model reasoning starts while recall/persist are still SSE-busy (events can arrive after reasoning deltas). */
function freezeMemoryWallWhenThinkingAfterRecall(
  items: TranscriptItem[],
  freezeAtMs: number,
): TranscriptItem[] {
  let userIdx = -1;
  for (let i = items.length - 1; i >= 0; i--) {
    const it = items[i];
    if (!it) continue;
    if (it && it.type === "user_message") {
      userIdx = i;
      break;
    }
  }
  if (userIdx < 0) return items;

  let memIdx = -1;
  let thinkingIdx = -1;
  for (let i = userIdx + 1; i < items.length; i++) {
    const it = items[i];
    if (!it) continue;
    if (it.type === "user_message") break;
    if (it.type === "memory_copilot") memIdx = i;
    if (it.type === "thinking" && it.status === "in_progress") {
      thinkingIdx = i;
      break;
    }
  }
  if (memIdx < 0 || thinkingIdx < 0) return items;

  const m = items[memIdx];
  if (!m || m.type !== "memory_copilot") return items;

  const memBusy =
    m.memoryStatus === "in_progress" ||
    m.recallStatus === "in_progress" ||
    m.persistStatus === "in_progress";
  if (!memBusy || typeof m.memoryWallLiveCapMs === "number") return items;

  const startMs = m.memoryWallStartedAtMs;
  if (typeof startMs !== "number") return items;

  const cap = Math.max(0, freezeAtMs - startMs);
  const next = [...items];
  next[memIdx] = { ...m, memoryWallLiveCapMs: cap };
  return next;
}

function parseRFC3339ms(s: string | undefined): number | null {
  const t = (s || "").trim();
  if (!t) return null;
  const ms = Date.parse(t);
  return Number.isFinite(ms) ? ms : null;
}

function reasoningDurationCacheKey(text: string): string {
  return text.trim().replace(/\s+/g, " ");
}

/**
 * Parse the filename from a `Content-Disposition: attachment; filename="..."`
 * header, decoding the percent-encoded form the server uses for non-ASCII
 * names. Returns null when the header is missing or carries no filename.
 */
function filenameFromDisposition(header: string | null): string | null {
  if (!header) {
    return null;
  }
  const m = /filename="([^"]+)"/i.exec(header);
  if (!m || !m[1]) {
    return null;
  }
  try {
    return decodeURIComponent(m[1]);
  } catch {
    return m[1];
  }
}

export function App() {
  // Only the active locale id is needed here: memoized labels below must recompute when the user
  // switches language. Translations themselves go through the module-level t().
  const { locale } = useT();
  const confirm = useConfirm();
  const [knownSkillNames, setKnownSkillNames] = useState<Set<string>>(
    () => new Set(),
  );
  const [sessionId, setSessionId] = useState("");
  /** Increments on each explicit "new chat" home transition so the hero verb rotates. */
  const [heroHomeGeneration, setHeroHomeGeneration] = useState(() =>
    Math.floor(Math.random() * HERO_ACCENT_VERBS.length),
  );
  const [sessions, setSessions] = useState<SessionRow[]>([]);
  const [sessionsCursor, setSessionsCursor] = useState<string | null>(null);
  const sessionsCursorRef = useRef<string | null>(null);
  const [sessionsError, setSessionsError] = useState<string | null>(null);
  const [items, setItems] = useState<TranscriptItem[]>([]);
  const [sessionLoading, setSessionLoading] = useState(false);
  const [sessionFadingOut, setSessionFadingOut] = useState(false);
  const [exportBusy, setExportBusy] = useState(false);
  const fadeOutTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const itemsRef = useRef<TranscriptItem[]>([]);
  itemsRef.current = items;
  const [editingUserMsgIdx, setEditingUserMsgIdx] = useState<number | null>(
    null,
  );
  const [editingAssetNote, setEditingAssetNote] = useState("");
  const [editingFiles, setEditingFiles] = useState<
    { name: string; mimeType: string }[]
  >([]);
  const pendingBranchSendRef = useRef<{ text: string; sid: string } | null>(
    null,
  );
  // Sessions explicitly chosen via branch nav — skip resolveLatestLeaf for these.
  const skipLeafResolveRef = useRef<Set<string>>(new Set());
  const [draft, setDraft] = useState("");
  // Paste-to-chip literals: `path:start-end` → the exact text the user pasted,
  // attached verbatim at send time. Lost on reload — the backend then re-reads
  // the line range from the file instead. Capped; cleared after each send.
  const pasteLiteralsRef = useRef<Map<string, string>>(new Map());
  // Workspace context chips: folder / git branch / worktree state per session.
  const [workspaceCtx, setWorkspaceCtx] = useState<WorkspaceContext | null>(null);
  const [worktreePref, setWorktreePref] = useState(false);
  // Subversion has no worktrees: this is the "check the branch out into its own
  // folder" preference behind the checkbox next to the SVN chip.
  const [svnFolderPref, setSvnFolderPref] = useState(false);
  // Pre-session workspace choices, applied right before the first send creates the session.
  const pendingWorkspaceRef = useRef<{
    path?: string;
    branch?: string;
    worktree?: boolean;
    vcs?: "git" | "svn";
  } | null>(null);
  const [clientDraftSessions, setClientDraftSessions] = useState<
    ClientDraftSession[]
  >(() => readClientDraftSessions());
  const [activeDraftId, setActiveDraftId] = useState("");
  const [permissionPendingSids, setPermissionPendingSids] = useState<
    Set<string>
  >(() => new Set(permissionPendingSessionIdsFromStorage()));
  // Desktop-only bottom-right toasts (permission prompts + plan-ready).
  const [desktopNotifications, setDesktopNotifications] = useState<
    DesktopNotification[]
  >([]);
  // Plan-ready keys (`${sessionId}:${slug}`) already surfaced as a toast, so we
  // don't re-notify on message reloads or when re-opening an old session.
  const notifiedPlanKeysRef = useRef<Set<string>>(new Set());
  const prevGeneratingRef = useRef(false);
  const prevGenSessionRef = useRef("");
  const [toolsPermissionPolicy, setToolsPermissionPolicy] =
    useState<ToolsPermissionPolicy | null>(null);
  const toolsPermissionPolicyRef = useRef<ToolsPermissionPolicy | null>(null);
  const [questionPendingSids, setQuestionPendingSids] = useState<Set<string>>(
    () => new Set(),
  );
  const [tokenUsage, setTokenUsage] = useState<TokenUsage | null>(null);
  /**
   * The viewed session has a turn running that this client is not streaming: a turn that
   * outlived its request, or the autonomous turn a finished background task woke. The
   * context ring has to keep updating through those, and `generating` cannot say so - it
   * only means "a composer stream is attached here".
   */
  const [viewedTurnActive, setViewedTurnActive] = useState(false);
  const [contextBreakdown, setContextBreakdown] = useState<NonNullable<
    SessionStats["contextBreakdown"]
  > | null>(null);

  const applySessionStatsPayload = useCallback(
    (
      stats: SessionStats | null | undefined,
      viewing: boolean,
      sessionKey?: string,
    ) => {
      if (!viewing) {
        return;
      }
      // While this client streams the turn, tokenUsage is baseline + the turn's live
      // deltas. Reseeding the baseline from a total that already counts the turn would
      // add it twice, so only the context breakdown is refreshed then.
      const key = (sessionKey ?? viewedSessionIdRef.current).trim();
      const plan = planSessionStatsApply(stats, {
        liveStreamAttached: key !== "" && activeComposerSidRef.current.has(key),
      });
      if (plan.tokenUsage) {
        tokenBaselineRef.current = {
          input: plan.tokenUsage.inputTokens,
          output: plan.tokenUsage.outputTokens,
          total: plan.tokenUsage.totalTokens,
        };
        setTokenUsage(plan.tokenUsage);
      }
      if (plan.contextBreakdown) {
        setContextBreakdown(plan.contextBreakdown);
      }
    },
    [],
  );

  const refreshSessionStats = useCallback(
    async (sid: string) => {
      const key = sid.trim();
      if (!key) {
        return;
      }
      const statsRes = await fetchJSON<{ stats?: SessionStats | null }>(
        `/foxxycode/sessions/${encodeURIComponent(key)}/stats`,
        { headers: { [HDR]: key } },
      );
      if (!statsRes.ok) {
        return;
      }
      applySessionStatsPayload(
        statsRes.data?.stats,
        viewedSessionIdRef.current.trim() === key,
        key,
      );
    },
    [applySessionStatsPayload],
  );

  const debouncedRefreshSessionStats = useMemo(
    () =>
      createDebouncedSessionStatsRefresh((sid) => {
        void refreshSessionStats(sid);
      }),
    [refreshSessionStats],
  );

  const markViewedSessionActivityRead = useCallback((sid: string) => {
    const key = sid.trim();
    if (!key) return;
    if (viewedSessionIdRef.current.trim() !== key) return;
    void markFoxxyCodeSessionActivityRead(key);
  }, []);
  const tokenBaselineRef = useRef<{
    input: number;
    output: number;
    total: number;
  }>({ input: 0, output: 0, total: 0 });
  /**
   * Per-session shadow transcript while that session streams in the
   * background, kept as a small LRU (see sessionTranscriptCache.ts). Every
   * write through `set` records recency, so `evictStaleSessionCaches` sees
   * every entry.
   */
  const streamShadowBySidRef = useRef(
    new ShadowTranscriptCache<TranscriptItem[]>(),
  );
  const postAbortBySidRef = useRef<Map<string, AbortController>>(new Map());
  const relayAbortBySidRef = useRef<Map<string, AbortController>>(new Map());
  /** Last composer relay frame id seen per session, so a re-attach can resume from it. */
  const relayLastEventIdBySidRef = useRef<Map<string, string>>(new Map());
  const streamingAssistantBySidRef = useRef<Map<string, string>>(new Map());
  /** Session ids with an active client-side composer POST or GET relay. */
  const activeComposerSidRef = useRef<Set<string>>(new Set());
  /** Session ids the user explicitly stopped, so a dropped stream is not auto-rejoined. */
  const userStoppedSidRef = useRef<Set<string>>(new Set());
  /** Bounded per-session retry counter for auto-reconnecting a dropped live stream. */
  const liveReconnectAttemptsRef = useRef<Map<string, number>>(new Map());
  // Consecutive failed /activity probes per session. Reconnects and the disk
  // poll give up on this rather than on an attempt count: while the server
  // still answers that the turn is alive, there is something to come back to.
  const activityFailuresRef = useRef<Map<string, number>>(new Map());
  // Last messageSeq seen per session, so a poll tick can skip a transcript
  // reload that would return exactly what is already rendered.
  const lastMessageSeqRef = useRef<Map<string, number>>(new Map());
  /** Per-session interval ids for the persisted-transcript fallback poller. */
  const diskFallbackTimerRef = useRef<Map<string, number>>(new Map());
  /**
   * Invalidates asynchronous transcript snapshots when a newer POST or relay
   * owns the session. This keeps late GET /messages responses from replacing
   * the live stream's ordering and assistant identity.
   */
  const transcriptEpochBySidRef = useRef<Map<string, number>>(new Map());
  /** Latest scheduleLiveStreamReconnect (assigned each render, declared later). */
  const scheduleLiveStreamReconnectRef = useRef<
    (sid: string, delayMs?: number) => void
  >(() => {});
  const [composerActivityEpoch, setComposerActivityEpoch] = useState(0);
  /** Session id currently shown in the transcript (updated synchronously on navigation). */
  const viewedSessionIdRef = useRef("");
  /** True while GET /foxxycode/events is connected; gates the fallback sessions poll. */
  const [serverEventsConnected, setServerEventsConnected] = useState(false);
  const serverEventHandlersRef = useRef<{
    turnStarted: (sid: string) => void;
    turnEnded: (sid: string) => void;
    providerUsage: (usage: ProviderUsage) => void;
    configReloaded: () => void;
  }>({
    turnStarted: () => {},
    turnEnded: () => {},
    providerUsage: () => {},
    configReloaded: () => {},
  });
  /** Set once the editor-embed last-session probe finished (or was skipped). */
  const lastSessionRestoreDoneRef = useRef(false);
  /** Last value sent to the last-session record; null until the first write. */
  const lastRecordedSessionRef = useRef<string | null>(null);
  const bumpComposerActivity = () =>
    setComposerActivityEpoch((n) => (n + 1) % 1_000_000_000);

  function addActiveComposer(sid: string) {
    const k = sid.trim();
    if (!k) return;
    // A stream is attached, so the live-status label stops saying "reconnecting".
    markConnected(k);
    if (activeComposerSidRef.current.has(k)) return;
    activeComposerSidRef.current.add(k);
    bumpComposerActivity();
  }

  function removeActiveComposer(sid: string) {
    const k = sid.trim();
    if (!k) return;
    // Backstop for the live-status label: every turn ends through here, however it ended.
    markConnected(k);
    setMcpConnecting(k, false);
    setLlmRetrying(k, false);
    if (!activeComposerSidRef.current.delete(k)) return;
    bumpComposerActivity();
  }

  function currentTranscriptEpoch(sid: string): number {
    return transcriptEpochBySidRef.current.get(sid.trim()) ?? 0;
  }

  function bumpTranscriptEpoch(sid: string): number {
    const key = sid.trim();
    if (!key) return 0;
    const next = currentTranscriptEpoch(key) + 1;
    transcriptEpochBySidRef.current.set(key, next);
    return next;
  }

  function applyStreamItemsForSession(
    streamSid: string,
    fn: (prev: TranscriptItem[]) => TranscriptItem[],
  ) {
    const key = streamSid.trim();
    if (!key) return;
    const viewing = viewedSessionIdRef.current.trim();
    const base = pickStreamMutationBase({
      mutationSessionId: key,
      viewingSid: viewing,
      shadow: streamShadowBySidRef.current.get(key),
      hasActiveComposer: activeComposerSidRef.current.has(key),
      itemsWhenViewingMatches: itemsRef.current,
    });
    const prevShadowLen = streamShadowBySidRef.current.get(key)?.length ?? 0;
    // Single funnel for every live stream mutation: keep plan cards at the end of
    // their turn so streamed text lands above the Run plan button, not below it.
    const next = pinPlanDocumentsToTurnEnd(fn(base));
    if (next.length === 0 && prevShadowLen > 0 && base.length === 0) {
      return;
    }
    streamShadowBySidRef.current.set(key, next);
    if (viewing === key) {
      itemsRef.current = next;
    }
    setItems((prev) => {
      const v = viewedSessionIdRef.current.trim();
      if (v === key) {
        return next;
      }
      return prev;
    });
  }

  /**
   * Drops least-recently-used shadow transcripts beyond the cache cap so old
   * dialogs stop accumulating in memory. The session about to be viewed and
   * any session with a live composer stream are pinned; evicted sessions are
   * simply re-fetched via loadMessages on the next visit.
   */
  function evictStaleSessionCaches(nextViewedSid: string) {
    const active = new Set<string>(activeComposerSidRef.current);
    for (const k of postAbortBySidRef.current.keys()) active.add(k);
    for (const k of relayAbortBySidRef.current.keys()) active.add(k);
    for (const k of streamingAssistantBySidRef.current.keys()) active.add(k);
    const victims = streamShadowBySidRef.current.evict({
      viewedSid: nextViewedSid,
      activeStreamSids: active,
    });
    for (const sid of victims) {
      relayLastEventIdBySidRef.current.delete(sid);
    }
  }

  const generating = useMemo(() => {
    const sid = sessionId.trim();
    if (!sid) return false;
    return activeComposerSidRef.current.has(sid);
  }, [sessionId, composerActivityEpoch]);

  // Poll while the session is working, not merely while this client streams it: a turn
  // recovered from disk, or an autonomous turn woken by a background task, burns context
  // just the same, and stopping here is what left the ring frozen until the turn ended.
  useEffect(() => {
    const sid = sessionId.trim();
    if (!sid || (!generating && !viewedTurnActive)) {
      return;
    }
    void refreshSessionStats(sid);
    const timer = window.setInterval(() => {
      void refreshSessionStats(sid);
    }, 800);
    return () => window.clearInterval(timer);
  }, [sessionId, generating, viewedTurnActive, refreshSessionStats]);

  // A session switch must not carry the previous chat's "still working" flag; the probes
  // below re-raise it within a couple of seconds if the new one is in fact busy.
  useEffect(() => {
    setViewedTurnActive(false);
  }, [sessionId]);

  // Arm audio unlock once so the first desktop chime is not swallowed by the
  // browser autoplay policy.
  useEffect(() => {
    if (isDesktopShell()) {
      armNotificationSoundUnlock();
    }
  }, []);

  // Desktop plan-ready toast + chime. Fires on the generation-finished edge
  // (true -> false) for the viewed session, so merely opening an old session
  // with an existing plan never notifies. Plans present when a turn *starts*
  // are seeded as already-seen so only a freshly produced plan triggers.
  useEffect(() => {
    const sid = sessionId.trim();
    const was = prevGeneratingRef.current;
    const wasSid = prevGenSessionRef.current;
    prevGeneratingRef.current = generating;
    prevGenSessionRef.current = sid;
    if (!isDesktopShell() || !sid || wasSid !== sid) {
      return;
    }
    if (!was && generating) {
      for (const it of itemsRef.current) {
        if (it.type === "plan_document" && it.slug) {
          notifiedPlanKeysRef.current.add(`${sid}:${it.slug}`);
        }
      }
      return;
    }
    if (was && !generating) {
      const timers = [0, 350, 900].map((delay) =>
        window.setTimeout(() => {
          if (viewedSessionIdRef.current.trim() !== sid) return;
          for (const it of itemsRef.current) {
            if (it.type !== "plan_document" || !it.slug || it.discarded) {
              continue;
            }
            const planKey = `${sid}:${it.slug}`;
            if (notifiedPlanKeysRef.current.has(planKey)) continue;
            notifiedPlanKeysRef.current.add(planKey);
            const body = (it.name || it.overview || "").trim();
            const planNotif: DesktopPlanNotification = {
              kind: "plan",
              id: `plan_${planKey}`,
              sessionId: sid,
              slug: it.slug,
              title: t("desktopNotify.planReadyTitle"),
              body,
            };
            setDesktopNotifications((prev) => [...prev, planNotif]);
            playNotificationSound();
            break;
          }
        }, delay),
      );
      return () => timers.forEach((tm) => window.clearTimeout(tm));
    }
    return;
  }, [generating, sessionId]);

  const sidebarActiveId = sessionId.trim() || activeDraftId.trim();

  const sessionsForSidebar = useMemo(
    () => mergeSessionsWithDrafts(sessions, clientDraftSessions),
    [sessions, clientDraftSessions],
  );

  const reasoningDurationMsByContentRef = useRef<Map<string, number>>(
    new Map(),
  );
  const [modelInfos, setModelInfos] = useState<ModelInfo[]>([]);
  /**
   * Bumped whenever the server's configuration moved: a settings save here, or a
   * `config_reloaded` event from a swap made elsewhere (the agent's `config_commit`,
   * a skill install, another tab, an edit on disk that `serve` picked up). Everything
   * derived from the config re-reads on it, so a model added mid-session reaches the
   * picker without a page reload.
   */
  const [configEpoch, setConfigEpoch] = useState(0);
  const [showProviderPicker, setShowProviderPicker] = useState(false);
  const [showTour, setShowTour] = useState(false);
  const [sessionsOpen, setSessionsOpen] = useState(false);
  /**
   * Root folder the server itself was launched with (`--cwd`). Unlike
   * `workspaceCtx`, which follows the *viewed session*, this stays put — so the
   * History scope does not flip when the user opens a session from elsewhere.
   */
  const [hostProjectRoot, setHostProjectRoot] = useState("");
  const [sessionsProjectOnly, setSessionsProjectOnly] = useState(false);
  /** null until first probe of /foxxycode/scheduler/jobs; false when route returns 404 (binary without scheduler). */
  const [schedulerHttpLinked, setSchedulerHttpLinked] = useState<
    boolean | null
  >(null);
  /** null until the optional Mini Apps capability probe completes. */
  const [miniAppsHttpLinked, setMiniAppsHttpLinked] = useState<boolean | null>(
    null,
  );
  const [schedulerOpen, setSchedulerOpen] = useState(false);
  const [miniAppsOpen, setMiniAppsOpen] = useState(false);
  const [miniAppsAppId, setMiniAppsAppId] = useState<string | null>(null);
  const [settingsRoute, setSettingsRoute] = useState(false);
  const [swarmRoute, setSwarmRoute] = useState(false);
  // The Swarm entry only appears when the environment answers as a relay: on a
  // plain agent there is no swarm to show.
  const [isSwarmEnv, setIsSwarmEnv] = useState(false);
  /**
   * True when this page IS the relay, not an agent reached through one.
   *
   * A relay holds no sessions and serves no /foxxycode/* at all, so a chat
   * box and a history drawer there are furniture for a room nobody can
   * enter. What it does have is the swarm, so that is what it shows.
   */
  const [atSwarmRoot, setAtSwarmRoot] = useState(false);
  // Active Settings section id from `#/settings/<section>` (null = default/grid).
  const [settingsSection, setSettingsSection] = useState<string | null>(null);
  const [schedulerEditor, setSchedulerEditor] =
    useState<SchedulerEditorState>(null);
  const [tasksOpen, setTasksOpen] = useState(false);
  const [tasksSelectedId, setTasksSelectedId] = useState<string | null>(null);
  const [backgroundTasks, setBackgroundTasks] = useState<BackgroundTask[]>([]);
  const [backgroundRunning, setBackgroundRunning] = useState(0);
  // Detached subagents blocked on a prompt. Kept as a number so the poll
  // effect below does not restart on every list refresh.
  const backgroundAwaiting = useMemo(
    () => awaitingPermissionCount(backgroundTasks),
    [backgroundTasks],
  );
  const [backgroundOutput, setBackgroundOutput] = useState("");
  const [backgroundListError, setBackgroundListError] = useState<string | null>(
    null,
  );
  const [backgroundListLoading, setBackgroundListLoading] = useState(false);
  /** Ticks once a second so elapsed times advance between polls. */
  const [backgroundNowMs, setBackgroundNowMs] = useState(() => Date.now());
  const [schedulerJobs, setSchedulerJobs] = useState<SchedulerJob[]>([]);
  const [schedulerInfo, setSchedulerInfo] = useState<SchedulerInfo | null>(
    null,
  );
  const [schedulerListError, setSchedulerListError] = useState<string | null>(
    null,
  );
  const [schedulerListLoading, setSchedulerListLoading] = useState(false);
  const [schedulerFilterDraft, setSchedulerFilterDraft] = useState("");
  const [schedulerFilterQ, setSchedulerFilterQ] = useState("");
  const schedulerDockClusterRef = useRef<HTMLDivElement>(null);
  const [schedDockClusterWidthPx, setSchedDockClusterWidthPx] = useState(0);
  const [sessionFilterDraft, setSessionFilterDraft] = useState("");
  const [sessionFilterQ, setSessionFilterQ] = useState("");
  const [sessionsHasMore, setSessionsHasMore] = useState(false);
  const [sessionsLoadingMore, setSessionsLoadingMore] = useState(false);
  const sessionsHasMoreRef = useRef(false);
  const sessionsLoadingMoreRef = useRef(false);
  const [viewportXL, setViewportXL] = useState(false);
  const [railLabelsWide, setRailLabelsWide] = useState(false);
  const [mode, setMode] = useState<string>("agent");
  const [llmModelIds, setLlmModelIds] = useState<string[]>([]);
  const [defaultAgentYamlModel, setDefaultAgentYamlModel] = useState("");
  const [llmModel, setLlmModel] = useState("");
  // Provider account usage for the composer's usage section and banner: read
  // over REST at session open, model change and after each viewed turn, pushed
  // by the events stream in between (chat/useProviderUsage.ts).
  const [providerUsageTurnEpoch, setProviderUsageTurnEpoch] = useState(0);
  const noteUsageTurnEnded = useCallback((sid: string) => {
    if (viewedSessionIdRef.current.trim() === sid.trim()) {
      setProviderUsageTurnEpoch((n) => n + 1);
    }
  }, []);
  const providerUsageState = useProviderUsage({
    sessionId,
    llmModel,
    turnEpoch: providerUsageTurnEpoch,
  });
  const [llmReasoning, setLlmReasoning] = useState("");
  /**
   * Raw mode/model/reasoning stored on the opened session. Held until the
   * backends list (`llmModelIds`) is available so the restore survives whichever
   * of `/v1/models` and `/foxxycode/sessions/.../messages` resolves first on
   * reload.
   */
  const [openSessionSelection, setOpenSessionSelection] = useState<{
    sid: string;
    model: string;
    reasoning: string;
    mode: string;
  } | null>(null);
  /**
   * Session whose stored selection has already been restored. `loadMessages`
   * also runs after every turn and on every reconnect, and each response
   * carries the stored selection, so without this the restore would keep
   * reverting a Mode or Model the user picked a moment earlier.
   */
  const appliedSelectionSidRef = useRef("");
  /** Session whose Mode/Model the user changed by hand; their pick wins. */
  const userTouchedSelectionSidRef = useRef("");
  /**
   * Bumped on every hand-made Mode change. A turn snapshots it so the
   * post-turn mode adoption can tell "the server switched under us" from
   * "the user picked a new mode while the turn was running".
   */
  const modeEditSeqRef = useRef(0);
  /** Current mode, readable from callbacks that outlive their render. */
  const modeRef = useRef(mode);
  modeRef.current = mode;
  const [describePreview, setDescribePreview] = useState<{
    sessionId: string;
    title: string;
  } | null>(null);
  const heroAccentVerb = useMemo(
    () => pickHeroAccentVerb(sessionId, heroHomeGeneration),
    [sessionId, heroHomeGeneration],
  );

  const handleComposerSseQuestion = useCallback(
    (raw: Record<string, unknown>) => {
      const p = parseFoxxyCodeQuestionPayload(raw);
      if (!p) return;
      const key = p.sessionId.trim();
      if (!key) return;
      // A relay rejoin replays the turn's SSE from the start — don't re-show a
      // question the user already answered.
      const rid = p.requestId.trim();
      if (
        loadQuestionPromptRecords(key).some(
          (r) => r.requestId.trim() === rid && r.resolved,
        )
      ) {
        return;
      }
      setQuestionPendingSids((prev) => {
        const next = new Set(prev);
        next.add(key);
        return next;
      });
      upsertQuestionPromptRecord(key, {
        requestId: p.requestId.trim(),
        payload: p,
      });
      applyStreamItemsForSession(key, (prev) => {
        const ridInner = p.requestId;
        const withoutStalePending = prev.filter(
          (x) => !(x.type === "question_prompt" && !x.resolved),
        );
        const withoutDup = withoutStalePending.filter(
          (x) =>
            !(x.type === "question_prompt" && x.payload.requestId === ridInner),
        );
        return [
          ...withoutDup,
          {
            id: `qp_${ridInner}`,
            type: "question_prompt" as const,
            payload: p,
          },
        ];
      });
    },
    [],
  );

  const handleComposerSsePermission = useCallback(
    (raw: Record<string, unknown>) => {
      const p = parseFoxxyCodePermissionPayload(raw);
      if (!p) return;
      const key = p.sessionId.trim();
      if (!key) return;
      const tcid = p.toolCall.toolCallId.trim();
      // A relay rejoin replays the turn's SSE from the start, so this event can
      // arrive again for a prompt the user already answered — don't re-show it.
      if (resolvedPermissionToolCallIds(key).has(tcid)) return;
      setPermissionPendingSids((prev) => {
        const next = new Set(prev);
        next.add(key);
        return next;
      });
      applyStreamItemsForSession(key, (prev) => {
        const withoutStalePending = prev.filter(
          (x) => !(x.type === "permission_prompt" && !x.resolved),
        );
        const withoutDup = withoutStalePending.filter(
          (x) =>
            !(
              x.type === "permission_prompt" &&
              x.payload.toolCall.toolCallId === tcid
            ),
        );
        const row = {
          id: stablePermissionPromptItemId(tcid),
          type: "permission_prompt" as const,
          payload: p,
        };
        upsertPermissionPromptRecord(key, {
          toolCallId: tcid,
          payload: p,
        });
        // Below the tool_call AND the composing assistant bubble, so the Allow
        // card never renders above the text that introduced the tool.
        const insertAt = permissionPromptInsertIndex(withoutDup, tcid);
        const result = [...withoutDup];
        result.splice(insertAt, 0, row);
        return result;
      });
      if (isDesktopShell()) {
        const notif: DesktopPermissionNotification = {
          kind: "permission",
          id: `perm_${tcid}`,
          sessionId: key,
          toolCallId: tcid,
          itemId: stablePermissionPromptItemId(tcid),
          // Same wording as the inline card so the toast and the transcript agree.
          title: buildPermissionToolPreview(p).title,
          detail: permissionPromptDetail(p),
          options: p.options.map((o) => ({
            optionId: o.optionId,
            name: o.name,
          })),
        };
        setDesktopNotifications((prev) => [
          ...prev.filter(
            (n) => !(n.kind === "permission" && n.toolCallId === tcid),
          ),
          notif,
        ]);
        playNotificationSound();
      }
    },
    [],
  );

  /**
   * `event: plan` with design meta means plan_write just published a plan. Pull the
   * document and put the card in the live transcript right away, instead of waiting
   * for the post-turn rebuild. pinPlanDocumentsToTurnEnd keeps it below the text that
   * keeps streaming in after it.
   */
  const handleComposerSseDesignPlan = useCallback(
    (sid: string, slug: string) => {
      const key = sid.trim();
      const s = slug.trim();
      if (!key || !s) return;
      void (async () => {
        const res = await fetchJSON<{
          slug?: string;
          name?: string;
          overview?: string;
          content?: string;
          body?: string;
          updatedAt?: string;
        }>(
          `/foxxycode/sessions/${encodeURIComponent(key)}/plans/${encodeURIComponent(s)}`,
          { headers: { [HDR]: key } },
        );
        if (!res.ok || !res.data) return;
        const doc = res.data;
        applyStreamItemsForSession(key, (prev) => {
          const at = prev.findIndex(
            (x) => x.type === "plan_document" && x.slug === s,
          );
          const existing = at >= 0 ? prev[at] : undefined;
          const row: TranscriptItem = {
            id:
              existing?.type === "plan_document" ? existing.id : newId("pd"),
            type: "plan_document",
            slug: s,
            name: String(doc.name ?? ""),
            overview: String(doc.overview ?? ""),
            content: String(doc.content ?? ""),
            body: String(doc.body ?? ""),
            // A rewrite of the same slug updates the card in place; never stack duplicates.
            expanded:
              existing?.type === "plan_document" ? existing.expanded : false,
            ...(existing?.type === "plan_document" && existing.path
              ? { path: existing.path }
              : {}),
            ...(doc.updatedAt ? { updatedAtUtc: String(doc.updatedAt) } : {}),
          };
          if (at >= 0) {
            const next = [...prev];
            next[at] = row;
            return next;
          }
          return [...prev, row];
        });
      })();
    },
    [],
  );

  const resolveQuestionPrompt = useCallback(
    (sessionId: string, itemId: string, resolved: QuestionResolvedState) => {
      const key = sessionId.trim();
      if (!key) return;
      setQuestionPendingSids((prev) => {
        const next = new Set(prev);
        next.delete(key);
        return next;
      });
      applyStreamItemsForSession(key, (prev) => {
        const next = prev.map((x) =>
          x.id === itemId && x.type === "question_prompt"
            ? { ...x, resolved }
            : x,
        );
        const hit = next.find(
          (x) => x.id === itemId && x.type === "question_prompt",
        );
        if (hit?.type === "question_prompt") {
          upsertQuestionPromptRecord(key, {
            requestId: hit.payload.requestId.trim(),
            payload: hit.payload,
            ...(hit.resolved !== undefined ? { resolved: hit.resolved } : {}),
          });
        }
        return next;
      });
      // Same safety net as permissions: re-attach if the stream died while pending.
      resetLiveRecoveryState(key);
      scheduleLiveStreamReconnectRef.current(key, 1100);
    },
    [],
  );

  const resolvePermissionPrompt = useCallback(
    (sessionId: string, itemId: string, resolved: PermissionResolvedState) => {
      const key = sessionId.trim();
      if (!key) return;
      setPermissionPendingSids((prev) => {
        const next = new Set(prev);
        next.delete(key);
        return next;
      });
      // Co-dismiss the matching desktop toast (whether resolved here or inline).
      setDesktopNotifications((prev) =>
        prev.filter((n) => !(n.kind === "permission" && n.itemId === itemId)),
      );
      applyStreamItemsForSession(key, (prev) => {
        const hit = prev.find(
          (x) => x.type === "permission_prompt" && x.id === itemId,
        );
        if (hit?.type === "permission_prompt") {
          // Keep the record marked as resolved so restorePermissionPromptsForPendingTools
          // won't re-synthesize a prompt for the same tool call on subsequent loadMessages.
          upsertPermissionPromptRecord(key, {
            toolCallId: hit.payload.toolCall.toolCallId.trim(),
            payload: hit.payload,
            resolved,
          });
        }
        return prev.filter(
          (x) => !(x.type === "permission_prompt" && x.id === itemId),
        );
      });
      for (const delayMs of [0, 250, 900]) {
        window.setTimeout(() => {
          void loadMessages(key, {
            preserveOnError: true,
            skipSetItems: viewedSessionIdRef.current.trim() !== key,
          });
          void loadSessionsList(true);
        }, delayMs);
      }
      // Safety net: if the live stream died while this prompt was pending, the
      // turn continues server-side after the answer — re-attach so the
      // continuation renders live (no-op when a stream is already attached).
      resetLiveRecoveryState(key);
      scheduleLiveStreamReconnectRef.current(key, 1100);
    },
    [],
  );

  /** Set while the viewed session is a subagent's transcript (read-only, no composer). */
  const [subagentTranscript, setSubagentTranscript] =
    useState<SubagentTranscriptMeta | null>(null);

  const currentTitle = useMemo(() => {
    if (!sessionId) {
      return t("chat.newChat");
    }
    if (describePreview?.sessionId === sessionId) {
      const hint = describePreview.title.trim();
      if (hint) {
        return hint;
      }
    }
    const row = sessions.find((s) => s.id === sessionId);
    const title = (row?.title || "").trim();
    if (title) {
      return title;
    }
    // A child session has no History row to name it, so name it by its role.
    if (subagentTranscript) {
      const name = subagentTranscript.name.trim();
      return name
        ? t("chat.subagentTitle", { name })
        : t("chat.subagentTitleUnnamed");
    }
    return t("chat.newChat");
  }, [sessionId, sessions, describePreview, locale, subagentTranscript]);

  const currentSessionCwd = useMemo(() => {
    const sid = sessionId.trim();
    if (!sid) {
      return "";
    }
    return (sessions.find((s) => s.id === sid)?.cwd || "").trim();
  }, [sessionId, sessions]);

  async function saveSessionTitle(id: string, title: string) {
    const t = title.trim();
    if (!t) {
      return;
    }
    await fetch(`/foxxycode/sessions/${encodeURIComponent(id)}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ title: t }),
    });
    setSessions((prev) =>
      prev.map((s) => (s.id === id ? { ...s, title: t } : s)),
    );
  }

  /**
   * Export the current session transcript as a document. The server renders the
   * chosen format and returns a binary attachment; we stream it to a Blob and
   * trigger a browser download, recovering the filename from the
   * Content-Disposition header when the server supplies one.
   */
  async function exportSession(format: ExportFormat) {
    if (!sessionId || exportBusy) {
      return;
    }
    const sid = sessionId;
    // A failed export must say so: the spinner stopping on its own reads as a
    // finished download that never arrived. Network errors are caught here too,
    // otherwise the rejected promise escapes the `void exportSession(...)` call
    // site as an unhandled rejection.
    const notice = (level: "error" | "info", message: string) => {
      applyStreamItemsForSession(sid, (prev) => [
        ...prev,
        {
          id: newId("s"),
          type: "system_notice" as const,
          level,
          message,
          createdAtUtc: new Date().toISOString(),
        },
      ]);
    };
    const fail = () => notice("error", t("chat.exportFailed"));
    setExportBusy(true);
    try {
      if (isEditorEmbed()) {
        // An editor webview cannot save a blob: IntelliJ's JCEF drops downloads
        // no CefDownloadHandler claims, and the VS Code panel hosts this SPA in
        // a cross-origin iframe with no download permission. Have the server
        // write the document out instead; a connected plugin reveals it.
        const res = await fetch(
          `/foxxycode/sessions/${encodeURIComponent(sid)}/export/file?format=${format}`,
          { method: "POST", headers: { [HDR]: sid } },
        );
        if (!res.ok) {
          // 409: every candidate name was held by something the server could not
          // replace — on Windows that is the exported document still open.
          notice(
            "error",
            res.status === 409
              ? t("chat.exportNameTaken")
              : t("chat.exportFailed"),
          );
          return;
        }
        const saved = (await res.json()) as { path?: string };
        const path = (saved.path ?? "").trim();
        if (path === "") {
          fail();
          return;
        }
        notice("info", t("chat.exportSaved", { path }));
        return;
      }
      const res = await fetch(
        `/foxxycode/sessions/${encodeURIComponent(sid)}/export?format=${format}`,
        { headers: { [HDR]: sid } },
      );
      if (!res.ok) {
        fail();
        return;
      }
      const blob = await res.blob();
      const filename =
        filenameFromDisposition(res.headers.get("Content-Disposition")) ??
        `session.${format}`;
      downloadBlob(filename, blob);
    } catch {
      fail();
    } finally {
      setExportBusy(false);
    }
  }

  const headers = useMemo(
    () => (sessionId ? { [HDR]: sessionId } : {}),
    [sessionId],
  );

  const refreshWorkspaceContext = useCallback(async (sid: string) => {
    try {
      const res = await fetch("/foxxycode/workspace/context", {
        headers: sid ? { [HDR]: sid } : {},
      });
      if (res.ok) {
        setWorkspaceCtx((await res.json()) as WorkspaceContext);
      }
    } catch {
      // ignore: chips keep the previous context
    }
  }, []);

  // Load the workspace context whenever the viewed session changes; a fresh
  // home/draft view also drops stale pre-session workspace choices.
  useEffect(() => {
    pendingWorkspaceRef.current = null;
    void refreshWorkspaceContext(sessionId);
  }, [sessionId, refreshWorkspaceContext]);

  // Host project root, once. Without a session header the endpoint reports the
  // server's own cwd, which is the project an editor plugin launched it for.
  useEffect(() => {
    void (async () => {
      try {
        const res = await fetch("/foxxycode/workspace/context");
        if (!res.ok) return;
        const ctx = (await res.json()) as { path?: string };
        const root = (ctx.path || "").trim();
        if (!root) return;
        setHostProjectRoot(root);
        setSessionsProjectOnly(readProjectOnlyPref(root, isEditorEmbed()));
      } catch {
        // No scope available: History stays unfiltered.
      }
    })();
  }, []);

  const changeSessionsProjectOnly = useCallback(
    (next: boolean) => {
      setSessionsProjectOnly(next);
      writeProjectOnlyPref(hostProjectRoot, next);
    },
    [hostProjectRoot],
  );

  async function switchWorkspace(payload: {
    path?: string;
    branch?: string;
    worktree?: boolean;
    vcs?: "git" | "svn";
  }) {
    const sid = sessionId.trim();
    if (!sid) {
      // No session yet: remember the choice and preview the target context.
      pendingWorkspaceRef.current = {
        ...(pendingWorkspaceRef.current || {}),
        ...payload,
      };
      if (payload.path) {
        try {
          const res = await fetch(
            "/foxxycode/workspace/context?path=" + encodeURIComponent(payload.path),
          );
          if (res.ok) {
            setWorkspaceCtx((await res.json()) as WorkspaceContext);
          }
        } catch {
          // ignore
        }
      } else if (payload.branch) {
        const nextBranch = payload.branch;
        setWorkspaceCtx((prev) => {
          if (!prev) {
            return prev;
          }
          if (payload.vcs === "svn") {
            return {
              ...prev,
              svn: { ...(prev.svn ?? { available: true }), branch: nextBranch },
            };
          }
          return {
            ...prev,
            branch: nextBranch,
            is_worktree: Boolean(payload.worktree),
          };
        });
      }
      return;
    }
    try {
      const res = await fetch(
        `/foxxycode/sessions/${encodeURIComponent(sid)}/workspace`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json", [HDR]: sid },
          body: JSON.stringify(payload),
        },
      );
      if (res.ok) {
        setWorkspaceCtx((await res.json()) as WorkspaceContext);
      } else {
        await refreshWorkspaceContext(sid);
      }
    } catch {
      // network error: keep the current chips
    }
  }

  // Applies pre-session workspace choices to the freshly created session id
  // right before the first send.
  async function applyPendingWorkspace(sid: string) {
    const pending = pendingWorkspaceRef.current;
    pendingWorkspaceRef.current = null;
    if (!pending || (!pending.path && !pending.branch)) {
      return;
    }
    const base = { "Content-Type": "application/json", [HDR]: sid };
    try {
      if (pending.path) {
        await fetch(`/foxxycode/sessions/${encodeURIComponent(sid)}/workspace`, {
            method: "POST",
            headers: base,
            body: JSON.stringify({ path: pending.path }),
        });
      }
      if (pending.branch) {
        await fetch(`/foxxycode/sessions/${encodeURIComponent(sid)}/workspace`, {
            method: "POST",
            headers: base,
            body: JSON.stringify({
              branch: pending.branch,
              worktree: Boolean(pending.worktree),
              vcs: pending.vcs ?? "git",
            }),
        });
      }
    } catch {
      // ignore: the session still starts in the default workspace
    }
  }

  const refreshBackgroundTasks = useCallback(
    async (opts?: { silent?: boolean }) => {
      const sid = sessionId.trim();
      if (!sid) {
        setBackgroundTasks([]);
        setBackgroundRunning(0);
        return;
      }
      const silent = !!opts?.silent;
      if (!silent) {
        setBackgroundListLoading(true);
        setBackgroundListError(null);
      }
      const res = await listBackgroundTasks(sid);
      if (!silent) {
        setBackgroundListLoading(false);
      }
      if (!res.ok) {
        if (!silent) {
          setBackgroundListError(res.message);
          setBackgroundTasks([]);
          setBackgroundRunning(0);
        }
        return;
      }
      setBackgroundListError(null);
      setBackgroundTasks(res.data.data || []);
      setBackgroundRunning(res.data.running || 0);
    },
    [sessionId],
  );

  const refreshBackgroundTaskOutput = useCallback(
    async (taskId: string) => {
      const sid = sessionId.trim();
      if (!sid || !taskId) {
        setBackgroundOutput("");
        return;
      }
      const res = await getBackgroundTask(sid, taskId);
      setBackgroundOutput(res.ok ? res.data.output || "" : "");
    },
    [sessionId],
  );

  const stopBackgroundTaskById = useCallback(
    async (taskId: string) => {
      const sid = sessionId.trim();
      if (!sid || !taskId) {
        return;
      }
      const res = await stopBackgroundTask(sid, taskId);
      if (res.ok) {
        setBackgroundOutput(res.data.output || "");
      }
      void refreshBackgroundTasks({ silent: true });
    },
    [sessionId, refreshBackgroundTasks],
  );

  const clearFinishedTasks = useCallback(async () => {
    const sid = sessionId.trim();
    if (!sid) {
      return;
    }
    await clearFinishedBackgroundTasks(sid);
    setTasksSelectedId(null);
    void refreshBackgroundTasks({ silent: true });
  }, [sessionId, refreshBackgroundTasks]);

  const refreshSchedulerJobs = useCallback(
    async (opts?: { silent?: boolean }) => {
      const silent = !!opts?.silent;
      if (!silent) {
        setSchedulerListLoading(true);
        setSchedulerListError(null);
      }
      const res = await schedulerListJobs(false);
      if (!silent) {
        setSchedulerListLoading(false);
      }
      if (!res.ok) {
        let msg = res.message;
        if (res.status === 404) {
          setSchedulerHttpLinked(false);
          setSchedulerOpen(false);
          setSchedulerEditor(null);
          msg = t("scheduler.apiNotAvailable");
          const sid = sessionId.trim();
          if (sid) {
            setSessionHashInLocation(sid);
          } else if (window.location.hash) {
            history.replaceState(
              null,
              "",
              `${window.location.pathname}${window.location.search}`,
            );
          }
          setSchedulerListError(msg);
          setSchedulerJobs([]);
          setSchedulerInfo(null);
          return;
        }
        if (res.status === 503) {
          msg = t("scheduler.disabled");
          if (!silent) {
            setSchedulerListError(msg);
            setSchedulerJobs([]);
            setSchedulerInfo(null);
          }
          return;
        }
        if (!silent) {
          setSchedulerListError(msg);
          setSchedulerJobs([]);
          setSchedulerInfo(null);
        }
        return;
      }
      setSchedulerInfo(res.data.scheduler);
      setSchedulerJobs(res.data.jobs || []);
    },
    [sessionId],
  );

  const applyLocationHash = useCallback(() => {
    const p = parseAppHash();
    if (p.branch !== "miniapps") {
      setMiniAppsOpen(false);
      setMiniAppsAppId(null);
    }
    if (p.branch === "session") {
      setSettingsRoute(false);
      setActiveDraftId("");
      viewedSessionIdRef.current = p.sessionId.trim();
      setSessionId(p.sessionId);
      setSessionLoading(true);
      void markFoxxyCodeSessionActivityRead(p.sessionId);
      setSchedulerOpen(false);
      setSchedulerEditor(null);
      setTasksOpen(p.tasksOpen);
      setTasksSelectedId(p.taskId);
      setSessionsOpen(!!p.historyOpen);
      return;
    }
    if (p.branch === "draft") {
      setSettingsRoute(false);
      setSchedulerOpen(false);
      setSchedulerEditor(null);
      setTasksOpen(false);
      setTasksSelectedId(null);
      setSessionId("");
      viewedSessionIdRef.current = "";
      setActiveDraftId(p.draftId.trim());
      const row = readClientDraftSessions().find(
        (r) => r.localId === p.draftId.trim(),
      );
      setDraft(row?.draftText || "");
      setItems([]);
      setSessionsOpen(!!p.historyOpen);
      return;
    }
    if (p.branch === "history") {
      setSettingsRoute(false);
      setSessionsOpen(true);
      setSchedulerOpen(false);
      setSchedulerEditor(null);
      setTasksOpen(false);
      setTasksSelectedId(null);
      return;
    }
    if (p.branch === "miniapps") {
      if (isEditorEmbed() || miniAppsHttpLinked === false) {
        const sid = viewedSessionIdRef.current.trim();
        if (sid) setSessionHashInLocation(sid);
        else if (window.location.hash) {
          history.replaceState(
            null,
            "",
            `${window.location.pathname}${window.location.search}`,
          );
        }
        setMiniAppsOpen(false);
        setMiniAppsAppId(null);
        return;
      }
      if (miniAppsHttpLinked === null) return;
      setSettingsRoute(false);
      setSchedulerOpen(false);
      setSchedulerEditor(null);
      setTasksOpen(false);
      setTasksSelectedId(null);
      setSessionsOpen(false);
      setMiniAppsOpen(true);
      setMiniAppsAppId(p.appId);
      return;
    }
    if (p.branch === "swarm") {
      setSwarmRoute(true);
      setSettingsRoute(false);
      setSchedulerOpen(false);
      setSchedulerEditor(null);
      setTasksOpen(false);
      setTasksSelectedId(null);
      setSessionsOpen(false);
      return;
    }
    setSwarmRoute(false);
    if (p.branch === "settings") {
      setSettingsRoute(true);
      setSettingsSection(p.section);
      setSchedulerOpen(false);
      setSchedulerEditor(null);
      setTasksOpen(false);
      setTasksSelectedId(null);
      setSessionsOpen(false);
      return;
    }
    if (p.branch === "scheduler") {
      setSettingsRoute(false);
      if (schedulerHttpLinked === false) {
        setSchedulerOpen(false);
        setSchedulerEditor(null);
        const sid = viewedSessionIdRef.current.trim();
        if (sid) {
          setSessionHashInLocation(sid);
        } else if (window.location.hash) {
          history.replaceState(
            null,
            "",
            `${window.location.pathname}${window.location.search}`,
          );
        }
        return;
      }
      if (schedulerHttpLinked === null) {
        return;
      }
      setSchedulerOpen(true);
      setSessionsOpen(false);
      setTasksOpen(false);
      setTasksSelectedId(null);
      setSchedulerEditor(schedulerEditorFromParsedHash(p));
      return;
    }
    viewedSessionIdRef.current = "";
    setSessionId("");
    setActiveDraftId("");
    setSettingsRoute(false);
    setSchedulerOpen(false);
    setSchedulerEditor(null);
    setTasksOpen(false);
    setTasksSelectedId(null);
    setSessionsOpen(!!p.historyOpen);
  }, [miniAppsHttpLinked, schedulerHttpLinked]);

  const openSessionFromRoute = useCallback(
    (id: string, opts?: { historySidebar?: boolean }) => {
      setActiveDraftId("");
      setSchedulerOpen(false);
      setSchedulerEditor(null);
      setTasksOpen(false);
      setTasksSelectedId(null);
      viewedSessionIdRef.current = id.trim();
      setSessionHashInLocation(id, opts);
      setSessionId(id);
      void markFoxxyCodeSessionActivityRead(id);
    },
    [],
  );

  const clearSessionRoute = useCallback(() => {
    setSchedulerOpen(false);
    setSchedulerEditor(null);
    setTasksOpen(false);
    setTasksSelectedId(null);
    viewedSessionIdRef.current = "";
    setSessionHashInLocation("");
    setSessionId("");
    setActiveDraftId("");
  }, []);

  const closeSchedulerDrawer = useCallback(() => {
    setSchedulerOpen(false);
    setSchedulerEditor(null);
    setTasksOpen(false);
    setTasksSelectedId(null);
    if (sessionsOpen) {
      setHistoryHash();
      return;
    }
    const sid = sessionId.trim();
    if (sid) {
      setSessionHashInLocation(sid);
    } else if (window.location.hash) {
      history.replaceState(
        null,
        "",
        `${window.location.pathname}${window.location.search}`,
      );
    }
  }, [sessionId, sessionsOpen]);

  const closeAllShellDrawers = useCallback(() => {
    setSessionsOpen(false);
    setSchedulerOpen(false);
    setSchedulerEditor(null);
    setTasksOpen(false);
    setTasksSelectedId(null);
    if (parseAppHash().branch === "settings") {
      const sid = sessionId.trim();
      if (sid) {
        setSessionHashInLocation(sid);
      } else {
        clearSessionRoute();
      }
      return;
    }
    const sid = sessionId.trim();
    if (sid) {
      setSessionHashInLocation(sid);
    } else if (window.location.hash) {
      history.replaceState(
        null,
        "",
        `${window.location.pathname}${window.location.search}`,
      );
    }
  }, [sessionId, clearSessionRoute]);

  const prevSessionsOpenRef = useRef(false);
  useEffect(() => {
    if (prevSessionsOpenRef.current && !sessionsOpen) {
      requestAnimationFrame(() => {
        document.getElementById("composer")?.focus();
      });
    }
    prevSessionsOpenRef.current = sessionsOpen;
  }, [sessionsOpen]);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const r = await fetch("/foxxycode/scheduler/jobs");
        if (cancelled) {
          return;
        }
        setSchedulerHttpLinked(r.status !== 404);
      } catch {
        if (!cancelled) {
          setSchedulerHttpLinked(true);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  // Mini Apps is an optional HTTP surface. Prefer the capability document, but
  // retain the catalog probe as a compatibility fallback for older tagged
  // servers that predate the capability field.
  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const capability = await fetch("/foxxycode/capabilities");
        if (capability.ok) {
          const body = (await capability.json()) as Record<string, unknown>;
          const nested = body.capabilities;
          const source =
            nested && typeof nested === "object"
              ? (nested as Record<string, unknown>)
              : body;
          const value = source.miniapps ?? source.mini_apps;
          if (typeof value === "boolean") {
            if (!cancelled) setMiniAppsHttpLinked(value);
            return;
          }
        }
      } catch {
        // The catalog probe below remains authoritative when capabilities is absent.
      }
      try {
        const catalog = await fetch("/foxxycode/miniapps");
        if (!cancelled) setMiniAppsHttpLinked(catalog.status !== 404);
      } catch {
        if (!cancelled) setMiniAppsHttpLinked(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    applyLocationHash();
  }, [applyLocationHash]);

  useEffect(() => {
    const onHash = () => applyLocationHash();
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, [applyLocationHash]);

  // Editor plugins open their panel on the bare route, so continue where the
  // user left off in this project instead of showing the hero screen. Runs
  // once; any explicit deep link in the hash wins.
  useEffect(() => {
    if (
      !shouldRestoreLastSession({
        embed: isEditorEmbed(),
        branch: parseAppHash().branch,
      })
    ) {
      lastSessionRestoreDoneRef.current = true;
      return;
    }
    const startHash = window.location.hash;
    void (async () => {
      const sid = await fetchLastProjectSession();
      lastSessionRestoreDoneRef.current = true;
      // The probe is a loopback call, but the user may still have navigated
      // (History pick, New chat) while it was in flight - never override that.
      if (
        !sid ||
        window.location.hash !== startHash ||
        viewedSessionIdRef.current
      ) {
        return;
      }
      openSessionFromRoute(sid);
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Remember what to reopen next launch. Gated on the probe above: the empty
  // sessionId of a fresh mount would otherwise clear the record before the
  // restore had a chance to read it. Once the user has a session open, going
  // back to a new chat records an empty id so that sticks across restarts too.
  useEffect(() => {
    if (!isEditorEmbed() || !lastSessionRestoreDoneRef.current) {
      return;
    }
    const sid = sessionId.trim();
    // A deep-linked id arrives one render after mount, so the opening empty
    // value is startup noise, not the user asking for a new chat.
    if (!sid && lastRecordedSessionRef.current === null) {
      return;
    }
    if (lastRecordedSessionRef.current === sid) {
      return;
    }
    lastRecordedSessionRef.current = sid;
    recordLastProjectSession(sid);
  }, [sessionId]);

  useEffect(() => {
    void (async () => {
      const res = await fetchJSON<{ items?: Array<{ name: string }> }>(
        "/foxxycode/slash-commands?page=1&page_size=200",
      );
      if (res.ok && res.data?.items) {
        setKnownSkillNames(new Set(res.data.items.map((i) => i.name)));
      }
    })();
    // Slash commands are derived from skills.dirs, so a config swap moves them too.
  }, [configEpoch]);

  // Background tasks outlive the SSE stream of the turn that started them, so
  // the drawer and the nav badge are kept honest by polling rather than by the
  // composer stream. The cadence drops to a slow heartbeat when nothing runs.
  useEffect(() => {
    if (!sessionId.trim()) {
      setBackgroundTasks([]);
      setBackgroundRunning(0);
      setBackgroundOutput("");
      return;
    }
    void refreshBackgroundTasks({ silent: !tasksOpen });
  }, [sessionId, tasksOpen, refreshBackgroundTasks]);

  useEffect(() => {
    if (!sessionId.trim()) {
      return;
    }
    const id = window.setInterval(
      () => {
        void refreshBackgroundTasks({ silent: true });
        if (tasksOpen && tasksSelectedId) {
          void refreshBackgroundTaskOutput(tasksSelectedId);
        }
      },
      // A task waiting for a permission answer keeps the fast cadence even if
      // the server has stopped counting it as running: the answer has to reach
      // the drawer promptly, and the prompt has to leave it once answered.
      tasksPollIntervalMs(backgroundRunning + backgroundAwaiting),
    );
    return () => window.clearInterval(id);
  }, [
    sessionId,
    tasksOpen,
    tasksSelectedId,
    backgroundRunning,
    backgroundAwaiting,
    refreshBackgroundTasks,
    refreshBackgroundTaskOutput,
  ]);

  useEffect(() => {
    if (!tasksOpen || !tasksSelectedId) {
      setBackgroundOutput("");
      return;
    }
    void refreshBackgroundTaskOutput(tasksSelectedId);
  }, [tasksOpen, tasksSelectedId, refreshBackgroundTaskOutput]);

  // Elapsed labels must advance between polls, so the clock ticks on its own
  // while something is actually running.
  useEffect(() => {
    if (backgroundRunning <= 0) {
      return;
    }
    const id = window.setInterval(() => setBackgroundNowMs(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, [backgroundRunning]);

  useEffect(() => {
    if (!schedulerOpen || schedulerHttpLinked === false) {
      return;
    }
    void refreshSchedulerJobs();
  }, [schedulerOpen, schedulerHttpLinked, refreshSchedulerJobs]);

  useEffect(() => {
    if (!schedulerOpen || schedulerHttpLinked !== true) {
      return;
    }
    const id = window.setInterval(() => {
      void refreshSchedulerJobs({ silent: true });
    }, SCHEDULER_JOBS_POLL_MS);
    return () => window.clearInterval(id);
  }, [schedulerOpen, schedulerHttpLinked, refreshSchedulerJobs]);

  useEffect(() => {
    void (async () => {
      const res = await fetchJSON<{
        default_agent_model?: string;
        data?: Array<{
          id?: string;
          owned_by?: string;
          max_context_tokens?: number;
          multimodal?: boolean;
          reasoning_levels?: string[];
          reasoning_default?: string;
        }>;
      }>("/v1/models");
      if (!res.ok || !res.data?.data) {
        return;
      }
      const raw = res.data.data
        .map((d) => ({
          id: (d.id || "").trim(),
          ownedBy: (d.owned_by || "").trim(),
          ...(d.max_context_tokens !== undefined
            ? { maxContextTokens: d.max_context_tokens }
            : {}),
          multimodal: !!d.multimodal,
          reasoningLevels: Array.isArray(d.reasoning_levels)
            ? d.reasoning_levels.map((s) => `${s}`.trim()).filter(Boolean)
            : [],
          reasoningDefault: (d.reasoning_default || "").trim(),
        }))
        .filter((d) => d.id);
      const rows: ModelInfo[] = raw.map((d) => {
        const m: ModelInfo = {
          id: d.id,
          ownedBy: d.ownedBy,
          multimodal: d.multimodal,
          reasoningLevels: d.reasoningLevels,
          reasoningDefault: d.reasoningDefault,
        };
        if (d.maxContextTokens !== undefined) {
          m.maxContextTokens = d.maxContextTokens;
        }
        return m;
      });
      setModelInfos(rows);
      const backends = raw
        .filter((r) => r.ownedBy !== "foxxycode")
        .map((r) => r.id);
      setLlmModelIds(backends);
      const defaultYaml = (res.data.default_agent_model || "").trim();
      setDefaultAgentYamlModel(defaultYaml);
      if (!viewedSessionIdRef.current.trim()) {
        setLlmModel(
          pickDefaultLlmModelForNewChat({
            backends,
            cookie: readLlmModelCookie(),
            defaultAgentModel: defaultYaml,
          }),
        );
      }
    })();
    // configEpoch bumps after every config swap, so a model added to models[] - and the
    // multimodal flag on one already there - reaches the picker without a page reload.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [configEpoch]);

  // Onboarding is decided once per page load. It used to re-run on configEpoch,
  // so every Settings save re-fetched the status and reopened a picker the user
  // had already dismissed. Re-entry is the "Restart onboarding" button
  // (restartOnboarding below), not a save.
  useEffect(() => {
    void (async () => {
      const status = await fetchOnboardingStatus();
      setShowProviderPicker(shouldShowOnboarding(status));
    })();
  }, []);

  // Apply the opened session's saved mode/model/reasoning once the backends
  // list is known. Runs whenever either input lands, so the restore is
  // independent of whether /v1/models or the session messages resolve first
  // after a reload — but only ONCE per session open, because the same snapshot
  // rides every later transcript reconcile and would otherwise revert a pick
  // the user just made.
  useEffect(() => {
    if (!openSessionSelection || llmModelIds.length === 0) {
      return;
    }
    if (
      !shouldApplySessionSelection({
        stashSid: openSessionSelection.sid,
        viewedSid: viewedSessionIdRef.current,
        appliedSid: appliedSelectionSidRef.current,
        userTouchedSid: userTouchedSelectionSidRef.current,
      })
    ) {
      return;
    }
    appliedSelectionSidRef.current = openSessionSelection.sid;
    setLlmModel(
      pickLlmModelForOpenSession({
        backends: llmModelIds,
        sessionModel: openSessionSelection.model,
        cookie: readLlmModelCookie(),
        defaultAgentModel: defaultAgentYamlModel,
      }),
    );
    setLlmReasoning(openSessionSelection.reasoning);
    // The session profile is stored server-side, so a reopened chat comes back
    // in the mode it was left in instead of dropping to agent.
    const storedMode = openSessionSelection.mode.trim();
    if ((PROFILE_MODES as readonly string[]).includes(storedMode)) {
      setMode(storedMode);
    }
  }, [openSessionSelection, llmModelIds, defaultAgentYamlModel]);

  useEffect(() => {
    setDescribePreview((p) => (p && p.sessionId !== sessionId ? null : p));
  }, [sessionId]);

  useEffect(() => {
    const mq = window.matchMedia("(min-width: 1920px)");
    const apply = () => setViewportXL(mq.matches);
    apply();
    mq.addEventListener("change", apply);
    return () => mq.removeEventListener("change", apply);
  }, []);

  useLayoutEffect(() => {
    if (!schedulerOpen || schedulerHttpLinked !== true) {
      setSchedDockClusterWidthPx(0);
      return;
    }
    const el = schedulerDockClusterRef.current;
    if (!el) {
      setSchedDockClusterWidthPx(0);
      return;
    }
    const ro = new ResizeObserver(() => {
      setSchedDockClusterWidthPx(Math.round(el.getBoundingClientRect().width));
    });
    ro.observe(el);
    setSchedDockClusterWidthPx(Math.round(el.getBoundingClientRect().width));
    return () => ro.disconnect();
  }, [schedulerOpen, schedulerHttpLinked, schedulerEditor]);

  useEffect(() => {
    if (!viewportXL) {
      return;
    }
    const c = readNavRailCookie();
    setRailLabelsWide(c === "wide");
  }, [viewportXL]);

  useEffect(() => {
    const t = window.setTimeout(
      () => setSessionFilterQ(sessionFilterDraft.trim()),
      300,
    );
    return () => window.clearTimeout(t);
  }, [sessionFilterDraft]);

  useEffect(() => {
    const t = window.setTimeout(
      () => setSchedulerFilterQ(schedulerFilterDraft.trim()),
      200,
    );
    return () => window.clearTimeout(t);
  }, [schedulerFilterDraft]);

  useEffect(() => {
    sessionsCursorRef.current = sessionsCursor;
  }, [sessionsCursor]);

  useEffect(() => {
    sessionsHasMoreRef.current = sessionsHasMore;
  }, [sessionsHasMore]);

  useEffect(() => {
    sessionsLoadingMoreRef.current = sessionsLoadingMore;
  }, [sessionsLoadingMore]);

  useEffect(() => {
    if (!sessionsOpen && !schedulerOpen) {
      return;
    }
    const onKey = (ev: KeyboardEvent) => {
      if (ev.key !== "Escape") {
        return;
      }
      if (schedulerEditor) {
        setSchedulerEditor(null);
        setSchedulerListHash();
        return;
      }
      if (schedulerOpen) {
        closeSchedulerDrawer();
        return;
      }
      if (sessionsOpen) {
        setSessionsOpen(false);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [sessionsOpen, schedulerOpen, schedulerEditor, closeSchedulerDrawer]);

  const loadSessionsList = useCallback(
    async (reset: boolean): Promise<SessionRow[] | null> => {
      if (reset) {
        sessionsCursorRef.current = null;
        setSessionsCursor(null);
      } else if (
        !sessionsHasMoreRef.current ||
        sessionsLoadingMoreRef.current
      ) {
        return null;
      }
      if (!reset) {
        sessionsLoadingMoreRef.current = true;
        setSessionsLoadingMore(true);
      }
      const ps = new URLSearchParams();
      ps.set("limit", "30");
      if (!reset) {
        const cur = sessionsCursorRef.current;
        if (cur) {
          ps.set("cursor", cur);
        }
      }
      if (sessionFilterQ) {
        ps.set("q", sessionFilterQ);
      }
      const scopeCwd = sessionsProjectCwdParam({
        projectOnly: sessionsProjectOnly,
        projectRoot: hostProjectRoot,
      });
      if (scopeCwd) {
        ps.set("cwd", scopeCwd);
      }
      ps.set("include_activity", "true");
      const res = await fetchJSON<{
        sessions: SessionRow[];
        nextCursor?: string | null;
        hasMore?: boolean;
      }>(`/foxxycode/sessions?${ps.toString()}`, {
        headers,
      });
      if (!reset) {
        sessionsLoadingMoreRef.current = false;
        setSessionsLoadingMore(false);
      }
      if (!res.ok || !res.data) {
        // status 0 is fetchJSON's "no answer from the server" (dropped connection),
        // which has no code worth showing the user.
        setSessionsError(
          res.status === 0
            ? t("sessions.offline")
            : t("sessions.backendUnavailable", { status: res.status }),
        );
        return null;
      }
      setSessionsError(null);
      const next = res.data.sessions || [];
      setSessions((prev) => {
        if (reset) {
          return next;
        }
        const seen = new Set(prev.map((s) => s.id));
        return [...prev, ...next.filter((s) => !seen.has(s.id))];
      });
      const nextCur = res.data.nextCursor ?? null;
      setSessionsCursor(nextCur);
      sessionsCursorRef.current = nextCur;
      const hm = !!res.data.hasMore;
      setSessionsHasMore(hm);
      sessionsHasMoreRef.current = hm;
      return next;
    },
    [sessionFilterQ, headers, sessionsProjectOnly, hostProjectRoot],
  );

  useEffect(() => {
    void (async () => {
      const res = await fetchJSON<Record<string, unknown>>(
        "/foxxycode/config",
        {
          headers,
        },
      );
      if (res.ok && res.data) {
        const policy = parseToolsPermissionPolicy(res.data);
        toolsPermissionPolicyRef.current = policy;
        setToolsPermissionPolicy(policy);
        const { applyStartupUiLocaleFromConfig, readUiLocaleFromConfigDoc } =
          await import("./i18n/localeConfig");
        applyStartupUiLocaleFromConfig(readUiLocaleFromConfigDoc(res.data));
        const { setSendMode, readSendModeFromConfigDoc } =
          await import("./i18n/sendModeConfig");
        setSendMode(readSendModeFromConfigDoc(res.data));
        const { setStatusLineEnabled, readStatusLineFromConfigDoc } =
          await import("./chat/statusLineConfig");
        setStatusLineEnabled(readStatusLineFromConfigDoc(res.data));
      }
    })();
  }, [headers]);

  useEffect(() => {
    const ids = new Set(permissionPendingSessionIdsFromStorage());
    for (const row of sessions) {
      if (row.permissionPending) {
        ids.add(row.id);
      }
    }
    const sid = sessionId.trim();
    if (
      sid &&
      items.some((x) => x.type === "permission_prompt" && !x.resolved)
    ) {
      ids.add(sid);
    }
    setPermissionPendingSids(ids);
  }, [sessions, items, sessionId]);

  // /foxxycode/config may resolve after the first loadMessages; re-synthesize permission_prompt rows then.
  useEffect(() => {
    const sid = sessionId.trim();
    if (!sid || !toolsPermissionPolicy) {
      return;
    }
    setItems((prev) => {
      if (prev.length === 0) {
        return prev;
      }
      const merged = mergePermissionPromptsIntoTranscript(
        prev,
        sid,
        toolsPermissionPolicy,
      );
      const hadPending = prev.some(
        (x) => x.type === "permission_prompt" && !x.resolved,
      );
      const hasPending = merged.some(
        (x) => x.type === "permission_prompt" && !x.resolved,
      );
      if (hadPending === hasPending && merged.length === prev.length) {
        return prev;
      }
      return merged;
    });
  }, [toolsPermissionPolicy, sessionId]);

  useEffect(() => {
    const hasBackgroundTurn = sessions.some(
      (s) => !!s.turnActive && s.id !== sessionId,
    );
    const anyLocalComposer = activeComposerSidRef.current.size > 0;
    // With the events stream up, a background turn announces itself, so the poll only
    // has to keep a local composer's stats and unread flags fresh. When it is down (an
    // older server, a proxy that eats SSE) the previous behaviour is restored verbatim
    // rather than leaving the sidebar frozen.
    const pollForBackground = hasBackgroundTurn && !serverEventsConnected;
    if (!anyLocalComposer && !pollForBackground) {
      return;
    }
    const timer = window.setInterval(() => {
      void loadSessionsList(true);
    }, 2000);
    return () => window.clearInterval(timer);
  }, [
    composerActivityEpoch,
    sessions,
    sessionId,
    loadSessionsList,
    serverEventsConnected,
  ]);

  useEffect(() => {
    if (!sessionsOpen) {
      return;
    }
    void loadSessionsList(true);
  }, [sessionsOpen, sessionFilterQ, loadSessionsList]);

  async function loadMessages(
    idOverride?: string,
    opts?: {
      skipSetItems?: boolean;
      preserveOnError?: boolean;
      freshLoad?: boolean;
      expectedEpoch?: number;
      allowApplyWhileActive?: boolean;
      /** Post-turn reconcile: line the composer up with the session's mode. */
      adoptServerMode?: boolean;
      /** modeEditSeqRef as it stood when that turn started. */
      modeEditSeqAtTurnStart?: number;
    },
  ): Promise<TranscriptItem[] | null> {
    const sid = (idOverride ?? sessionId).trim();
    if (!sid) {
      setItems([]);
      return null;
    }
    const requestEpoch =
      opts?.expectedEpoch !== undefined
        ? opts.expectedEpoch
        : currentTranscriptEpoch(sid);
    const canApplySnapshot = () =>
      shouldApplyTranscriptSnapshot({
        requestEpoch,
        currentEpoch: currentTranscriptEpoch(sid),
        activeComposer: activeComposerSidRef.current.has(sid),
        ...(opts?.allowApplyWhileActive !== undefined
          ? { allowWhileActive: opts.allowApplyWhileActive }
          : {}),
      });
    const res = await fetchJSON<{
      messages: Array<any>;
      model?: string;
      selectedModelId?: string;
      selectedReasoning?: string;
      mode?: string;
      memoryTurns?: MemoryTurnApi[];
      subagent?: {
        parentSessionId?: string;
        name?: string;
        taskId?: string;
      } | null;
      readOnly?: boolean;
      uiLog?: Array<{
        id?: string;
        level?: string;
        message?: string;
        userTurnIndex?: number;
        createdAt?: string;
      }>;
    }>(`/foxxycode/sessions/${encodeURIComponent(sid)}/messages`, {
      headers: sid === sessionId ? headers : { [HDR]: sid },
    });
    // Re-read the viewed session after the await: the viewer may have moved
    // on while the request was in flight, and a stale response must neither
    // clear the new session's rows nor merge them into this session's shadow.
    const viewingNow = viewedSessionIdRef.current.trim();
    if (!res.ok || !res.data) {
      if (!opts?.preserveOnError) {
        if (viewingNow === sid && canApplySnapshot()) {
          setItems([]);
        }
      }
      return null;
    }
    if (viewingNow === sid && canApplySnapshot()) {
      // Stash the session's saved selection; an effect applies it once the
      // backends list is loaded (the two fetches race on reload). The reasoning
      // level is later validated by the clamp effect against the chosen model.
      // The effect restores it once per session open — see
      // shouldApplySessionSelection — because this call also runs after every
      // turn and every reconnect.
      setOpenSessionSelection({
        sid,
        model: (res.data.model || res.data.selectedModelId || "").trim(),
        reasoning: (res.data.selectedReasoning || "").trim(),
        mode: (res.data.mode || "").trim(),
      });
      // The backend may have switched the profile itself (a plan run, or the
      // model calling plan_exit). The SSE `mode` frame is the primary signal;
      // this is the recovery for a turn whose stream we did not see whole, and
      // it defers to a Mode the user changed while the turn was running.
      if (opts?.adoptServerMode) {
        const serverMode = (res.data.mode || "").trim();
        if (
          shouldAdoptServerModeAfterTurn({
            serverMode,
            currentMode: modeRef.current,
            knownModes: PROFILE_MODES,
            editSeqAtTurnStart: opts.modeEditSeqAtTurnStart ?? modeEditSeqRef.current,
            editSeqNow: modeEditSeqRef.current,
          })
        ) {
          setMode(serverMode);
        }
      }
      // A child session locks the composer; an ordinary one carries no marker.
      setSubagentTranscript(parseSubagentTranscriptMeta(res.data));
    }
    type UILogRow = {
      id: string;
      level: string;
      message: string;
      createdAt: string;
    };
    const noticesByTurn = new Map<number, UILogRow[]>();
    for (const raw of res.data.uiLog || []) {
      const msg = typeof raw.message === "string" ? raw.message.trim() : "";
      if (!msg) continue;
      const turn =
        typeof raw.userTurnIndex === "number" &&
        Number.isFinite(raw.userTurnIndex) &&
        raw.userTurnIndex >= 1
          ? Math.floor(raw.userTurnIndex)
          : 1;
      const id =
        typeof raw.id === "string" && raw.id.trim() !== ""
          ? raw.id.trim()
          : newId("s");
      const level = (raw.level || "error").trim() || "error";
      const createdAt = typeof raw.createdAt === "string" ? raw.createdAt : "";
      const row: UILogRow = { id, level, message: msg, createdAt };
      const bucket = noticesByTurn.get(turn) ?? [];
      bucket.push(row);
      noticesByTurn.set(turn, bucket);
    }
    for (const [turn, bucket] of noticesByTurn) {
      bucket.sort((a, b) => a.createdAt.localeCompare(b.createdAt));
      noticesByTurn.set(turn, bucket);
    }

    const memByTurn = new Map<number, MemoryTurnApi>();
    for (const row of res.data.memoryTurns || []) {
      if (typeof row.userTurnIndex === "number" && row.userTurnIndex > 0) {
        memByTurn.set(row.userTurnIndex, row);
      }
    }
    const next: TranscriptItem[] = [];
    const pushUiNoticesForTurn = (turn: number) => {
      for (const row of noticesByTurn.get(turn) || []) {
        // Only the two levels the transcript knows how to render; a level a
        // newer server may add stays invisible rather than mis-rendered.
        if (row.level !== "error" && row.level !== "notice") continue;
        next.push({
          id: row.id,
          type: "system_notice",
          // The server calls the neutral level "notice"; the transcript has
          // called it "info" since the re-attach rows, and renders one style
          // for both.
          level: row.level === "notice" ? "info" : "error",
          message: row.message,
          createdAtUtc: row.createdAt,
        });
      }
    };
    const toolIdx = new Map<string, number>();
    let userTurnIdx = 0;
    let thinkingInTurn = 0;
    let pendingAssistant = emptyDeferredAssistant();
    let assistantInTurn = 0;
    /**
     * Emits whatever assistant text has accumulated so far.
     *
     * Assistant text is buffered rather than pushed on sight so that a reply split across
     * several messages reads as one bubble. It therefore has to be flushed before anything
     * that comes *after* it chronologically — the tool calls of the same step, and the
     * reasoning of the next one. Without that, a turn that spoke before calling a tool
     * renders as "user, every tool call, then all the text at the bottom": invisible while a
     * model only talks at the end, glaring the moment a turn is stopped mid-flight and its
     * partial answer is persisted ahead of the tools.
     */
    const flushAssistantForTurn = () => {
      const row = deferredAssistantItem(
        pendingAssistant,
        userTurnIdx,
        assistantInTurn,
      );
      if (row) {
        next.push(row);
        assistantInTurn++;
      }
      pendingAssistant = emptyDeferredAssistant();
    };
    for (const m of res.data.messages || []) {
      const role = (m.role || "").trim();
      // Compaction summary rows travel as user-role messages flagged
      // compaction_summary; render them as their own foldout, not a user turn.
      if ((m as Record<string, unknown>).compaction_summary === true) {
        next.push({
          id: newId("compaction"),
          type: "compaction",
          summary: stripCompactionPreamble(m.content || ""),
        });
        continue;
      }
      if (role === "user") {
        // Flush notices for the previous turn before starting a new one so
        // error notices land at the end of the turn they belong to, not at
        // the top of the next one.
        if (userTurnIdx > 0) {
          flushAssistantForTurn();
          pushUiNoticesForTurn(userTurnIdx);
        }
        userTurnIdx++;
        thinkingInTurn = 0;
        assistantInTurn = 0;
        const cat = readMessageCreatedAtUTC(m as Record<string, unknown>);
        const rawContent = m.content || "";
        const parsedAssets = sessionMessageFiles(
          (m as Record<string, unknown>).files,
          rawContent,
        );
        next.push({
          id: stableUserItemId(userTurnIdx),
          type: "user_message",
          content: rawContent,
          ...(cat ? { createdAtUtc: cat } : {}),
          ...(parsedAssets.length > 0 ? { files: parsedAssets } : {}),
        });
        const mt = memByTurn.get(userTurnIdx);
        if (mt) {
          next.push(memoryTranscriptFromApi(mt));
        }
        continue;
      }
      if (role === "assistant") {
        const pdRaw = (m as Record<string, unknown>).plan_document;
        if (pdRaw && typeof pdRaw === "object" && !Array.isArray(pdRaw)) {
          const pd = pdRaw as Record<string, unknown>;
          const slug = String(pd.slug ?? "").trim();
          if (slug) {
            next.push({
              id: newId("pd"),
              type: "plan_document",
              slug,
              name: String(pd.name ?? ""),
              overview: String(pd.overview ?? ""),
              content: String(pd.content ?? ""),
              body: String(pd.body ?? ""),
              expanded: false,
              ...(pd.path ? { path: String(pd.path) } : {}),
              ...(pd.discarded === true ? { discarded: true } : {}),
              ...(pd.updatedAt ? { updatedAtUtc: String(pd.updatedAt) } : {}),
            });
          }
        }
        const reasoning = (m.reasoning || "").trim();
        if (reasoning) {
          // Reasoning opens a new step, so anything said in the previous one closes here.
          flushAssistantForTurn();
          const dk = reasoningDurationCacheKey(reasoning);
          const cachedMs = dk
            ? reasoningDurationMsByContentRef.current.get(dk)
            : undefined;
          const durRaw = (m as { reasoning_duration_ms?: unknown })
            .reasoning_duration_ms;
          let fromApi: number | undefined;
          if (
            typeof durRaw === "number" &&
            Number.isFinite(durRaw) &&
            durRaw >= 0
          ) {
            fromApi = Math.round(durRaw);
          } else if (typeof durRaw === "string" && durRaw.trim() !== "") {
            const n = Number(durRaw);
            if (Number.isFinite(n) && n >= 0) {
              fromApi = Math.round(n);
            }
          }
          const durationMs = fromApi !== undefined ? fromApi : cachedMs;
          if (fromApi !== undefined && dk.length > 0) {
            reasoningDurationMsByContentRef.current.set(dk, fromApi);
          }
          next.push({
            id: stableThinkingItemId(userTurnIdx, thinkingInTurn++),
            type: "thinking",
            status: "completed",
            content: reasoning,
            ...(durationMs !== undefined ? { durationMs } : {}),
          });
        }
        const content = m.content || "";
        if (content) {
          const acat = readMessageCreatedAtUTC(m as Record<string, unknown>);
          pendingAssistant = appendDeferredAssistant(
            pendingAssistant,
            content,
            acat,
          );
        }
        const tcs = Array.isArray(m.tool_calls) ? m.tool_calls : [];
        // Whatever the model said before reaching for a tool belongs above that tool.
        if (tcs.length > 0) {
          flushAssistantForTurn();
        }
        for (const tc of tcs) {
          const id = tc?.id || "";
          const fn = tc?.function || {};
          const name = (fn?.name || "").trim();
          const args = fn?.arguments || "";
          if (!id) continue;
          if (toolIdx.has(id)) continue;
          const it: Extract<TranscriptItem, { type: "tool_call" }> = {
            id: stableToolCallItemId(id),
            type: "tool_call",
            toolCallId: id,
            status: "pending",
          };
          if (name) it.title = name;
          if (args) it.argsText = args;
          toolIdx.set(id, next.length);
          next.push(it);
        }
        continue;
      }
      if (role === "tool") {
        const id = (m.tool_call_id || "").trim();
        if (!id) continue;
        const idx = toolIdx.get(id);
        if (idx === undefined) {
          const it: Extract<TranscriptItem, { type: "tool_call" }> = {
            id: stableToolCallItemId(id),
            type: "tool_call",
            toolCallId: id,
            status: "completed",
            resultText: m.content || "",
          };
          toolIdx.set(id, next.length);
          next.push(it);
          continue;
        }
        const cur = next[idx] as Extract<TranscriptItem, { type: "tool_call" }>;
        next[idx] = {
          ...cur,
          status: "completed",
          resultText: m.content || "",
        };
      }
    }
    // Flush the final assistant row and notices (no later user message triggers it).
    flushAssistantForTurn();
    pushUiNoticesForTurn(userTurnIdx);

    // Enrich tool calls with persisted previews when available.
    const tcRes = await fetchJSON<{ toolCalls: ToolCallListRow[] }>(
      `/foxxycode/sessions/${encodeURIComponent(sid)}/tool-calls`,
      {
        headers: sid === sessionId ? headers : { [HDR]: sid },
      },
    );
    if (tcRes.ok && tcRes.data?.toolCalls) {
      for (const row of tcRes.data.toolCalls) {
        const id = (row.toolCallId || "").trim();
        if (!id) continue;
        const idx = toolIdx.get(id);
        if (idx === undefined) continue;
        const cur = next[idx] as Extract<TranscriptItem, { type: "tool_call" }>;
        const title = (row.name || cur.title || "").trim() || undefined;
        const kind = (row.kind || cur.kind || "").trim() || undefined;
        const status = (row.status as any) || cur.status;
        const merged: Extract<TranscriptItem, { type: "tool_call" }> = {
          ...cur,
          status,
        };
        if (title) merged.title = title;
        if (kind) merged.kind = kind;
        if (row.argsPreview) {
          const titleLower = (title || "").trim().toLowerCase();
          const pickedArgs =
            titleLower === "question"
              ? pickRicherQuestionToolArgs(cur.argsText, row.argsPreview)
              : pickRicherToolArgs(cur.argsText, row.argsPreview);
          if (pickedArgs) merged.argsText = pickedArgs;
        }
        if (row.resultPreview) merged.resultText = row.resultPreview;
        if (row.resultPreviewTruncated === true)
          merged.resultWasTruncated = true;
        const todoPlan = normalizeTodoPlanSnapshot(row.planSnapshot);
        if (todoPlan !== undefined) merged.todoPlan = todoPlan;
        const st = parseRFC3339ms(row.startedAt);
        const fin = parseRFC3339ms(row.finishedAt);
        if (st != null && fin != null && fin >= st) {
          merged.durationMs = fin - st;
        }
        next[idx] = merged;
      }
    }
    const prevShadow = streamShadowBySidRef.current.get(sid);
    // freshLoad: don't inherit stale items from a previous session (e.g. when first loading a branch).
    const localForMerge = opts?.freshLoad
      ? prevShadow && prevShadow.length > 0
        ? prevShadow
        : undefined
      : prevShadow && prevShadow.length > 0
        ? prevShadow
        : viewingNow === sid
          ? itemsRef.current
          : undefined;
    const mergedTranscript = mergeTranscriptPreferLocalSuffix(
      next,
      localForMerge,
    );
    revokeSupersededUserMessagePreviews(mergedTranscript, localForMerge);
    const mergedBase = preserveUserMessageFiles(
      mergedTranscript,
      localForMerge,
    );
    let merged = reattachLocalQuestionPrompts(mergedBase, localForMerge);
    merged = mergePermissionPromptsIntoTranscript(
      merged,
      sid,
      toolsPermissionPolicyRef.current,
    );
    merged = mergeStoredQuestionPromptsIntoTranscript(merged, sid);
    merged = patchQuestionToolArgsFromPromptRecords(merged, sid);
    // Final row order before preserveTranscriptItemIds does its positional match.
    merged = pinPlanDocumentsToTurnEnd(merged);
    const appliedRaw =
      keepLocalTranscriptIfServerEmpty({
        serverNext: merged,
        sid,
        viewingSid: viewingNow,
        prevShadow,
        prevItems: itemsRef.current,
      }) ?? merged;
    const withStableIds = preserveTranscriptItemIds(
      appliedRaw,
      localForMerge ?? prevShadow ?? itemsRef.current,
    );
    const applied = dedupeAdjacentDuplicateThinkingCompleted(withStableIds);
    const hasPendingPermission = applied.some(
      (x) => x.type === "permission_prompt" && !x.resolved,
    );
    if (canApplySnapshot()) {
      setPermissionPendingSids((prev) => {
        const next = new Set(prev);
        if (hasPendingPermission) {
          next.add(sid);
        } else {
          next.delete(sid);
        }
        return next;
      });
    }
    if (opts?.skipSetItems) {
      if (canApplySnapshot()) {
        streamShadowBySidRef.current.set(sid, applied);
        evictStaleSessionCaches(viewedSessionIdRef.current);
      }
      return applied;
    }

    // Fetch branch points and inject branch_nav items if any exist.
    let withBranches = applied;
    try {
      const brRes = await fetchJSON<{ branchPoints?: BranchPointData[] }>(
        `/foxxycode/sessions/${encodeURIComponent(sid)}/branches`,
        { headers: sid === sessionId ? headers : { [HDR]: sid } },
      );
      if (brRes.ok && brRes.data?.branchPoints?.length) {
        withBranches = deduplicateBranchNavs(
          injectBranchNavItems(
            applied.filter((it) => it.type !== "branch_nav"),
            brRes.data.branchPoints,
          ),
        );
        if (canApplySnapshot() && sid === viewedSessionIdRef.current.trim()) {
          setSessionHashInLocation(sid, { historySidebar: sessionsOpen });
        }
      }
    } catch {
      // ignore — branch nav is optional
    }

    if (!canApplySnapshot()) {
      return withBranches;
    }
    streamShadowBySidRef.current.set(sid, withBranches);
    evictStaleSessionCaches(viewedSessionIdRef.current);
    // The viewer moved on while this fetch was in flight (the user picked
    // another session or went home): keep the shadow for the next visit, but
    // never paint a stale transcript under the current route.
    if (viewedSessionIdRef.current.trim() !== sid) {
      return withBranches;
    }
    if (fadeOutTimerRef.current !== null) {
      clearTimeout(fadeOutTimerRef.current);
      fadeOutTimerRef.current = null;
    }
    setSessionFadingOut(false);
    setItems(withBranches);
    setSessionLoading(false);
    return withBranches;
  }

  function persistComposerDraftBeforeLeave() {
    if (sessionId.trim()) {
      return;
    }
    const text = draft.trim();
    const existing = activeDraftId.trim();
    if (!text && !existing) {
      return;
    }
    const localId = existing || newClientDraftId();
    const rows = upsertClientDraftSession({
      localId,
      draftText: text,
      updatedAt: new Date().toISOString(),
    });
    setClientDraftSessions(rows);
  }

  function switchBranch(id: string) {
    skipLeafResolveRef.current.add(id);
    pickSession(id);
  }

  function pickSession(id: string) {
    if (isRedundantSessionPick(id, sessionId)) {
      return;
    }
    persistComposerDraftBeforeLeave();
    reasoningDurationMsByContentRef.current = new Map();
    if (fadeOutTimerRef.current !== null) {
      clearTimeout(fadeOutTimerRef.current);
      fadeOutTimerRef.current = null;
    }
    // In an IDE tool window the drawer covers the chat, so hand the panel back to
    // the conversation right away and let the skeleton report the load.
    const keepHistoryOpen =
      sessionsOpen && !shouldCloseHistoryOnSessionPick(isEditorEmbed());
    if (sessionsOpen && !keepHistoryOpen) {
      setSessionsOpen(false);
    }
    if (isClientDraftSessionId(id)) {
      setSessionFadingOut(false);
      setItems([]);
      setActiveDraftId(id);
      setSessionId("");
      viewedSessionIdRef.current = "";
      // A local draft is a new chat: it has no stored profile of its own, so it
      // must not inherit the mode of the session being left.
      setMode("agent");
      setOpenSessionSelection(null);
      appliedSelectionSidRef.current = "";
      userTouchedSelectionSidRef.current = "";
      const row = readClientDraftSessions().find((r) => r.localId === id);
      setDraft(row?.draftText || "");
      setDraftHashInLocation(id, { historySidebar: keepHistoryOpen });
      return;
    }
    setSessionLoading(true);
    setActiveDraftId("");
    openSessionFromRoute(id, { historySidebar: keepHistoryOpen });
    if (itemsRef.current.length > 0) {
      setSessionFadingOut(true);
      fadeOutTimerRef.current = setTimeout(() => {
        fadeOutTimerRef.current = null;
        setSessionFadingOut(false);
        setItems([]);
      }, 110);
    } else {
      setItems([]);
    }
    streamShadowBySidRef.current.touch(id);
    evictStaleSessionCaches(id);
  }

  function goHome() {
    persistComposerDraftBeforeLeave();
    setSessionsOpen(false);
    setSchedulerOpen(false);
    setSchedulerEditor(null);
    setTasksOpen(false);
    setTasksSelectedId(null);
    if (fadeOutTimerRef.current !== null) {
      clearTimeout(fadeOutTimerRef.current);
      fadeOutTimerRef.current = null;
    }
    clearSessionRoute();
    setHeroHomeGeneration((g) => g + 1);
    setItems([]);
    setSessionLoading(false);
    setSessionFadingOut(false);
    setDraft("");
    setTokenUsage(null);
    setContextBreakdown(null);
    setDescribePreview(null);
    reasoningDurationMsByContentRef.current = new Map();
    evictStaleSessionCaches("");
    // Drop any stashed session selection so its restore effect cannot reapply
    // the old session's model over the new chat default. Clearing the applied
    // marker lets the same session restore again when it is reopened later.
    setOpenSessionSelection(null);
    appliedSelectionSidRef.current = "";
    userTouchedSelectionSidRef.current = "";
    // A new chat starts in agent; the mode used to survive New chat while the
    // model was reset, so a plan session leaked its profile into the next one.
    setMode("agent");
    if (llmModelIds.length > 0) {
      setLlmModel(
        pickDefaultLlmModelForNewChat({
          backends: llmModelIds,
          cookie: readLlmModelCookie(),
          defaultAgentModel: defaultAgentYamlModel,
        }),
      );
    }
  }

  async function deleteSession(id: string) {
    if (isClientDraftSessionId(id)) {
      const ok = await confirm({
        title: t("app.confirmDeleteDraft"),
        message: t("app.confirmDeleteDraftBody"),
        confirmLabel: t("app.delete"),
        variant: "danger",
      });
      if (!ok) {
        return;
      }
      const rows = removeClientDraftSession(id);
      setClientDraftSessions(rows);
      if (id === activeDraftId || id === sidebarActiveId) {
        setSessionsOpen(false);
        goHome();
      }
      return;
    }
    const ok = await confirm({
      title: t("app.confirmDeleteChat"),
      message: t("app.confirmDeleteChatBody"),
      confirmLabel: t("app.delete"),
      variant: "danger",
    });
    if (!ok) {
      return;
    }
    clearQuestionPromptRecords(id);
    await fetch(`/foxxycode/sessions/${encodeURIComponent(id)}`, {
      method: "DELETE",
      headers,
    });
    setSessions((prev) => prev.filter((s) => s.id !== id));
    const viewingId = sidebarActiveId.trim();
    if (id === viewingId) {
      setSessionsOpen(false);
      goHome();
      return;
    }
    await loadSessionsList(true);
  }

  async function handleBranchSend(text: string, userMsgIdx: number) {
    const sourceSid = sessionId.trim();
    if (!sourceSid) return;

    const showBranchError = (msg: string) => {
      applyStreamItemsForSession(sourceSid, (prev) => [
        ...prev,
        {
          id: newId("s"),
          type: "system_notice" as const,
          level: "error" as const,
          message: msg,
          createdAtUtc: new Date().toISOString(),
        },
      ]);
    };

    let data: { newSessionId?: string; error?: { message?: string } } = {};
    try {
      const res = await fetch(
        `/foxxycode/sessions/${encodeURIComponent(sourceSid)}/branches`,
        {
          method: "POST",
          headers: { ...headers, "Content-Type": "application/json" },
          body: JSON.stringify({ userMessageIndex: userMsgIdx }),
        },
      );
      if (!res.ok) {
        let errMsg = t("app.branchCreationFailed", { status: res.status });
        try {
          const body = (await res.json()) as { error?: { message?: string } };
          if (body?.error?.message) errMsg = body.error.message;
        } catch {
          /* ignore */
        }
        showBranchError(errMsg);
        return;
      }
      data = (await res.json()) as { newSessionId?: string };
    } catch (err) {
      showBranchError(
        t("app.branchCreationError", {
          error: err instanceof Error ? err.message : String(err),
        }),
      );
      return;
    }
    const newSid = (data.newSessionId || "").trim();
    if (!newSid) {
      showBranchError(t("app.branchCreationNoSessionId"));
      return;
    }
    pendingBranchSendRef.current = { text, sid: newSid };
    pickSession(newSid);
  }

  useEffect(() => {
    setEditingUserMsgIdx(null);
    setEditingAssetNote("");
    setEditingFiles([]);
    setSubagentTranscript(null);
    if (!sessionId) {
      setItems([]);
      setDraft("");
      setSessionLoading(false);
      void loadSessionsList(true);
      return;
    }
    const pending = pendingBranchSendRef.current;
    if (pending && pending.sid === sessionId) {
      pendingBranchSendRef.current = null;
      const text = pending.text;
      const branchSid = sessionId;
      void (async () => {
        // Clear any stale shadow cache for the new branch session so loadMessages
        // doesn't inherit a branch_nav from the previous session's items.
        streamShadowBySidRef.current.delete(branchSid);
        // Load the shared prefix first so the user sees prior context while streaming.
        // freshLoad: skip itemsRef.current as localForMerge so old session items don't bleed in.
        await loadMessages(branchSid, { freshLoad: true });
        void streamResponses(text).then(async () => {
          // After streaming completes, inject the branch_nav so the user can navigate threads.
          try {
            const brRes = await fetchJSON<{ branchPoints?: BranchPointData[] }>(
              `/foxxycode/sessions/${encodeURIComponent(branchSid)}/branches`,
              { headers: { [HDR]: branchSid } },
            );
            if (brRes.ok && brRes.data?.branchPoints?.length) {
              applyStreamItemsForSession(branchSid, (prev) =>
                deduplicateBranchNavs(
                  injectBranchNavItems(
                    prev.filter((it) => it.type !== "branch_nav"),
                    brRes.data!.branchPoints!,
                  ),
                ),
              );
              if (branchSid === viewedSessionIdRef.current.trim()) {
                setSessionHashInLocation(branchSid, {
                  historySidebar: sessionsOpen,
                });
              }
            }
          } catch {
            // ignore
          }
        });
      })();
      return;
    }
    setDraft("");
    setTokenUsage(null);
    setContextBreakdown(null);
    tokenBaselineRef.current = { input: 0, output: 0, total: 0 };
    const lifecycle = new AbortController();
    void (async () => {
      // If the user explicitly navigated here via branch nav, skip leaf resolution
      // so they stay on the session they chose rather than being redirected to the newest thread.
      const skipLeaf = skipLeafResolveRef.current.has(sessionId);
      skipLeafResolveRef.current.delete(sessionId);

      if (!skipLeaf) {
        // Resolve the most-recently-active leaf in the branch tree.
        // If a more recent thread exists, navigate there instead of loading this one.
        const leafId = await resolveLatestLeaf(sessionId, async (sid) => {
          const r = await fetchJSON<{ branchPoints?: BranchPointData[] }>(
            `/foxxycode/sessions/${encodeURIComponent(sid)}/branches`,
            { headers: { [HDR]: sid } },
          );
          return r.ok ? (r.data ?? null) : null;
        });
        if (lifecycle.signal.aborted) return;
        if (
          leafId !== sessionId &&
          viewedSessionIdRef.current.trim() === sessionId
        ) {
          openSessionFromRoute(leafId, { historySidebar: sessionsOpen });
          return;
        }
      }

      // The transcript must not queue behind History: the list is a whole-store scan
      // and one more round trip, and blocking on it is what leaves the panel on a
      // spinner after a reload. Start it here, read it only where it is needed.
      const listPromise = loadSessionsList(true);
      const statsPromise = fetchJSON<{ stats?: SessionStats | null }>(
        `/foxxycode/sessions/${encodeURIComponent(sessionId)}/stats`,
        { headers },
      );

      const shadowSnap = streamShadowBySidRef.current.get(sessionId);
      let loaded: TranscriptItem[] | null = null;
      if (
        activeComposerSidRef.current.has(sessionId) &&
        shadowSnap &&
        shadowSnap.length > 0
      ) {
        // pickSession scheduled a fade that clears the rows in 110ms, and this
        // branch is synchronous - so without cancelling it the transcript just
        // restored from the stream's shadow is wiped a moment later, and the
        // chat sits empty until the stream next paints. While a foreground
        // spawn_agent waits on its child that is minutes, which is exactly what
        // it looked like: an empty window with a Stop button in it. The
        // loadMessages path below already cancels the same timer before it
        // paints.
        if (fadeOutTimerRef.current !== null) {
          clearTimeout(fadeOutTimerRef.current);
          fadeOutTimerRef.current = null;
        }
        setSessionFadingOut(false);
        setItems([...shadowSnap]);
      } else {
        // freshLoad when no shadow: prevents stale itemsRef from a previous session
        // bleeding into this session (e.g. React StrictMode double-invoke of effects).
        const noShadow = !shadowSnap || shadowSnap.length === 0;
        loaded = await loadMessages(undefined, {
          freshLoad: noShadow,
          preserveOnError: true,
        });
        if (lifecycle.signal.aborted) {
          return;
        }
        const shAfter = streamShadowBySidRef.current.get(sessionId);
        if (activeComposerSidRef.current.has(sessionId) && shAfter?.length) {
          setItems([...shAfter]);
        } else if (
          !loaded &&
          !shAfter?.length &&
          !activeComposerSidRef.current.has(sessionId)
        ) {
          // Nothing on disk for this id yet (a draft chat, or a read that failed):
          // show it empty rather than the previous session's transcript.
          setItems([]);
        }
      }
      // The chat is resolved either way. A failed or empty read must never leave the
      // panel on a skeleton - that reads as "loading forever" to the user.
      setSessionLoading(false);

      const statsRes = await statsPromise;
      if (lifecycle.signal.aborted) {
        return;
      }
      if (statsRes.ok && statsRes.data?.stats) {
        applySessionStatsPayload(
          statsRes.data.stats,
          viewedSessionIdRef.current.trim() === sessionId,
          sessionId,
        );
      }

      const list = await listPromise;
      if (lifecycle.signal.aborted) {
        return;
      }
      if (!activeComposerSidRef.current.has(sessionId)) {
        const sess = list?.find((s) => s.id === sessionId);
        if (loaded && sess?.turnActive) {
          void rejoinComposerLiveStream(sessionId, loaded);
        } else {
          // The list row is only the fast path: one page deep (limit 30,
          // project-scoped) and a moment stale, so a chat missing from it may still
          // be working. Ask the session itself before deciding it is idle.
          void reconnectLiveStreamIfActive(sessionId);
        }
      }
    })();
    return () => {
      lifecycle.abort();
    };
    // Intentionally sessionId only for loadMessages coalescing; rejoin runs detached.
  }, [sessionId]);

  function upsertToolCall(
    update: Partial<Extract<TranscriptItem, { type: "tool_call" }>> & {
      toolCallId: string;
    },
  ) {
    const targetSid = sessionId.trim();
    if (!targetSid) return;
    applyStreamItemsForSession(targetSid, (prev) => {
      const idx = prev.findIndex(
        (x) => x.type === "tool_call" && x.toolCallId === update.toolCallId,
      );
      if (idx < 0) {
        const itBase: Extract<TranscriptItem, { type: "tool_call" }> = {
          id: newId("t"),
          type: "tool_call",
          toolCallId: update.toolCallId,
          status: (update.status as any) || "pending",
        };
        const it: Extract<TranscriptItem, { type: "tool_call" }> = {
          ...itBase,
        };
        if (update.title !== undefined) it.title = update.title;
        if (update.kind !== undefined) it.kind = update.kind;
        if (update.argsText !== undefined) it.argsText = update.argsText;
        if (update.resultText !== undefined) it.resultText = update.resultText;
        if (update.resultWasTruncated !== undefined)
          it.resultWasTruncated = update.resultWasTruncated;
        if (update.fullResultText !== undefined)
          it.fullResultText = update.fullResultText;
        if (update.todoPlan !== undefined) it.todoPlan = update.todoPlan;
        if (update.startedAtMs !== undefined)
          it.startedAtMs = update.startedAtMs;
        if (update.finishedAtMs !== undefined)
          it.finishedAtMs = update.finishedAtMs;
        if (update.durationMs !== undefined) it.durationMs = update.durationMs;
        return [...prev, it];
      }
      const next = [...prev];
      const cur = next[idx] as Extract<TranscriptItem, { type: "tool_call" }>;
      const nextStarted =
        update.startedAtMs !== undefined ? update.startedAtMs : cur.startedAtMs;
      const nextFinished =
        update.finishedAtMs !== undefined
          ? update.finishedAtMs
          : cur.finishedAtMs;
      const nextDuration =
        update.durationMs !== undefined
          ? update.durationMs
          : nextStarted && nextFinished
            ? Math.max(0, nextFinished - nextStarted)
            : cur.durationMs;
      const merged: Extract<TranscriptItem, { type: "tool_call" }> = {
        ...cur,
        status: (update.status as any) || cur.status,
      };
      if (nextStarted !== undefined) merged.startedAtMs = nextStarted;
      if (nextFinished !== undefined) merged.finishedAtMs = nextFinished;
      if (nextDuration !== undefined) merged.durationMs = nextDuration;
      if (update.title !== undefined) merged.title = update.title;
      if (update.kind !== undefined) merged.kind = update.kind;
      if (update.argsText !== undefined) merged.argsText = update.argsText;
      if (update.resultText !== undefined)
        merged.resultText = update.resultText;
      if (update.resultWasTruncated !== undefined)
        merged.resultWasTruncated = update.resultWasTruncated;
      if (update.fullResultText !== undefined)
        merged.fullResultText = update.fullResultText;
      if (update.todoPlan !== undefined) merged.todoPlan = update.todoPlan;
      next[idx] = merged;
      return next;
    });
  }

  /**
   * When a live composer stream drops before its final [DONE] (e.g. the embedded
   * Chromium throttled/aborted the fetch while the webview was backgrounded), the
   * turn keeps running on the backend. Re-attach to it via the composer relay so the
   * answer keeps rendering without a manual page reload. No-op when the turn already
   * finished or a local composer is already streaming for this session.
   */
  async function reconnectLiveStreamIfActive(
    rawSid: string,
    opts?: { reconcileIfDone?: boolean },
  ): Promise<void> {
    const key = rawSid.trim();
    if (!key) return;
    if (userStoppedSidRef.current.has(key)) {
      userStoppedSidRef.current.delete(key);
      markConnected(key);
      return;
    }
    if (activeComposerSidRef.current.has(key)) {
      markConnected(key);
      return;
    }
    let active = false;
    try {
      const act = await fetchJSON<{ turnActive?: boolean; messageSeq?: number }>(
        `/foxxycode/sessions/${encodeURIComponent(key)}/activity`,
        { headers: { [HDR]: key } },
      );
      if (!act.ok) {
        // The server answered something other than a probe result; treat it as
        // a failed probe so a server that has stopped answering is given up on.
        noteActivityFailure(key);
        markConnected(key);
        return;
      }
      activityFailuresRef.current.delete(key);
      active = !!act.data?.turnActive;
      if (typeof act.data?.messageSeq === "number") {
        lastMessageSeqRef.current.set(key, act.data.messageSeq);
      }
      noteViewedTurnActive(key, active);
    } catch {
      noteActivityFailure(key);
      markConnected(key);
      return;
    }
    if (!active) {
      resetLiveRecoveryState(key);
      markConnected(key);
      // The turn already finished. If we got here from a dropped stream, pull the
      // persisted transcript so a stuck partial assistant is replaced by the result.
      if (opts?.reconcileIfDone && !activeComposerSidRef.current.has(key)) {
        await loadMessages(key, {
          skipSetItems: viewedSessionIdRef.current.trim() !== key,
        });
      }
      return;
    }
    if (activeComposerSidRef.current.has(key)) {
      markConnected(key);
      return;
    }
    const loaded = await loadMessages(key, {
      skipSetItems: viewedSessionIdRef.current.trim() !== key,
    });
    if (!loaded) {
      markConnected(key);
      return;
    }
    if (activeComposerSidRef.current.has(key)) {
      markConnected(key);
      return;
    }
    // addActiveComposer clears the reconnecting flag the moment the stream attaches;
    // this call runs for the whole rejoined turn, so clearing it here would be too late.
    await rejoinComposerLiveStream(key, loaded);
  }

  /**
   * Retry the re-attach, backing off but never giving up on a turn the server
   * still reports as alive. The attempt count only sets the delay now: what
   * ends the retries is the server no longer answering the probe, which is the
   * only evidence that there is nothing to come back to. `delayMs` overrides
   * the backoff for the first, deliberately quick, retry after a clean drop.
   */
  function scheduleLiveStreamReconnect(rawSid: string, delayMs?: number): void {
    const key = rawSid.trim();
    if (!key) return;
    if (userStoppedSidRef.current.has(key)) return;
    if (!shouldKeepWatching(activityFailuresRef.current.get(key) ?? 0)) {
      markConnected(key);
      return;
    }
    const attempts = liveReconnectAttemptsRef.current.get(key) ?? 0;
    liveReconnectAttemptsRef.current.set(key, attempts + 1);
    markReconnecting(key);
    window.setTimeout(() => {
      // Go through the ref so the delayed call uses the current render's closure.
      reconnectLiveStreamRef.current(key, { reconcileIfDone: true });
    }, delayMs ?? liveReconnectDelayMs(attempts));
  }

  /**
   * Forget what this session's recovery loop learned: the retry budget, the
   * failed-probe count and the last transcript size. Called wherever a turn
   * starts, finishes cleanly or is stopped — a new turn must not inherit the
   * previous one's give-up state.
   */
  function resetLiveRecoveryState(rawSid: string): void {
    const key = rawSid.trim();
    if (!key) return;
    liveReconnectAttemptsRef.current.delete(key);
    activityFailuresRef.current.delete(key);
    lastMessageSeqRef.current.delete(key);
  }

  /** One failed /activity probe. Enough of them in a row and the session is dropped. */
  function noteActivityFailure(rawSid: string): void {
    const key = rawSid.trim();
    if (!key) return;
    activityFailuresRef.current.set(
      key,
      (activityFailuresRef.current.get(key) ?? 0) + 1,
    );
  }

  /** Record a turnActive probe, but only for the chat the user is actually looking at. */
  function noteViewedTurnActive(rawSid: string, active: boolean): void {
    const key = rawSid.trim();
    if (!key || viewedSessionIdRef.current.trim() !== key) return;
    setViewedTurnActive(active);
  }

  function stopDiskFallbackPoll(rawSid: string): void {
    const key = rawSid.trim();
    const h = diskFallbackTimerRef.current.get(key);
    if (h === undefined) return;
    window.clearTimeout(h);
    diskFallbackTimerRef.current.delete(key);
  }

  /**
   * Last-resort fallback when the live stream cannot be (re)established — e.g. an
   * embedded browser that keeps killing long-lived fetches. Polls the persisted
   * transcript so the turn's progress still appears without a manual page reload,
   * and stops as soon as a live stream attaches or the turn ends.
   *
   * It runs for as long as the server says the turn is alive. There used to be a
   * ~5-minute tick cap here, reset only by a window focus event — which never
   * fires for someone watching an editor panel, exactly where a delegated turn
   * can run for half an hour. What ends the poll now is the turn ending, a live
   * stream taking over, the user stopping, or the server no longer answering.
   *
   * Each tick is scheduled from the previous one rather than by setInterval, so
   * the cadence can back off while nothing is being written, and the transcript
   * is re-read only when `messageSeq` says it grew.
   */
  function startDiskFallbackPoll(rawSid: string): void {
    const key = rawSid.trim();
    if (!key || diskFallbackTimerRef.current.has(key)) return;
    let firstTick = true;
    let quietTicks = 0;

    const tick = async (): Promise<void> => {
      if (
        activeComposerSidRef.current.has(key) ||
        userStoppedSidRef.current.has(key) ||
        !shouldKeepWatching(activityFailuresRef.current.get(key) ?? 0)
      ) {
        stopDiskFallbackPoll(key);
        return;
      }
      let active = false;
      let messageSeq: number | undefined;
      try {
        const act = await fetchJSON<{ turnActive?: boolean; messageSeq?: number }>(
          `/foxxycode/sessions/${encodeURIComponent(key)}/activity`,
          { headers: { [HDR]: key } },
        );
        if (!act.ok) {
          noteActivityFailure(key);
          return;
        }
        activityFailuresRef.current.delete(key);
        active = !!act.data?.turnActive;
        messageSeq =
          typeof act.data?.messageSeq === "number" ? act.data.messageSeq : undefined;
        noteViewedTurnActive(key, active);
      } catch {
        noteActivityFailure(key);
        return;
      }
      if (activeComposerSidRef.current.has(key)) return;

      const lastSeq = lastMessageSeqRef.current.get(key);
      const reload = shouldReloadTranscript({
        firstTick,
        turnActive: active,
        messageSeq,
        lastMessageSeq: lastSeq,
      });
      quietTicks = reload ? 0 : quietTicks + 1;
      firstTick = false;
      if (messageSeq !== undefined) {
        lastMessageSeqRef.current.set(key, messageSeq);
      }
      if (reload) {
        await loadMessages(key, {
          skipSetItems: viewedSessionIdRef.current.trim() !== key,
          preserveOnError: true,
        });
      }
      if (!active) {
        stopDiskFallbackPoll(key);
        void loadSessionsList(true);
      }
    };

    const schedule = () => {
      // Re-checked every tick: a stop clears the handle, and the poll ends by
      // simply not scheduling the next one.
      const handle = window.setTimeout(() => {
        void (async () => {
          await tick();
          if (diskFallbackTimerRef.current.has(key)) {
            schedule();
          }
        })();
      }, diskFallbackDelayMs(quietTicks));
      diskFallbackTimerRef.current.set(key, handle);
    };
    schedule();
  }

  // Stable handles to the latest closures so once-subscribed listeners and
  // stable useCallbacks always invoke the current render's versions.
  const reconnectLiveStreamRef = useRef<
    (sid: string, opts?: { reconcileIfDone?: boolean }) => void
  >(() => {});
  reconnectLiveStreamRef.current = (
    sid: string,
    opts?: { reconcileIfDone?: boolean },
  ) => {
    void reconnectLiveStreamIfActive(sid, opts);
  };
  scheduleLiveStreamReconnectRef.current = scheduleLiveStreamReconnect;

  // Handlers are read through a ref so the subscription below can mount once: it must
  // survive re-renders, and the callbacks it needs are redefined on every one of them.
  serverEventHandlersRef.current = {
    turnStarted: (sid: string) => {
      void loadSessionsList(true);
      const key = sid.trim();
      if (!key || key !== viewedSessionIdRef.current.trim()) return;
      if (activeComposerSidRef.current.has(key)) return;
      // Reuse the fork's re-attach path rather than opening a relay here: it keeps the
      // user-stopped guard, the activity probe and the reconnect bookkeeping in one place.
      reconnectLiveStreamRef.current(key);
    },
    turnEnded: (sid: string) => {
      void loadSessionsList(true);
      const key = sid.trim();
      if (!key || key !== viewedSessionIdRef.current.trim()) return;
      if (activeComposerSidRef.current.has(key)) return;
      void loadMessages(key, { preserveOnError: true });
      void refreshSessionStats(key);
    },
    providerUsage: providerUsageState.applyPushed,
    configReloaded: () => setConfigEpoch((e) => e + 1),
  };

  useEffect(() => {
    const ctl = new AbortController();
    void subscribeServerEvents({
      onTurnStarted: (sid) => serverEventHandlersRef.current.turnStarted(sid),
      onTurnEnded: (sid) => serverEventHandlersRef.current.turnEnded(sid),
      onProviderUsage: (_sid, usage) =>
        serverEventHandlersRef.current.providerUsage(usage),
      onConfigReloaded: () => serverEventHandlersRef.current.configReloaded(),
      onConnectedChange: setServerEventsConnected,
      signal: ctl.signal,
    });
    return () => ctl.abort();
  }, []);


  // On return to foreground (alt-tab back, tool window shown), re-attach to any
  // in-flight turn whose live stream was cut while the webview was backgrounded.
  useEffect(() => {
    const onForeground = () => {
      if (document.visibilityState !== "visible") return;
      const sid = viewedSessionIdRef.current.trim();
      if (sid) reconnectLiveStreamRef.current(sid);
    };
    document.addEventListener("visibilitychange", onForeground);
    window.addEventListener("focus", onForeground);
    return () => {
      document.removeEventListener("visibilitychange", onForeground);
      window.removeEventListener("focus", onForeground);
    };
  }, []);

  /**
   * Heartbeat on the open chat while this client is not streaming it.
   *
   * Every other re-attach path needs an event we do not get here: a send, a session
   * switch, a window focus. A turn can start with none of those - the autonomous turn a
   * finished background task wakes is the ordinary case - and the user sitting on that
   * chat would see nothing move until they clicked away and back. One HEAD-sized GET
   * every few seconds is what makes such a turn appear at all, and what keeps the context
   * ring honest while it runs.
   */
  useEffect(() => {
    const sid = sessionId.trim();
    if (!sid || generating) {
      return;
    }
    let stopped = false;
    const probe = async () => {
      if (stopped) return;
      // A stream of our own took over, or the user pressed Stop and must not be
      // re-attached behind their back (that flag is consumed by the reconnect path,
      // so this one only reads it).
      if (
        activeComposerSidRef.current.has(sid) ||
        userStoppedSidRef.current.has(sid)
      ) {
        return;
      }
      try {
        const act = await fetchJSON<{ turnActive?: boolean }>(
          `/foxxycode/sessions/${encodeURIComponent(sid)}/activity`,
          { headers: { [HDR]: sid } },
        );
        if (stopped || !act.ok) return;
        const active = !!act.data?.turnActive;
        noteViewedTurnActive(sid, active);
        if (active && !activeComposerSidRef.current.has(sid)) {
          reconnectLiveStreamRef.current(sid);
        }
      } catch {
        // Server not reachable right now; the next tick retries.
      }
    };
    const id = window.setInterval(() => void probe(), VIEWED_ACTIVITY_POLL_MS);
    return () => {
      stopped = true;
      window.clearInterval(id);
    };
  }, [sessionId, generating]);

  async function rejoinComposerLiveStream(
    sid: string,
    baseline: TranscriptItem[],
  ): Promise<void> {
    const key = sid.trim();
    if (!key) return;

    bumpTranscriptEpoch(key);
    relayAbortBySidRef.current.get(key)?.abort();
    const fetchCtl = new AbortController();
    relayAbortBySidRef.current.set(key, fetchCtl);

    addActiveComposer(key);
    // addActiveComposer clears the reconnecting flag, and until the relay actually says
    // something we have nothing but the transcript as it was before this turn started.
    // Deriving a status line from that is how the row froze on the previous turn's last
    // tool for the whole time the server held the request open with no relay to attach
    // to - the case a background task's autonomous turn used to produce.
    markReconnecting(key);
    let relayDelivered = false;
    const noteRelayByte = () => {
      if (relayDelivered) return;
      relayDelivered = true;
      markConnected(key);
    };
    const assistantId = newId("a");
    streamingAssistantBySidRef.current.set(key, assistantId);
    streamShadowBySidRef.current.set(key, [...baseline]);
    if (viewedSessionIdRef.current.trim() === key) {
      setItems([...baseline]);
    }

    // The relay replays the in-flight turn's SSE from the start, so the partial turn
    // output loaded from disk has to go — otherwise the turn's text renders twice.
    // The gate trims on the FIRST relay byte, not up front: when no relay answers
    // (turn already over, backend restarted) the transcript must stay as the user
    // left it instead of being emptied by a replay that never arrives.
    const trimOnFirstReplayByte = createTurnReplayTrimGate();
    const applyStreamItems = (
      fn: (prev: TranscriptItem[]) => TranscriptItem[],
    ) => {
      noteRelayByte();
      applyStreamItemsForSession(key, (prev) =>
        fn(trimOnFirstReplayByte(prev)),
      );
    };

    // Give up on the live stream and let the disk poller show the turn's progress.
    // (The finally block below clears the abort controller and reconciles.)
    const fallBackToDisk = () => {
      streamingAssistantBySidRef.current.delete(key);
      removeActiveComposer(key);
      startDiskFallbackPoll(key);
    };

    // Usage events carry no transcript rows, so they clear the flag on their own:
    // a turn whose first sign of life is a token count is attached all the same.
    const branchTokenUsage = (u: TokenUsage | null) => {
      if (u === null) return;
      noteRelayByte();
      if (viewedSessionIdRef.current.trim() === key) {
        setTokenUsage(u);
        debouncedRefreshSessionStats(key);
      }
    };
    const branchContextUsage = (u: ContextUsageUpdate) => {
      noteRelayByte();
      if (viewedSessionIdRef.current.trim() === key) {
        setContextBreakdown((prev) => withContextUsedTokens(prev, u.used));
        debouncedRefreshSessionStats(key);
      }
    };

    let reconcileOnExit = true;
    try {
      // Resume after the last frame this tab consumed, so a dropped connection costs
      // a gap rather than a replay of the whole turn.
      const resumeFrom = relayLastEventIdBySidRef.current.get(key) ?? "";
      const headers: Record<string, string> = { [HDR]: key };
      if (resumeFrom) {
        headers["Last-Event-ID"] = resumeFrom;
      }
      const res = await fetch(
        `/foxxycode/sessions/${encodeURIComponent(key)}/composer-stream`,
        { headers, signal: fetchCtl.signal },
      );
      if (!res.ok || !res.body) {
        // No relay to attach to (auth, restart, session gone). Keep the transcript
        // and let the poller reveal the turn's progress if it is still running.
        fallBackToDisk();
        return;
      }
      const reader = res.body.getReader();
      const dec = new TextDecoder();
      const carry = { buf: "" };
      const {
        streamErrorMessage,
        streamErrorCode,
        lastEventId,
        desynced,
        endedWithoutDone,
        finalAssistantId,
        flushToolQueue,
        finishThinking,
        ensureAssistant,
      } = await consumeComposerSseReader({
        reader,
        dec,
        carry,
        assistantId,
        applyStreamItems,
        setTokenUsage: branchTokenUsage,
        setContextUsage: branchContextUsage,
        tokenBaselineRef,
        reasoningDurationMsByContentRef,
        newId,
        applyMemoryPhaseToItems,
        applyMemoryChunkToItems,
        onQuestion: handleComposerSseQuestion,
        onPermission: handleComposerSsePermission,
        onProviderUsage: providerUsageState.applyPushed,
        onCompaction: () =>
          debouncedRefreshSessionStats(viewedSessionIdRef.current.trim()),
        onMcpConnecting: (connecting: boolean) =>
          setMcpConnecting(key, connecting),
        onLlmRetrying: (retrying: boolean) => setLlmRetrying(key, retrying),
        onDesignPlan: (slug: string) =>
          handleComposerSseDesignPlan(key, slug),
        onModeChanged: (m: string) => onServerModeChanged(key, m),
      });

      const syncAssistantFromServer = async () => {
        try {
          const res2 = await fetchJSON<{ messages: Array<any> }>(
            `/foxxycode/sessions/${encodeURIComponent(key)}/messages`,
            { headers: { [HDR]: key } },
          );
          if (!res2.ok || !res2.data?.messages) return false;
          let last = "";
          let lastCreated: string | undefined;
          for (const m of res2.data.messages) {
            if ((m.role || "").trim() !== "assistant") continue;
            const c = (m.content || "").trim();
            if (c) {
              last = c;
              lastCreated = readMessageCreatedAtUTC(
                m as Record<string, unknown>,
              );
            }
          }
          if (!last) return false;
          ensureAssistant();
          applyStreamItems((prev) =>
            prev.map((it) =>
              it.type === "assistant_message" && it.id === finalAssistantId
                ? {
                    ...it,
                    content: last,
                    ...(lastCreated ? { createdAtUtc: lastCreated } : {}),
                  }
                : it,
            ),
          );
          return true;
        } catch {
          return false;
        }
      };

      if (lastEventId) {
        relayLastEventIdBySidRef.current.set(key, lastEventId);
      }
      // The relay had already dropped frames this tab never saw, so the transcript on
      // screen has a hole in it. The persisted messages are the source of truth.
      if (desynced) {
        await loadMessages(key, {
          skipSetItems: viewedSessionIdRef.current.trim() !== key,
          preserveOnError: true,
        });
      }

      if (isNoLiveTurnRelayError(streamErrorCode, streamErrorMessage)) {
        // The relay has nothing to attach to. That is a state, not a failure: the
        // turn may have finished while we reconnected, or it runs somewhere this
        // process cannot see. Reconcile from disk and keep watching if it is live.
        flushToolQueue();
        finishThinking();
        fallBackToDisk();
        return;
      }

      if (streamErrorMessage) {
        flushToolQueue();
        finishThinking();
        const errText = streamErrorMessage;
        applyStreamItems((prev) => {
          const withoutEmptyAssistant = prev.filter(
            (it) =>
              !(
                it.type === "assistant_message" &&
                it.id === finalAssistantId &&
                !it.content.trim()
              ),
          );
          return [
            ...withoutEmptyAssistant,
            {
              id: newId("s"),
              type: "system_notice",
              level: "error" as const,
              message: errText,
              createdAtUtc: new Date().toISOString(),
            },
          ];
        });
        void loadSessionsList(true);
        return;
      }

      if (endedWithoutDone) {
        // Relay was cut before [DONE]; the turn is likely still running. Schedule
        // another re-attach and leave the transcript as-is for now. Poll the
        // persisted rows meanwhile so progress shows even if every retry fails.
        flushToolQueue();
        finishThinking();
        reconcileOnExit = false;
        scheduleLiveStreamReconnect(key);
        startDiskFallbackPoll(key);
        return;
      }
      resetLiveRecoveryState(key);

      flushToolQueue();
      finishThinking();
      ensureAssistant({
        streaming: false,
        createdAtUtc: new Date().toISOString(),
      });

      void loadSessionsList(true);
      let ok = await syncAssistantFromServer();
      for (let i = 0; i < 10 && !ok; i++) {
        await new Promise((r) => setTimeout(r, 500));
        ok = await syncAssistantFromServer();
      }
      const viewing = viewedSessionIdRef.current.trim();
      const reconcileEpoch = bumpTranscriptEpoch(key);
      await loadMessages(key, {
        skipSetItems: viewing !== key,
        expectedEpoch: reconcileEpoch,
        allowApplyWhileActive: true,
      });
      reconcileOnExit = false;
    } catch {
      // AbortError when relay superseded or fetch aborted
    } finally {
      if (relayAbortBySidRef.current.get(key) === fetchCtl) {
        relayAbortBySidRef.current.delete(key);
      }
      streamingAssistantBySidRef.current.delete(key);
      removeActiveComposer(key);
      // A relay this client cut short (its own POST or a newer relay took
      // over) did not end the turn; only a stream that ran to its end
      // spent quota.
      if (!fetchCtl.signal.aborted) noteUsageTurnEnded(key);
      // The session is no longer pinned; bound the cache now rather than
      // only after the reconciliation below succeeds.
      evictStaleSessionCaches(viewedSessionIdRef.current);
      void loadSessionsList(true);
      if (reconcileOnExit) {
        const viewing = viewedSessionIdRef.current.trim();
        void loadMessages(key, { skipSetItems: viewing !== key });
      }
      void refreshSessionStats(key);
      markViewedSessionActivityRead(key);
    }
  }

  async function streamResponses(
    text: string,
    opts?: { modeOverride?: string; runPlanSlug?: string; files?: File[] },
  ) {
    const abortCtl = new AbortController();
    let postSessionKey = "";
    let completedNormally = false;
    let assistantStreamId = "";
    // A Mode the user switches while this turn runs is a choice for the NEXT
    // turn, so the post-turn reconcile must not overwrite it with the mode the
    // session is in now.
    const modeEditSeqAtTurnStart = modeEditSeqRef.current;
    const isNewChatFirstSend = !sessionId.trim();
    let releaseSessionId: ((id: string) => void) | undefined;
    const sessionIdWhenKnown = isNewChatFirstSend
      ? new Promise<string>((resolve) => {
          releaseSessionId = resolve;
        })
      : null;

    let sidEffective = "";

    try {
      let sid = sessionId;
      if (!sid) {
        sid = randomSessionId();
        migrateWorkspaceAtRecents(WORKSPACE_AT_RECENTS_NO_SESSION_KEY, sid);
        await applyPendingWorkspace(sid);
        if (activeDraftId.trim()) {
          setClientDraftSessions(
            removeClientDraftSession(activeDraftId.trim()),
          );
          setActiveDraftId("");
        }
        openSessionFromRoute(sid);
      }
      sidEffective = sid;
      postSessionKey = sid.trim();
      // Fresh send supersedes any prior user-stop / reconnect budget for this session.
      userStoppedSidRef.current.delete(postSessionKey);
      resetLiveRecoveryState(postSessionKey);
      stopDiskFallbackPoll(postSessionKey);
      bumpTranscriptEpoch(postSessionKey);
      // Own the transcript before any asynchronous attachment preparation so
      // a concurrent GET /messages cannot erase the optimistic user row.
      addActiveComposer(postSessionKey);
      postAbortBySidRef.current.set(postSessionKey, abortCtl);
      relayAbortBySidRef.current.get(postSessionKey)?.abort();
      relayAbortBySidRef.current.delete(postSessionKey);

      let streamKey = postSessionKey;
      const applyStreamItems = (
        fn: (prev: TranscriptItem[]) => TranscriptItem[],
      ) => applyStreamItemsForSession(streamKey, fn);

      const branchTokenUsage = (u: TokenUsage | null) => {
        if (u === null) return;
        if (viewedSessionIdRef.current.trim() === streamKey) {
          setTokenUsage(u);
          debouncedRefreshSessionStats(streamKey);
        }
      };
      const branchContextUsage = (u: ContextUsageUpdate) => {
        if (viewedSessionIdRef.current.trim() === streamKey) {
          setContextBreakdown((prev) => withContextUsedTokens(prev, u.used));
          debouncedRefreshSessionStats(streamKey);
        }
      };

      if (isNewChatFirstSend && sessionIdWhenKnown) {
        // The backend hidden "title" agent generates and persists an LLM session title after the
        // first exchange (for all clients). Refresh the list a few times so it surfaces in the UI
        // once it lands. The frontend no longer generates or pins a title itself.
        scheduleSessionTitleRefresh({
          sessionIdPromise: sessionIdWhenKnown,
          refresh: () => {
            void loadSessionsList(true);
          },
        });
      }

      const hdrs = sid ? { [HDR]: sid } : {};
      const userItem: TranscriptItem = {
        id: newId("u"),
        type: "user_message",
        content: text,
        createdAtUtc: new Date().toISOString(),
        ...(opts?.files && opts.files.length > 0
          ? {
              files: optimisticUserFiles(opts.files),
            }
          : {}),
      };
      const assistantId = newId("a");
      assistantStreamId = assistantId;
      streamingAssistantBySidRef.current.set(streamKey, assistantId);
      const viewingNow = viewedSessionIdRef.current.trim();
      const baseItems = pickStreamMutationBase({
        mutationSessionId: streamKey,
        viewingSid: viewingNow,
        shadow: streamShadowBySidRef.current.get(streamKey),
        hasActiveComposer: activeComposerSidRef.current.has(streamKey),
        itemsWhenViewingMatches: itemsRef.current,
        assumeActiveForBase: true,
      });
      const nextShadow = [...baseItems, userItem];
      streamShadowBySidRef.current.set(streamKey, nextShadow);
      if (viewingNow === streamKey) {
        setItems(nextShadow);
      }
      if (viewingNow === streamKey) {
        setTokenUsage(null);
      }

      const reqBody: Record<string, unknown> = {
        model: opts?.modeOverride || mode || "agent",
        input: text,
        stream: true,
      };
      const atts = extractAtFileAttachments(text);
      const profileModel = (PROFILE_MODES as readonly string[]).includes(mode);
      if (atts.length > 0 && profileModel) {
        // A ranged mention attaches the pasted literal when we still hold it;
        // otherwise the backend reads the line range from the file.
        reqBody.attachments = atts.map((a) => {
          if (a.startLine == null || a.endLine == null) {
            return { path: a.path };
          }
          const literal = pasteLiteralsRef.current.get(
            `${a.path}:${a.startLine}-${a.endLine}`,
          );
          return {
            path: a.path,
            source: {
              ...(literal != null ? { literal } : {}),
              startLine: a.startLine,
              endLine: a.endLine,
            },
          };
        });
        const wk = sid.trim() || WORKSPACE_AT_RECENTS_NO_SESSION_KEY;
        for (const a of atts) {
          recordWorkspaceAtRecent(wk, { path_rel: a.path, kind: "file" });
        }
        pasteLiteralsRef.current.clear();
      }
      if (opts?.files && opts.files.length > 0) {
        const inlineFiles = await Promise.all(
          opts.files.map(
            (f) =>
              new Promise<{ name: string; data_url: string }>(
                (resolve, reject) => {
                  const reader = new FileReader();
                  reader.onload = () =>
                    resolve({
                      name: f.name,
                      data_url: reader.result as string,
                    });
                  reader.onerror = reject;
                  reader.readAsDataURL(f);
                },
              ),
          ),
        );
        reqBody.inline_files = inlineFiles;
      }
      const yamlSel = llmModel.trim();
      const reasoningSel = llmReasoning.trim();
      const runSlug = (opts?.runPlanSlug || "").trim();
      if (yamlSel || reasoningSel || runSlug) {
        const meta: Record<string, string> = {};
        if (yamlSel) meta.model = yamlSel;
        if (reasoningSel) meta.reasoning = reasoningSel;
        if (runSlug) meta.runPlanSlug = runSlug;
        reqBody.metadata = meta;
      }
      const res = await fetch("/v1/responses", {
        method: "POST",
        headers: { ...hdrs, "Content-Type": "application/json" },
        body: JSON.stringify(reqBody),
        signal: abortCtl.signal,
      });

      if (res.status === 409) {
        let parsedBody: unknown = null;
        try {
          parsedBody = await res.json();
        } catch {
          // ignore
        }
        const busy = parseSessionBusyResponse(res.status, parsedBody);
        const serverMsg = busy.message.trim();
        const busySid = (busy.sessionId || postSessionKey).trim();

        // The turn that owns this chat is still running server-side (typically a
        // long request the user reloaded away from). Nothing was accepted, so put
        // the text back in the composer, drop the optimistic row, and open the
        // running turn instead of leaving a dead-end error behind.
        applyStreamItems((prev) => [
          ...prev.filter((it) => it.id !== userItem.id),
          {
            id: newId("s"),
            type: "system_notice",
            level: busy.busy ? ("info" as const) : ("error" as const),
            message: busy.busy
              ? t("app.chatBusyReattached")
              : serverMsg || t("app.chatBusy"),
            createdAtUtc: new Date().toISOString(),
          },
        ]);
        postAbortBySidRef.current.delete(postSessionKey);
        streamingAssistantBySidRef.current.delete(postSessionKey);
        completedNormally = true;
        if (busy.busy) {
          setDraft(text);
          if (busySid && viewedSessionIdRef.current.trim() !== busySid) {
            openSessionFromRoute(busySid);
          }
          // Detached: the finally block below still has to release this send's
          // composer state before the re-attach can claim it.
          void Promise.resolve().then(() =>
            reconnectLiveStreamIfActive(busySid, { reconcileIfDone: true }),
          );
        }
        return;
      }

      const sidHdr = res.headers.get(HDR);
      if (sidHdr && sidHdr !== sid) {
        const oldKey = postSessionKey;
        migrateWorkspaceAtRecents(sid, sidHdr);
        sidEffective = sidHdr;
        postSessionKey = sidHdr.trim();
        streamKey = postSessionKey;
        bumpTranscriptEpoch(postSessionKey);
        postAbortBySidRef.current.delete(oldKey);
        postAbortBySidRef.current.set(postSessionKey, abortCtl);
        relayAbortBySidRef.current.get(oldKey)?.abort();
        relayAbortBySidRef.current.delete(oldKey);
        streamShadowBySidRef.current.rename(oldKey, postSessionKey);
        const relayCursor = relayLastEventIdBySidRef.current.get(oldKey);
        relayLastEventIdBySidRef.current.delete(oldKey);
        if (relayCursor !== undefined) {
          relayLastEventIdBySidRef.current.set(postSessionKey, relayCursor);
        }
        streamingAssistantBySidRef.current.delete(oldKey);
        streamingAssistantBySidRef.current.set(postSessionKey, assistantId);
        openSessionFromRoute(sidHdr);
        setDescribePreview((p) =>
          p?.sessionId === sid ? { ...p, sessionId: sidHdr } : p,
        );
        setSessions((prev) =>
          prev.map((s) => (s.id === sid ? { ...s, id: sidHdr } : s)),
        );
        removeActiveComposer(oldKey);
        addActiveComposer(postSessionKey);
      }
      releaseSessionId?.(sidEffective);

      if (!res.ok || !res.body) {
        const msg = !res.body
          ? t("app.emptyResponseBody")
          : t("app.requestFailedWithStatus", { status: res.status });
        applyStreamItems((prev) => [
          ...prev,
          {
            id: newId("s"),
            type: "system_notice",
            level: "error" as const,
            message: msg,
            createdAtUtc: new Date().toISOString(),
          },
        ]);
        postAbortBySidRef.current.delete(postSessionKey);
        streamingAssistantBySidRef.current.delete(postSessionKey);
        completedNormally = true;
        return;
      }

      const reader = res.body.getReader();
      const dec = new TextDecoder();
      const carry = { buf: "" };

      const {
        streamErrorMessage,
        endedWithoutDone,
        finalAssistantId,
        flushToolQueue,
        finishThinking,
        ensureAssistant,
      } = await consumeComposerSseReader({
        reader,
        dec,
        carry,
        assistantId,
        applyStreamItems,
        setTokenUsage: branchTokenUsage,
        setContextUsage: branchContextUsage,
        tokenBaselineRef,
        reasoningDurationMsByContentRef,
        newId,
        applyMemoryPhaseToItems,
        applyMemoryChunkToItems,
        onQuestion: handleComposerSseQuestion,
        onPermission: handleComposerSsePermission,
        onProviderUsage: providerUsageState.applyPushed,
        onCompaction: () =>
          debouncedRefreshSessionStats(viewedSessionIdRef.current.trim()),
        onMcpConnecting: (connecting: boolean) =>
          setMcpConnecting(streamKey, connecting),
        onLlmRetrying: (retrying: boolean) =>
          setLlmRetrying(streamKey, retrying),
        onDesignPlan: (slug: string) =>
          handleComposerSseDesignPlan(streamKey, slug),
        onModeChanged: (m: string) => onServerModeChanged(streamKey, m),
      });

      const syncAssistantFromServer = async () => {
        try {
          const res2 = await fetchJSON<{ messages: Array<any> }>(
            `/foxxycode/sessions/${encodeURIComponent(sidEffective)}/messages`,
            { headers: { [HDR]: sidEffective } },
          );
          if (!res2.ok || !res2.data?.messages) return false;
          let last = "";
          let lastCreated: string | undefined;
          for (const m of res2.data.messages) {
            if ((m.role || "").trim() !== "assistant") continue;
            const c = (m.content || "").trim();
            if (c) {
              last = c;
              lastCreated = readMessageCreatedAtUTC(
                m as Record<string, unknown>,
              );
            }
          }
          if (!last) return false;
          ensureAssistant();
          applyStreamItems((prev) =>
            prev.map((it) =>
              it.type === "assistant_message" && it.id === finalAssistantId
                ? {
                    ...it,
                    content: last,
                    ...(lastCreated ? { createdAtUtc: lastCreated } : {}),
                  }
                : it,
            ),
          );
          return true;
        } catch {
          return false;
        }
      };

      if (streamErrorMessage) {
        flushToolQueue();
        finishThinking();
        const errText = streamErrorMessage;
        applyStreamItems((prev) => {
          const withoutEmptyAssistant = prev.filter(
            (it) =>
              !(
                it.type === "assistant_message" &&
                it.id === finalAssistantId &&
                !it.content.trim()
              ),
          );
          return [
            ...withoutEmptyAssistant,
            {
              id: newId("s"),
              type: "system_notice",
              level: "error" as const,
              message: errText,
              createdAtUtc: new Date().toISOString(),
            },
          ];
        });
        void loadSessionsList(true);
        completedNormally = true;
        return;
      }

      if (endedWithoutDone) {
        // Stream cut before [DONE] (e.g. the embedded browser throttled a
        // backgrounded webview). Deliberately leave completedNormally=false: the
        // finally block then reconciles the transcript from disk AND schedules the
        // re-attach. Showing the persisted progress must not depend on the
        // re-attach succeeding, otherwise a webview that keeps dropping streams
        // renders nothing at all until the user reloads.
        flushToolQueue();
        finishThinking();
        return;
      }
      resetLiveRecoveryState(postSessionKey);

      flushToolQueue();

      finishThinking();
      ensureAssistant({
        streaming: false,
        createdAtUtc: new Date().toISOString(),
      });

      void loadSessionsList(true);
      const kProbe = postSessionKey.trim();
      let mergedForSyncProbe: TranscriptItem[] = [];
      for (let attempt = 0; attempt < 40; attempt++) {
        const sh = streamShadowBySidRef.current.get(kProbe);
        if (sh && sh.length > 0) {
          mergedForSyncProbe = sh;
        } else if (viewedSessionIdRef.current.trim() === kProbe) {
          mergedForSyncProbe = itemsRef.current;
        } else {
          mergedForSyncProbe = sh ?? [];
        }
        if (transcriptHasFilledAssistant(mergedForSyncProbe, finalAssistantId)) {
          break;
        }
        await new Promise((r) => setTimeout(r, 16));
      }
      const localStreamingAssistantReady = transcriptHasFilledAssistant(
        mergedForSyncProbe,
        finalAssistantId,
      );
      let ok = localStreamingAssistantReady;
      if (!ok) {
        ok = await syncAssistantFromServer();
        for (let i = 0; i < 10 && !ok; i++) {
          await new Promise((r) => setTimeout(r, 500));
          ok = await syncAssistantFromServer();
        }
      }
      const viewingEnd = viewedSessionIdRef.current.trim();
      const reconcileEpoch = bumpTranscriptEpoch(sidEffective);
      await loadMessages(sidEffective, {
        skipSetItems: viewingEnd !== postSessionKey,
        preserveOnError: true,
        expectedEpoch: reconcileEpoch,
        allowApplyWhileActive: true,
        adoptServerMode: viewingEnd === postSessionKey,
        modeEditSeqAtTurnStart,
      });
      void refreshSessionStats(sidEffective);
      markViewedSessionActivityRead(sidEffective);
      completedNormally = true;
    } catch (_err: unknown) {
      // AbortError stops the stream client-side after optional POST cancel
    } finally {
      postAbortBySidRef.current.delete(postSessionKey);
      if (!completedNormally) {
        // Only patch the transcript when this turn actually produced a bubble; a
        // stream cut before the first token leaves assistantStreamId empty, and the
        // disk reconcile below must still run (that is the "always show progress"
        // fallback when the live stream never delivers).
        if (assistantStreamId && postSessionKey.trim() !== "") {
          const now = Date.now();
          const patchIncomplete = (prev: TranscriptItem[]) =>
            prev.map((it) => {
              if (it.type === "thinking" && it.status === "in_progress") {
                const dur = Math.max(0, now - (it.startedAtMs || now));
                const nextIt = {
                  ...it,
                  status: "completed" as const,
                  durationMs: dur,
                };
                const dk = reasoningDurationCacheKey(nextIt.content);
                if (dk.length > 0)
                  reasoningDurationMsByContentRef.current.set(dk, dur);
                return nextIt;
              }
              // A turn can span several bubbles; clear whichever is still live
              // rather than only the id this turn started with.
              if (it.type === "assistant_message" && it.streaming === true) {
                return { ...it, streaming: false };
              }
              return it;
            });
          applyStreamItemsForSession(postSessionKey, patchIncomplete);
        }
        const viewingFin = viewedSessionIdRef.current.trim();
        void loadMessages(sidEffective, {
          skipSetItems: viewingFin !== postSessionKey.trim(),
          preserveOnError: true,
          adoptServerMode: viewingFin === postSessionKey.trim(),
          modeEditSeqAtTurnStart,
        });
        void loadSessionsList(true);
        markViewedSessionActivityRead(postSessionKey.trim());
        // The turn may still be running server-side (stream cut, not a user stop):
        // re-attach so the rest renders live, and poll the persisted transcript
        // meanwhile so progress still appears if every re-attach attempt fails.
        scheduleLiveStreamReconnect(postSessionKey.trim());
        startDiskFallbackPoll(postSessionKey.trim());
      }
      removeActiveComposer(postSessionKey);
      streamingAssistantBySidRef.current.delete(postSessionKey);
      releaseSessionId?.(sidEffective);
      // A background stream that just finished on a no-longer-recent session
      // should release its transcript without waiting for the next navigation.
      evictStaleSessionCaches(viewedSessionIdRef.current);
    }
  }

  function stopActiveGeneration(): void {
    const sid = sessionId.trim();
    if (!sid) return;
    // Mark as user-stopped so a resulting stream drop is not auto-rejoined.
    userStoppedSidRef.current.add(sid);
    resetLiveRecoveryState(sid);
    stopDiskFallbackPoll(sid);
    // Always send the server-side cancel so Stop works even after page reload.
    // Fire-and-forget: if the connection is already gone the turn is unreachable
    // anyway, and an escaping rejection would paint the IDE panel's error overlay.
    void fetch(`/foxxycode/sessions/${encodeURIComponent(sid)}/cancel`, {
      method: "POST",
      headers: { [HDR]: sid },
    }).catch(() => {
      /* ignore */
    });
    // Also abort the in-progress fetch request if we have one from this page session.
    postAbortBySidRef.current.get(sid)?.abort();
  }

  const maxContextTokens = useMemo(() => {
    const row = modelInfos.find((m) => m.id === llmModel);
    return row?.maxContextTokens || 128000;
  }, [modelInfos, llmModel]);

  const llmModelMultimodal = useMemo(() => {
    const row = modelInfos.find((m) => m.id === llmModel);
    return row?.multimodal ?? false;
  }, [modelInfos, llmModel]);

  const llmReasoningLevels = useMemo(() => {
    const row = modelInfos.find((m) => m.id === llmModel);
    return row?.reasoningLevels ?? [];
  }, [modelInfos, llmModel]);

  // Keep the selected reasoning level valid for the current model: keep the user's
  // pick when the new model still offers it, else fall back (cookie -> model default).
  useEffect(() => {
    const row = modelInfos.find((m) => m.id === llmModel);
    const levels = row?.reasoningLevels ?? [];
    setLlmReasoning((prev) =>
      pickReasoningLevel({
        levels,
        cookie: readReasoningCookie(),
        sessionLevel: prev,
        modelDefault: row?.reasoningDefault ?? null,
      }),
    );
  }, [llmModel, modelInfos]);

  const onLlmReasoningChange = useCallback(
    (level: string) => {
      const lv = level.trim();
      if (!lv) {
        return;
      }
      setLlmReasoning(lv);
      writeReasoningCookie(lv);
      const sid = sessionId.trim();
      if (!sid) {
        return;
      }
      void fetch(`/foxxycode/sessions/${encodeURIComponent(sid)}`, {
        method: "PATCH",
        headers: { ...headers, "Content-Type": "application/json" },
        body: JSON.stringify({ selectedReasoning: lv }),
      }).catch(() => {
        // The choice is already applied locally and in the cookie; a lost write
        // must not surface as an unhandled rejection.
      });
    },
    [sessionId, headers],
  );

  const onLlmModelChange = useCallback(
    (id: string) => {
      const mid = id.trim();
      if (!mid) {
        return;
      }
      setLlmModel(mid);
      writeLlmModelCookie(mid);
      const sid = sessionId.trim();
      if (!sid || !llmModelIds.includes(mid)) {
        return;
      }
      // The pick outranks whatever the session had stored, so a reconcile still
      // in flight cannot revert it.
      userTouchedSelectionSidRef.current = sid;
      // A client draft has no server session yet; the first turn persists the
      // pick through metadata.model, and a PATCH here would only 404.
      if (isClientDraftSessionId(sid)) {
        return;
      }
      void fetch(`/foxxycode/sessions/${encodeURIComponent(sid)}`, {
        method: "PATCH",
        headers: { ...headers, "Content-Type": "application/json" },
        body: JSON.stringify({ selectedModelId: mid }),
      }).catch(() => {
        // The choice is already applied locally and in the cookie; a lost write
        // must not surface as an unhandled rejection.
      });
    },
    [sessionId, llmModelIds, headers],
  );

  // A Mode switch is a session change, not a client preference: it is stored
  // right away so a reload comes back in the same profile, and it still rides
  // the next turn as the top-level `model` of POST /v1/responses.
  const onModeChange = useCallback(
    (next: string) => {
      const m = next.trim();
      if (!m || !(PROFILE_MODES as readonly string[]).includes(m)) {
        return;
      }
      setMode(m);
      modeEditSeqRef.current += 1;
      const sid = sessionId.trim();
      if (!sid) {
        return;
      }
      userTouchedSelectionSidRef.current = sid;
      if (isClientDraftSessionId(sid)) {
        return;
      }
      void fetch(`/foxxycode/sessions/${encodeURIComponent(sid)}`, {
        method: "PATCH",
        headers: { ...headers, "Content-Type": "application/json" },
        body: JSON.stringify({ mode: m }),
      }).catch(() => {
        // The mode is already applied locally; a lost write must not surface as
        // an unhandled rejection.
      });
    },
    [sessionId, headers],
  );

  // The backend switched the profile on its own (a plan run, or the model
  // calling plan_exit). Adopting it keeps the composer from posting a mode the
  // session has already left, which the next turn would write back. A relay we
  // watch in the background must not repaint the composer of another chat, so
  // the frame only lands when it names the session on screen.
  const onServerModeChanged = useCallback((sid: string, next: string) => {
    const m = next.trim();
    if (!m || !(PROFILE_MODES as readonly string[]).includes(m)) {
      return;
    }
    if (sid.trim() !== viewedSessionIdRef.current.trim()) {
      return;
    }
    setMode(m);
  }, []);

  const contextPct = useMemo(
    () => contextUsagePercent(maxContextTokens, contextBreakdown),
    [maxContextTokens, contextBreakdown],
  );

  const onSchedulerRunJob = useCallback(
    async (jobId: string) => {
      const r = await schedulerRunJob(jobId);
      if (!r.ok) {
        setSchedulerListError(r.message);
        return;
      }
      void refreshSchedulerJobs({ silent: true });
    },
    [refreshSchedulerJobs],
  );

  const onSchedulerCancelJob = useCallback(
    async (jobId: string) => {
      const r = await schedulerCancelJob(jobId);
      if (!r.ok) {
        setSchedulerListError(r.message);
        return;
      }
      void refreshSchedulerJobs({ silent: true });
    },
    [refreshSchedulerJobs],
  );

  const openSchedulerFromNav = useCallback(() => {
    if (schedulerHttpLinked !== true) {
      return;
    }
    setSessionsOpen(false);
    setTasksOpen(false);
    setTasksSelectedId(null);
    setSchedulerOpen(true);
    setSchedulerEditor(null);
    setSchedulerListHash();
  }, [schedulerHttpLinked]);

  const openMiniAppsFromNav = useCallback(() => {
    if (miniAppsHttpLinked !== true) return;
    setSessionsOpen(false);
    setSchedulerOpen(false);
    setSchedulerEditor(null);
    setTasksOpen(false);
    setTasksSelectedId(null);
    setSettingsRoute(false);
    setMiniAppsOpen(true);
    setMiniAppsAppId(null);
    setMiniAppsHash();
  }, [miniAppsHttpLinked]);

  const closeMiniApps = useCallback(() => {
    setMiniAppsOpen(false);
    setMiniAppsAppId(null);
    const sid = sessionId.trim();
    if (sid) setSessionHashInLocation(sid);
    else if (window.location.hash) {
      history.replaceState(
        null,
        "",
        `${window.location.pathname}${window.location.search}`,
      );
    }
  }, [sessionId]);

  const createMiniAppFromSession = useCallback(() => {
    if (miniAppsHttpLinked !== true || !sessionId.trim()) return;
    openMiniAppsFromNav();
    setMiniAppsHash();
  }, [miniAppsHttpLinked, openMiniAppsFromNav, sessionId]);

  const openTasksFromNav = useCallback(() => {
    const sid = sessionId.trim();
    if (!sid) {
      return;
    }
    setSessionsOpen(false);
    setSchedulerOpen(false);
    setSchedulerEditor(null);
    setSettingsRoute(false);
    setTasksOpen(true);
    setTasksSelectedId(null);
    setSessionTasksHash(sid);
  }, [sessionId]);

  const closeTasksDrawer = useCallback(() => {
    setTasksOpen(false);
    setTasksSelectedId(null);
    if (sessionsOpen) {
      setHistoryHash();
      return;
    }
    const sid = sessionId.trim();
    if (sid) {
      setSessionHashInLocation(sid);
    } else if (window.location.hash) {
      history.replaceState(
        null,
        "",
        `${window.location.pathname}${window.location.search}`,
      );
    }
  }, [sessionId, sessionsOpen]);

  const openBackgroundTask = useCallback(
    (taskId: string) => {
      const sid = sessionId.trim();
      if (!sid) {
        return;
      }
      setTasksOpen(true);
      setTasksSelectedId(taskId);
      setSessionTasksHash(sid, taskId);
    },
    [sessionId],
  );

  const backToBackgroundTaskList = useCallback(() => {
    const sid = sessionId.trim();
    setTasksSelectedId(null);
    if (sid) {
      setSessionTasksHash(sid);
    }
  }, [sessionId]);

  /** Opens a session in this tab: the child transcript behind an agent task,
   *  or the parent chat from a read-only notice. Same path as a History pick,
   *  so the panel closes and the hash becomes `#/s/<id>`. */
  const openSessionInPlace = (targetId: string) => {
    const id = targetId.trim();
    if (id) {
      pickSession(id);
    }
  };

  useEffect(() => {
    const env = getEnv();
    // Inside a node the relay's own routes are no longer under the base URL, so
    // asking again would say "not a swarm" and take away the way back. What we
    // came through is remembered instead.
    if (env.mode === "remote" && env.swarmRelay) {
      setIsSwarmEnv(true);
      setAtSwarmRoot(false);
      return undefined;
    }
    const ac = new AbortController();
    void probeSwarm(ac.signal).then((info) => {
      setIsSwarmEnv(!!info);
      setAtSwarmRoot(!!info);
    });
    return () => ac.abort();
  }, []);

  // Entering a node points the whole app at that node's mount, so every screen
  // that already existed works against it with a relay in the middle.
  const openSwarmNode = useCallback((nodePath: string[], hash?: string) => {
    const env = getEnv();
    // Served by the relay from its own root, the environment is plain
    // same-origin: the relay is then this page's origin. Without that fallback
    // the one entry point this screen exists for silently did nothing.
    const relay =
      env.mode === "remote"
        ? (env.swarmRelay ?? env.baseUrl)
        : window.location.origin;
    if (!relay) {
      return;
    }
    connectSwarmNode(
      relay,
      nodePath,
      env.mode === "remote" ? env.token : "",
      hash,
    );
  }, []);

  /**
   * Where the app has been in this swarm, as a route.
   *
   * Inside a node it is that node; back on the relay it is whatever
   * `returnToSwarm` remembered. The map marks it and draws the path to it, so
   * the screen can say where we are rather than only what exists.
   */
  const swarmCurrentNode = useMemo(() => {
    const env = getEnv();
    if (env.mode !== "remote") {
      return [] as string[];
    }
    const route = env.swarmNode || env.swarmFrom || "";
    return route.split("/").filter(Boolean);
  }, []);

  const openSwarmFromNav = useCallback(() => {
    const env = getEnv();
    // Inside a node, going to the swarm means going back out to its relay.
    if (env.mode === "remote" && env.swarmRelay) {
      returnToSwarm();
      return;
    }
    setSchedulerOpen(false);
    setSchedulerEditor(null);
    setTasksOpen(false);
    setTasksSelectedId(null);
    setSessionsOpen(false);
    setSettingsRoute(false);
    window.location.hash = appNavHrefSwarm();
  }, []);

  const openSettingsFromNav = useCallback(() => {
    setSchedulerOpen(false);
    setSchedulerEditor(null);
    setTasksOpen(false);
    setTasksSelectedId(null);
    setSessionsOpen(false);
    setSettingsHash();
  }, []);

  const onCloseSettings = useCallback(() => {
    const sid = sessionId.trim();
    if (sid) {
      setSessionHashInLocation(sid);
    } else {
      clearSessionRoute();
    }
  }, [sessionId, clearSessionRoute]);

  // After the onboarding form is dismissed (saved or skipped), play the guided
  // tour once — desktop shell only, and only if it has not been seen before.
  const maybeStartTour = useCallback(() => {
    if (isDesktopShell() && !isTourSeen()) {
      setShowTour(true);
    }
  }, []);

  // "Restart onboarding" in Settings: close settings, clear the tour-seen flag
  // so it can replay, and reopen the provider form.
  const restartOnboarding = useCallback(() => {
    onCloseSettings();
    resetTour();
    setShowTour(false);
    setShowProviderPicker(true);
  }, [onCloseSettings]);

  const onOpenHistoryFromNav = useCallback(() => {
    setSchedulerOpen(false);
    setSchedulerEditor(null);
    setTasksOpen(false);
    setTasksSelectedId(null);

    setSettingsRoute(false);
    setSessionsOpen(true);
    setHistoryHash();
  }, []);

  /** Background tasks indexed by the tool call that started them, so a
   *  transcript row can keep ticking after the tool itself returned. */
  const backgroundTasksByToolCallId = useMemo(() => {
    const byToolCall = new Map<string, BackgroundTask>();
    for (const t of backgroundTasks) {
      const tc = (t.tool_call_id || "").trim();
      if (tc) {
        byToolCall.set(tc, t);
      }
    }
    return byToolCall;
  }, [backgroundTasks]);

  const miniAppSessionEligible = useMemo(() => {
    return isMiniAppSessionEligible({
      miniAppsHttpLinked,
      editorEmbed: isEditorEmbed(),
      sessionId,
      generating,
      items,
    });
  }, [generating, items, miniAppsHttpLinked, sessionId]);

  // The panel belongs to a chat, so it only exists when one is open.
  const tasksPanelOpen = tasksOpen && !!sessionId.trim();

  const shellBackdropOpen =
    sessionsOpen ||
    (schedulerOpen && schedulerHttpLinked === true) ||
    settingsRoute ||
    swarmRoute;

  const filteredSchedulerJobs = useMemo(() => {
    const q = schedulerFilterQ.trim().toLowerCase();
    if (!q) {
      return schedulerJobs;
    }
    return schedulerJobs.filter((j) => {
      const id = (j.job_id || "").toLowerCase();
      const desc = (j.description || "").toLowerCase();
      return id.includes(q) || desc.includes(q);
    });
  }, [schedulerJobs, schedulerFilterQ]);

  const sessionPanelShared = {
    sessionId: sidebarActiveId,
    permissionPendingSessionIds: permissionPendingSids,
    questionPendingSessionIds: questionPendingSids,
    sessions: sessionsForSidebar,
    ...(sessionsError ? { error: sessionsError } : {}),
    open: sessionsOpen,
    onClose: () => {
      setSessionsOpen(false);
      const p = parseAppHash();
      if (p.branch === "history") {
        const sid = sessionId.trim();
        const did = activeDraftId.trim();
        if (sid) {
          setSessionHashInLocation(sid);
        } else if (did) {
          setDraftHashInLocation(did);
        } else if (window.location.hash) {
          history.replaceState(
            null,
            "",
            `${window.location.pathname}${window.location.search}`,
          );
        }
        return;
      }
      stripHistorySidebarFromHash();
    },
    onPick: pickSession,
    onTitleSave: saveSessionTitle as (id: string, title: string) => void,
    onDelete: deleteSession as (id: string) => void | Promise<void>,
    searchDraft: sessionFilterDraft,
    onSearchDraftChange: setSessionFilterDraft,
    onSearchClear: () => setSessionFilterDraft(""),
    projectRoot: hostProjectRoot,
    projectOnly: sessionsProjectOnly,
    onProjectOnlyChange: changeSessionsProjectOnly,
    hasMore: sessionsHasMore,
    loadingMore: sessionsLoadingMore,
    onLoadMore: () => void loadSessionsList(false),
  };

  const toggleRailWidth = () => {
    setRailLabelsWide((prev) => {
      const next = !prev;
      writeNavRailCookie(next ? "wide" : "narrow");
      return next;
    });
  };

  // Identity-stable handlers for the React.memo message rows: a shell
  // re-render (every streamed token) must not invalidate their props.
  const handleEditUserMessage = useStableHandler(
    (content: string, userMsgIdx: number) => {
      const assetNote = extractSessionAssetsXml(content);
      setDraft(stripFoxxyCodeAttachmentsForUserDisplay(content));
      setEditingUserMsgIdx(userMsgIdx);
      setEditingAssetNote(assetNote);
      setEditingFiles(parseSessionAssetFiles(content));
    },
  );
  const handleStopBackgroundTask = useStableHandler((id: string) => {
    void stopBackgroundTaskById(id);
  });
  const handleFetchToolCallFull = useStableHandler(
    async (toolCallId: string) => {
      if (!sessionId) return;
      const det = await fetchJSON<{
        args?: string;
        result?: string;
        meta?: {
          status?: string;
          kind?: string;
          name?: string;
          planSnapshot?: unknown;
        };
      }>(
        `/foxxycode/sessions/${encodeURIComponent(sessionId)}/tool-calls/${encodeURIComponent(toolCallId)}`,
        { headers },
      );
      if (!det.ok || !det.data) return;
      const meta = det.data.meta || {};
      const patch: Record<string, unknown> = { toolCallId };
      if (meta.name) patch.title = meta.name;
      if (meta.kind) patch.kind = meta.kind;
      if (meta.status) patch.status = meta.status;
      const todoPlan = normalizeTodoPlanSnapshot(meta.planSnapshot);
      if (todoPlan !== undefined) patch.todoPlan = todoPlan;
      if (det.data.args) patch.argsText = det.data.args;
      if (det.data.result !== undefined) patch.fullResultText = det.data.result;
      upsertToolCall(patch as any);
    },
  );

  return (
    <div
      className={[
        "shell",
        viewportXL && railLabelsWide ? "shell-rail-wide" : "",
      ]
        .filter(Boolean)
        .join(" ")}
    >
      <EnvHealthBanner />
      <NavRail
        onNewChat={goHome}
        onOpenHistory={onOpenHistoryFromNav}
        historyOpen={sessionsOpen}
        showHistory={!atSwarmRoot}
        showScheduler={schedulerHttpLinked === true && !atSwarmRoot}
        showMiniApps={
          miniAppsHttpLinked === true && !isEditorEmbed() && !atSwarmRoot
        }
        onOpenMiniApps={openMiniAppsFromNav}
        miniAppsOpen={miniAppsOpen}
        onOpenScheduler={openSchedulerFromNav}
        schedulerOpen={schedulerOpen}
        showSwarm={isSwarmEnv}
        onOpenSwarm={openSwarmFromNav}
        swarmOpen={swarmRoute}
        settingsOpen={settingsRoute}
        onOpenSettings={openSettingsFromNav}
        canWidenRail={viewportXL}
        railLabelsWide={railLabelsWide}
        onToggleRailLabels={toggleRailWidth}
      />

      <div
        className={[
          "shell-main",
          sessionsOpen ? "shell-history-open" : "",
          tasksPanelOpen ? "shell-tasks-open" : "",
        ]
          .filter(Boolean)
          .join(" ")}
        style={
          schedDockClusterWidthPx > 0
            ? ({
                "--sched-dock-cluster-width": `${schedDockClusterWidthPx}px`,
              } as CSSProperties)
            : undefined
        }
      >
        <div
          className={`backdrop ${shellBackdropOpen ? "is-open" : ""}`}
          onClick={() => {
            if (shellBackdropOpen) {
              closeAllShellDrawers();
            }
          }}
          aria-hidden={!shellBackdropOpen}
        />

        {sessionsOpen ? <SessionsSidebar {...sessionPanelShared} /> : null}

        {schedulerOpen && schedulerHttpLinked === true ? (
          <div
            ref={schedulerDockClusterRef}
            className={[
              "scheduler-dock-cluster",
              schedulerEditor ? "scheduler-dock-cluster-editor-active" : "",
            ]
              .filter(Boolean)
              .join(" ")}
          >
            <SchedulerJobsDrawer
              open={schedulerOpen}
              selectedJobId={
                schedulerEditor?.mode === "edit" ? schedulerEditor.jobId : null
              }
              className="scheduler-dock-drawer"
              onClose={closeSchedulerDrawer}
              scheduler={schedulerInfo}
              jobs={filteredSchedulerJobs}
              listError={schedulerListError}
              loading={schedulerListLoading}
              onAddJob={() => {
                setSchedulerCreateHash();
              }}
              onOpenJob={(jid) => {
                setSchedulerEditor({ mode: "edit", jobId: jid });
                setSchedulerJobHash(jid);
              }}
              onRunJob={(jid) => void onSchedulerRunJob(jid)}
              onCancelJob={(jid) => void onSchedulerCancelJob(jid)}
              searchDraft={schedulerFilterDraft}
              onSearchDraftChange={setSchedulerFilterDraft}
              onSearchClear={() => setSchedulerFilterDraft("")}
            />

            <SchedulerJobEditorSheet
              open={schedulerHttpLinked === true && !!schedulerEditor}
              mode={schedulerEditor?.mode === "create" ? "create" : "edit"}
              jobId={
                schedulerEditor?.mode === "edit" ? schedulerEditor.jobId : null
              }
              availableModels={llmModelIds}
              defaultModel={llmModel}
              currentCwd={currentSessionCwd}
              onClose={() => {
                setSchedulerEditor(null);
                setSchedulerListHash();
              }}
              onSaved={(createdId) => {
                void refreshSchedulerJobs({ silent: true });
                if (createdId) {
                  setSchedulerEditor({ mode: "edit", jobId: createdId });
                }
              }}
              onDeleted={() => {
                setSchedulerEditor(null);
                void refreshSchedulerJobs({ silent: true });
              }}
            />
          </div>
        ) : null}

        {swarmRoute || (atSwarmRoot && !settingsRoute) ? (
          <div className="swarm-dock-cluster">
            <SwarmView
              onOpenNode={(nodePath: string[]) => openSwarmNode(nodePath)}
              onOpenSession={(s) => openSwarmNode(s.node_path, `#/s/${s.id}`)}
              {...(swarmCurrentNode.length > 0
                ? { currentNode: swarmCurrentNode }
                : {})}
              {...(atSwarmRoot ? { headerSlot: <EnvironmentChip /> } : {})}
            />
          </div>
        ) : null}
        {settingsRoute ? (
          <div className="settings-dock-cluster">
            <Settings
              onClose={onCloseSettings}
              onConfigSaved={() => setConfigEpoch((e) => e + 1)}
              initialSection={settingsSection}
              onRestartOnboarding={restartOnboarding}
              // Subagent approvals are keyed by workspace, and spawn_agent
              // checks the session's own cwd: the viewed session's workspace
              // is the one the tab must ask about. Falls back to the host
              // project root, which is the cwd an editor plugin launched the
              // server for.
              workspacePath={workspaceCtx?.path || hostProjectRoot || undefined}
            />
          </div>
        ) : null}
        {tasksPanelOpen ? (
          <BackgroundTasksPanel
            open
            selectedTaskId={tasksSelectedId}
            tasks={backgroundTasks}
            selectedOutput={backgroundOutput}
            listError={backgroundListError}
            loading={backgroundListLoading}
            nowMs={backgroundNowMs}
            onClose={closeTasksDrawer}
            onOpenTask={openBackgroundTask}
            onOpenSession={openSessionInPlace}
            onBackToList={backToBackgroundTaskList}
            onStopTask={(id) => {
              void stopBackgroundTaskById(id);
            }}
            onClearFinished={() => {
              void clearFinishedTasks();
            }}
            onRefresh={() => {
              void refreshBackgroundTasks({ silent: true });
            }}
          />
        ) : null}
        {miniAppsOpen ? (
          <MiniAppsPage
            selectedAppId={miniAppsAppId}
            sessionId={sessionId}
            sessionEligible={miniAppSessionEligible}
            onNavigate={(appId) => {
              setMiniAppsAppId(appId || null);
              setMiniAppsHash(appId);
            }}
            onClose={closeMiniApps}
          />
        ) : atSwarmRoot ? null : (
          <ChatScreen
            title={currentTitle}
            sessionId={sessionId}
            backgroundTasks={backgroundTasks}
            onOpenBackgroundTasks={openTasksFromNav}
            backgroundTasksByToolCallId={backgroundTasksByToolCallId}
            backgroundNowMs={backgroundNowMs}
            onOpenBackgroundTask={openBackgroundTask}
            onStopBackgroundTask={handleStopBackgroundTask}
            subagentTranscript={subagentTranscript}
            onOpenSession={openSessionInPlace}
            workspaceCtx={workspaceCtx}
            worktreePref={worktreePref}
            svnFolderPref={svnFolderPref}
            workspaceLocked={items.length > 0}
            onWorkspacePickFolder={(p: string) => void switchWorkspace({ path: p })}
            onWorkspacePickBranch={(b: string, wt: boolean) =>
              void switchWorkspace({ branch: b, worktree: wt })
            }
            onWorktreeToggle={() => setWorktreePref((v) => !v)}
            onWorkspacePickSvnBranch={(b: string, folder: boolean) =>
              void switchWorkspace({ branch: b, worktree: folder, vcs: "svn" })
            }
            onSvnFolderToggle={() => setSvnFolderPref((v) => !v)}
            sessionLoading={sessionLoading}
            sessionFadingOut={sessionFadingOut}
            heroAccentVerb={heroAccentVerb}
            heroComposerFocusEpoch={heroHomeGeneration}
            onTitleSave={(t: string) => void saveSessionTitle(sessionId, t)}
            {...(miniAppSessionEligible
              ? {
                  headerActions: (
                    <button
                      type="button"
                      className="chat-header-action"
                      title={t("miniapps.create")}
                      aria-label={t("miniapps.create")}
                      data-testid="chat-create-miniapp"
                      onClick={createMiniAppFromSession}
                    >
                      <span aria-hidden="true">+</span>
                    </button>
                  ),
                }
              : {})}
            onExportSession={(f: ExportFormat) => void exportSession(f)}
            exportBusy={exportBusy}
            items={items}
            draft={draft}
            tokenUsage={tokenUsage}
            providerUsage={providerUsageState.usage}
            usageBannerDismissedKey={providerUsageState.dismissedKey}
            onUsageBannerDismiss={providerUsageState.dismissBanner}
            contextPct={contextPct}
            maxContextTokens={maxContextTokens}
            contextBreakdown={contextBreakdown}
            mode={mode}
            modes={[...PROFILE_MODES]}
            {...(llmModelIds.length > 0
              ? {
                  llmModels: llmModelIds,
                  llmModel,
                  onLlmModelChange,
                  llmModelMultimodal,
                  ...(llmReasoningLevels.length > 0
                    ? {
                        llmReasoningLevels,
                        llmReasoning,
                        onLlmReasoningChange,
                      }
                    : {}),
                }
              : {})}
            onModeChange={onModeChange}
            onDraftChange={setDraft}
            generating={generating}
            onContextRingOpen={() => {
              const sid = sessionId.trim();
              if (sid) {
                void refreshSessionStats(sid);
              }
            }}
            onStop={() => stopActiveGeneration()}
            onQuestionPromptResolved={resolveQuestionPrompt}
            onPermissionPromptResolved={resolvePermissionPrompt}
            onPlanDocumentExpanded={(itemId, expanded) => {
              setItems((prev) =>
                prev.map((x) =>
                  x.id === itemId && x.type === "plan_document"
                    ? { ...x, expanded }
                    : x,
                ),
              );
            }}
            // A subagent transcript is read-only: like onEdit below, Run plan and
            // Discard are withheld rather than stubbed, so the plan card renders
            // without its footer and its editor is read-only.
            {...(subagentTranscript
              ? {}
              : {
                  onPlanDocumentRun: (slug: string) => {
                    if (
                      sessionId.trim() &&
                      activeComposerSidRef.current.has(sessionId.trim())
                    ) {
                      return;
                    }
                    void streamResponses(t("chat.runPlanMessage"), {
                      modeOverride: "agent",
                      runPlanSlug: slug,
                    });
                  },
                  onPlanDocumentDiscard: async (itemId: string, slug: string) => {
                    const sid = sessionId.trim();
                    if (!sid) return;
                    try {
                      await fetch(
                        `/foxxycode/sessions/${encodeURIComponent(sid)}/plans/${encodeURIComponent(slug)}`,
                        {
                          method: "DELETE",
                          headers,
                        },
                      );
                    } catch {
                      return;
                    }
                    setItems((prev) =>
                      prev.map((x) =>
                        x.id === itemId && x.type === "plan_document"
                          ? { ...x, discarded: true }
                          : x,
                      ),
                    );
                  },
                })}
            {...(subagentTranscript ? {} : { onEdit: handleEditUserMessage })}
            {...(editingFiles.length > 0 ? { editingFiles } : {})}
            onBranchSwitch={(sid) => switchBranch(sid)}
            {...(knownSkillNames.size > 0 ? { knownSkillNames } : {})}
            onPasteChipCaptured={(key, literal) => {
              const m = pasteLiteralsRef.current;
              m.set(key, literal);
              // Bound the map: drop oldest entries past 32 (Map keeps insertion order).
              while (m.size > 32) {
                const oldest = m.keys().next().value;
                if (oldest == null) {
                  break;
                }
                m.delete(oldest);
              }
            }}
            onSend={(text: string, files?: File[]) => {
              // A subagent transcript is read-only: the server answers 409.
              if (subagentTranscript) {
                return;
              }
              if (
                sessionId.trim() &&
                activeComposerSidRef.current.has(sessionId.trim())
              ) {
                return;
              }
              setDraft("");
              if (editingUserMsgIdx !== null) {
                const idx = editingUserMsgIdx;
                const note = editingAssetNote;
                setEditingUserMsgIdx(null);
                setEditingAssetNote("");
                setEditingFiles([]);
                const textWithAssets = note ? `${text}\n${note}` : text;
                void handleBranchSend(textWithAssets, idx);
              } else {
                void streamResponses(text, files ? { files } : undefined);
              }
            }}
            onFetchToolCallFull={handleFetchToolCallFull}
          />
        )}
        <ProviderPickerDialog
          open={showProviderPicker}
          onSaved={() => {
            setShowProviderPicker(false);
            setConfigEpoch((e) => e + 1);
            maybeStartTour();
          }}
          onSkip={() => {
            setShowProviderPicker(false);
            maybeStartTour();
          }}
        />
        <GuidedTour
          open={showTour}
          steps={TOUR_STEPS}
          onClose={() => {
            markTourSeen();
            setShowTour(false);
          }}
        />
        <DesktopNotifications
          notifications={desktopNotifications}
          onDismiss={(id) =>
            setDesktopNotifications((prev) => prev.filter((n) => n.id !== id))
          }
          onPermissionChoose={(n, optionId, label) => {
            void (async () => {
              try {
                await submitPermissionChoice(
                  n.sessionId,
                  n.toolCallId,
                  optionId,
                );
              } catch {
                // still resolve locally on transient network errors
              }
              resolvePermissionPrompt(n.sessionId, n.itemId, {
                optionId,
                summaryLine: label,
              });
            })();
          }}
          onRunPlan={(n) => {
            setDesktopNotifications((prev) =>
              prev.filter((x) => x.id !== n.id),
            );
            const sid = n.sessionId.trim();
            if (
              !sid ||
              sid !== sessionId.trim() ||
              activeComposerSidRef.current.has(sid)
            ) {
              return;
            }
            void streamResponses(t("chat.runPlanMessage"), {
              modeOverride: "agent",
              runPlanSlug: n.slug,
            });
          }}
        />
      </div>
    </div>
  );
}
