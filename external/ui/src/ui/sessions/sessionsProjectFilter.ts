/**
 * Scoping the History list to the open project.
 *
 * Sessions live in one shared home (`~/.foxxycode/sessions`), so an editor plugin
 * that runs one server per project would otherwise list every workspace the user
 * ever opened. The SPA sends the host project root as **`cwd`** on
 * **`GET /foxxycode/sessions`** (see `session.CWDInScope` in
 * `internal/session/cwd.go`), and the History drawer exposes a toggle to drop the
 * scope and browse everything.
 *
 * The preference is per host root, so switching projects does not inherit a
 * "show everything" choice made somewhere else.
 */

import { readSessionPref, writeSessionPref, type SessionPrefCookie } from "./sessionPrefs";

const STORAGE_PREFIX = "foxxycode.sessions.projectOnly:";

function cookieKey(projectRoot: string): SessionPrefCookie {
  return `foxxycode_sessions_project_only_${encodeURIComponent(projectRoot.trim())}`;
}

const isScopeValue = (value: string): value is "0" | "1" => value === "0" || value === "1";

/**
 * The **`cwd`** query value for a session-list request, or `null` when the list
 * must not be scoped (toggle off, or no host project root known).
 */
export function sessionsProjectCwdParam(input: {
  projectOnly: boolean;
  projectRoot?: string | null;
}): string | null {
  if (!input.projectOnly) {
    return null;
  }
  const root = (input.projectRoot || "").trim();
  return root === "" ? null : root;
}

function storageKey(projectRoot: string): string {
  return `${STORAGE_PREFIX}${projectRoot.trim()}`;
}

/**
 * Stored preference for a host root, falling back to `fallback` (scoped inside an
 * editor embed, unscoped in the browser) when nothing was stored yet.
 */
export function readProjectOnlyPref(
  projectRoot: string,
  fallback: boolean,
): boolean {
  const root = (projectRoot || "").trim();
  if (root === "" || typeof window === "undefined") {
    return fallback;
  }
  // The IDE picks a new loopback port on every start. Host cookies cross that
  // port boundary; localStorage alone cannot restore the previous choice.
  try {
    const saved = readSessionPref(cookieKey(root), isScopeValue);
    if (saved !== null) return saved === "1";
  } catch {
    // Keep the legacy fallback when cookies are unavailable.
  }
  try {
    const raw = window.localStorage.getItem(storageKey(root));
    if (raw === "1" || raw === "0") {
      writeSessionPref(cookieKey(root), raw);
      return raw === "1";
    }
  } catch {
    // Private mode / disabled storage: fall back to the caller's default.
  }
  return fallback;
}

export function writeProjectOnlyPref(
  projectRoot: string,
  projectOnly: boolean,
): void {
  const root = (projectRoot || "").trim();
  if (root === "" || typeof window === "undefined") {
    return;
  }
  try {
    writeSessionPref(cookieKey(root), projectOnly ? "1" : "0");
  } catch {
    // A blocked cookie must not disable the in-memory choice or legacy store.
  }
  try {
    window.localStorage.setItem(storageKey(root), projectOnly ? "1" : "0");
  } catch {
    // Best-effort: the toggle still works for this session.
  }
}

/** Last path segment of a folder path, for labelling the toggle. */
export function projectRootLabel(projectRoot: string): string {
  const cleaned = (projectRoot || "").trim().replace(/[\\/]+$/, "");
  if (cleaned === "") {
    return "";
  }
  const parts = cleaned.split(/[\\/]/);
  return parts[parts.length - 1] || cleaned;
}
