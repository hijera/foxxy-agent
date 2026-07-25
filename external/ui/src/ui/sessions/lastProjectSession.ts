/**
 * Reopening the session the user last had open in this project.
 *
 * The VS Code and IntelliJ plugins load the SPA at `/?theme=…&lang=…&embed=…`
 * with **no hash**, so the panel always landed on the hero "new chat" screen and
 * the user had to dig through History to get back to work. The plugins also bind
 * the backend to a **fresh random port** on every IDE launch, which makes
 * `localStorage`/`sessionStorage` (keyed by origin) useless across restarts —
 * the record lives server-side in `~/.foxxycode/projects.json` instead, behind
 * `GET`/`PUT` **`/foxxycode/project/last-session`**.
 *
 * Only editor embeds restore: the desktop app and a plain browser tab keep
 * opening on the hero screen.
 */

import type { ParsedAppHash } from "../scheduler/hashRoute";

const ENDPOINT = "/foxxycode/project/last-session";

/**
 * Whether the startup route should be replaced with the project's last session.
 * Any explicit route — a session/draft deep link, History, Settings, Scheduler —
 * wins over the restore, so middle-click and shared links keep working.
 */
export function shouldRestoreLastSession(input: {
  embed: boolean;
  branch: ParsedAppHash["branch"];
}): boolean {
  return input.embed && input.branch === "none";
}

/** Session id recorded for the current project, or "" when there is none. */
export async function fetchLastProjectSession(): Promise<string> {
  try {
    const res = await fetch(ENDPOINT);
    if (!res.ok) {
      return "";
    }
    const body = (await res.json()) as { session_id?: string };
    return (body.session_id || "").trim();
  } catch {
    // Older backend or transient failure: just show the hero screen.
    return "";
  }
}

/**
 * Record (or, with an empty id, clear) the session to reopen next launch.
 * Fire-and-forget: a failure only costs the restore on the next start.
 */
export function recordLastProjectSession(sessionId: string): void {
  try {
    void fetch(ENDPOINT, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ session_id: sessionId.trim() }),
    }).catch(() => {
      // ignore
    });
  } catch {
    // ignore
  }
}
