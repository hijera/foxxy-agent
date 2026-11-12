import { envStorageSuffix } from "../env/remoteEnv";
import { readSessionPref, writeSessionPref, type SessionPrefCookie } from "./sessionPrefs";

const STORAGE_KEY = "foxxycode_sessions_collapsed_groups_v1";
const MAX_GROUPS = 500;

function storageKey(): string {
  return `${STORAGE_KEY}::${envStorageSuffix()}`;
}

function cookieKey(): SessionPrefCookie {
  return `foxxycode_sessions_collapsed_${encodeURIComponent(envStorageSuffix())}`;
}

function writeCookie(groups: readonly string[]): void {
  // Keep the most recent headings within a browser cookie's size limit. The
  // larger per-origin cache still retains up to MAX_GROUPS in the current run.
  const recent = [...groups];
  while (recent.length && encodeURIComponent(JSON.stringify(recent)).length > 3500) recent.shift();
  writeSessionPref(cookieKey(), JSON.stringify(recent));
}

export function readCollapsedSessionGroups(): ReadonlySet<string> {
  if (typeof localStorage === "undefined") {
    return new Set();
  }
  try {
    const saved = readSessionPref(cookieKey(), (value): value is string => value.startsWith("["));
    const raw = saved ?? localStorage.getItem(storageKey());
    if (!raw) {
      return new Set();
    }
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) {
      return new Set();
    }
    const groups = parsed
        .filter((key): key is string => typeof key === "string" && key !== "")
        .slice(-MAX_GROUPS);
    if (saved === null) writeCookie(groups);
    return new Set(groups);
  } catch {
    return new Set();
  }
}

export function writeCollapsedSessionGroups(groups: ReadonlySet<string>): void {
  if (typeof localStorage === "undefined") {
    return;
  }
  const recent = [...groups].filter(Boolean).slice(-MAX_GROUPS);
  try {
    writeCookie(recent);
  } catch {
    // A blocked cookie leaves the per-origin cache available.
  }
  try {
    localStorage.setItem(
      storageKey(),
      JSON.stringify(recent),
    );
  } catch {
    // The drawer remains usable when browser storage is unavailable.
  }
}

export const COLLAPSED_SESSION_GROUPS_STORAGE_KEY = STORAGE_KEY;
