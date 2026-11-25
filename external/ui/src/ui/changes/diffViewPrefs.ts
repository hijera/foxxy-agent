/**
 * Which way the edits window draws a diff, remembered per browser.
 *
 * Presentation only, so it follows the house style for that kind of preference
 * (theme, nav rail, reasoning level) and lives in a cookie rather than the
 * config file: it says nothing about the session and nothing the agent needs.
 */

export type DiffView = "unified" | "split";

export const DEFAULT_DIFF_VIEW: DiffView = "unified";

const COOKIE = "foxxycode_diff_view";
const START_EXPANDED_COOKIE = "foxxycode_edits_start_expanded";
const MAX_AGE_SECONDS = 365 * 24 * 60 * 60;

function isDiffView(value: string): value is DiffView {
  return value === "unified" || value === "split";
}

export function readDiffViewCookie(): DiffView {
  if (typeof document === "undefined") {
    return DEFAULT_DIFF_VIEW;
  }
  for (const part of document.cookie.split(";")) {
    const s = part.trim();
    if (!s.startsWith(`${COOKIE}=`)) {
      continue;
    }
    const value = decodeURIComponent(s.slice(COOKIE.length + 1).trim()).trim();
    if (isDiffView(value)) {
      return value;
    }
  }
  return DEFAULT_DIFF_VIEW;
}

export function writeDiffViewCookie(view: DiffView): void {
  writePreference(COOKIE, view);
}

export function readEditsStartExpanded(): boolean {
  if (typeof window === "undefined") return false;
  try {
    const stored = window.localStorage.getItem(START_EXPANDED_COOKIE);
    if (stored === "0" || stored === "1") return stored === "1";
  } catch {
    // An embedded browser can restrict storage.
  }
  try {
    return document.cookie.split(";").some(
      (part) => part.trim() === START_EXPANDED_COOKIE + "=1",
    );
  } catch {
    return false;
  }
}

export function writeEditsStartExpanded(expanded: boolean): void {
  const value = expanded ? "1" : "0";
  try {
    window.localStorage.setItem(START_EXPANDED_COOKIE, value);
  } catch {
    // Fall back to the cookie on this origin.
  }
  try {
    writePreference(START_EXPANDED_COOKIE, value);
  } catch {
    // The settings switch still reflects the choice for this render.
  }
}

function writePreference(name: string, value: string): void {
  if (typeof document === "undefined") {
    return;
  }
  const secure =
    typeof window !== "undefined" && window.location.protocol === "https:"
      ? "; Secure"
      : "";
  document.cookie = `${name}=${encodeURIComponent(value)}; Path=/; Max-Age=${MAX_AGE_SECONDS}; SameSite=Lax${secure}`;
}
