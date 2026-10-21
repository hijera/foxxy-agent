import type {
  FoxxyCodePermissionPayload,
  PermissionResolvedState,
} from "./permissionTypes";
import { restorePermissionPromptsForPendingTools } from "./restorePermissionPrompts";
import { stablePermissionPromptItemId } from "./transcriptItemIds";
import type { TranscriptItem } from "./types";
import type { ToolsPermissionPolicy } from "./toolsPermissionPolicy";

const STORAGE_PREFIX = "foxxycode_pp_v1:";

export type StoredPermissionPromptRecord = {
  toolCallId: string;
  payload: FoxxyCodePermissionPayload;
  resolved?: PermissionResolvedState | undefined;
};

function storageKey(sessionId: string): string {
  return `${STORAGE_PREFIX}${sessionId.trim()}`;
}

export function loadPermissionPromptRecords(
  sessionId: string,
): StoredPermissionPromptRecord[] {
  if (typeof window === "undefined") return [];
  const sid = sessionId.trim();
  if (!sid) return [];
  try {
    const raw = window.localStorage.getItem(storageKey(sid));
    if (!raw) return [];
    const v = JSON.parse(raw) as unknown;
    if (!Array.isArray(v)) return [];
    const out: StoredPermissionPromptRecord[] = [];
    for (const row of v) {
      if (!row || typeof row !== "object") continue;
      const o = row as Record<string, unknown>;
      const toolCallId = String(o.toolCallId ?? "").trim();
      const payload = o.payload as FoxxyCodePermissionPayload | undefined;
      if (!toolCallId || !payload?.toolCall?.toolCallId) continue;
      out.push({
        toolCallId,
        payload,
        ...(o.resolved !== undefined
          ? { resolved: o.resolved as PermissionResolvedState }
          : {}),
      });
    }
    return out;
  } catch {
    return [];
  }
}

export function upsertPermissionPromptRecord(
  sessionId: string,
  record: StoredPermissionPromptRecord,
): void {
  if (typeof window === "undefined") return;
  const sid = sessionId.trim();
  const tcid = record.toolCallId.trim();
  if (!sid || !tcid) return;
  const list = loadPermissionPromptRecords(sid);
  const i = list.findIndex((r) => r.toolCallId.trim() === tcid);
  const next: StoredPermissionPromptRecord = {
    toolCallId: tcid,
    payload: record.payload,
    ...(record.resolved !== undefined ? { resolved: record.resolved } : {}),
  };
  if (i >= 0) {
    list[i] = {
      ...list[i],
      ...next,
      resolved:
        record.resolved !== undefined ? record.resolved : list[i]?.resolved,
    };
  } else {
    list.push(next);
  }
  try {
    window.localStorage.setItem(storageKey(sid), JSON.stringify(list));
  } catch {
    //
  }
}

export function removePermissionPromptRecord(
  sessionId: string,
  toolCallId: string,
): void {
  if (typeof window === "undefined") return;
  const sid = sessionId.trim();
  const tcid = toolCallId.trim();
  if (!sid || !tcid) return;
  const list = loadPermissionPromptRecords(sid).filter(
    (r) => r.toolCallId.trim() !== tcid,
  );
  try {
    if (list.length === 0) {
      window.localStorage.removeItem(storageKey(sid));
    } else {
      window.localStorage.setItem(storageKey(sid), JSON.stringify(list));
    }
  } catch {
    //
  }
}

/** clearPermissionPromptRecords drops every prompt stored for the session:
 *  a history rewind made the tool calls they point at leave the transcript. */
export function clearPermissionPromptRecords(sessionId: string): void {
  if (typeof window === "undefined") return;
  const sid = sessionId.trim();
  if (!sid) return;
  try {
    window.localStorage.removeItem(storageKey(sid));
  } catch {
    //
  }
}

export function mergeStoredPermissionPromptsIntoTranscript(
  merged: TranscriptItem[],
  sessionId: string,
): TranscriptItem[] {
  const records = loadPermissionPromptRecords(sessionId);
  if (records.length === 0) return merged;

  let out = [...merged];
  const existing = new Set(
    out
      .filter(
        (x): x is Extract<TranscriptItem, { type: "permission_prompt" }> =>
          x.type === "permission_prompt",
      )
      .map((x) => x.payload.toolCall.toolCallId.trim())
      .filter(Boolean),
  );

  for (const rec of records) {
    const tcid = rec.toolCallId.trim();
    if (!tcid || existing.has(tcid) || rec.resolved) continue;
    const idx = out.findIndex(
      (x) => x.type === "tool_call" && x.toolCallId === tcid,
    );
    if (idx < 0) continue;
    out.splice(idx + 1, 0, {
      id: stablePermissionPromptItemId(tcid),
      type: "permission_prompt",
      payload: rec.payload,
    });
    existing.add(tcid);
  }
  return out;
}

export function resolvedPermissionToolCallIds(
  sessionId: string,
): ReadonlySet<string> {
  const records = loadPermissionPromptRecords(sessionId);
  const out = new Set<string>();
  for (const rec of records) {
    if (rec.resolved) out.add(rec.toolCallId.trim());
  }
  return out;
}

/** Stored SSE rows, then pending tool_call synthesis (reload / server restart). */
/** Session ids with unresolved rows in localStorage (sidebar ? after reload). */
export function permissionPendingSessionIdsFromStorage(): ReadonlySet<string> {
  if (typeof window === "undefined") return new Set();
  const out = new Set<string>();
  const prefix = STORAGE_PREFIX;
  for (let i = 0; i < window.localStorage.length; i++) {
    const key = window.localStorage.key(i);
    if (!key?.startsWith(prefix)) continue;
    const sid = key.slice(prefix.length).trim();
    if (!sid) continue;
    const rows = loadPermissionPromptRecords(sid);
    if (rows.some((r) => !r.resolved)) {
      out.add(sid);
    }
  }
  return out;
}

export function mergePermissionPromptsIntoTranscript(
  merged: TranscriptItem[],
  sessionId: string,
  policy: ToolsPermissionPolicy | null,
): TranscriptItem[] {
  // A terminal tool outcome is authoritative even when Stop released the
  // stream before its final update arrived. Retire both the visible card and
  // its stored copy so a reload cannot revive an obsolete approval request.
  const finished = new Set(
    merged.flatMap((item) =>
      item.type === "tool_call" &&
      ["completed", "failed", "cancelled"].includes(item.status)
        ? [item.toolCallId.trim()]
        : [],
    ),
  );
  const records = loadPermissionPromptRecords(sessionId);
  for (const item of merged) {
    if (
      item.type === "permission_prompt" &&
      !item.resolved &&
      finished.has(item.payload.toolCall.toolCallId.trim())
    ) {
      records.push({
        toolCallId: item.payload.toolCall.toolCallId,
        payload: item.payload,
      });
    }
  }
  for (const record of records) {
    if (!record.resolved && finished.has(record.toolCallId.trim())) {
      upsertPermissionPromptRecord(sessionId, {
        ...record,
        resolved: { optionId: "cancelled", summaryLine: "" },
      });
    }
  }
  const current = merged.filter(
    (item) =>
      item.type !== "permission_prompt" ||
      !finished.has(item.payload.toolCall.toolCallId.trim()),
  );
  const withStored = mergeStoredPermissionPromptsIntoTranscript(
    current,
    sessionId,
  );
  return restorePermissionPromptsForPendingTools(withStored, sessionId, policy);
}
