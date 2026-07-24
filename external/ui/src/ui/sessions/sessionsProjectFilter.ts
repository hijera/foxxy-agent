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

const STORAGE_PREFIX = "foxxycode.sessions.projectOnly:";

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
  try {
    const raw = window.localStorage.getItem(storageKey(root));
    if (raw === "1") return true;
    if (raw === "0") return false;
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
