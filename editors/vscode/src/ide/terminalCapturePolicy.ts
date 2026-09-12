/** Pure decision logic behind the clipboard-based terminal screen capture.
 *  Kept free of any `vscode` import so it can be unit-tested in plain Node
 *  (mirrors `ide/terminalStatePayload.ts`).
 *
 *  Background: VS Code has no stable API to read an existing terminal's
 *  buffer. Shell integration (>= 1.93) streams the output of commands that
 *  start after the extension is running; everything else — scrollback from
 *  before activation, terminals without shell integration — is reachable only
 *  the way cline does it: select all → copy → read the clipboard → restore it.
 *  This module decides *when* that is worth doing and how the result folds
 *  into the reported buffer; the vscode-bound half lives in
 *  `terminalStateService.ts`. */

import { stripAnsi } from "./terminalStatePayload";

/** Minimum gap between two captures of the same terminal. */
export const CAPTURE_MIN_INTERVAL_MS = 2000;

/** While shell integration delivered output this recently, it is the better
 *  source and the clipboard round-trip is skipped. */
export const FRESH_EXECUTION_MS = 5000;

/** Written to the clipboard before `copySelection` so a copy that did nothing
 *  (empty buffer, a terminal that was never rendered) is detectable instead
 *  of being mistaken for terminal output. NULs never occur in real clipboard
 *  text. */
export const CAPTURE_SENTINEL = "\u0000foxxycode-terminal-capture\u0000";

export interface CaptureDecisionInput {
  /** `trackTerminals && terminalClipboardCapture`. */
  enabled: boolean;
  hasActiveTerminal: boolean;
  /** A capture is already in flight (re-entrancy guard). */
  capturing: boolean;
  now: number;
  /** Time of the last capture of this terminal, if any. */
  lastCaptureAt: number | null;
  /** Time of the last shell-integration output chunk for this terminal, if any. */
  lastExecutionOutputAt: number | null;
}

/** Whether to run the clipboard capture now. */
export function shouldCaptureScreen(
  input: CaptureDecisionInput,
  minIntervalMs: number = CAPTURE_MIN_INTERVAL_MS,
  freshExecutionMs: number = FRESH_EXECUTION_MS,
): boolean {
  if (!input.enabled || !input.hasActiveTerminal || input.capturing) return false;
  if (input.lastCaptureAt !== null && input.now - input.lastCaptureAt < minIntervalMs) {
    return false;
  }
  if (
    input.lastExecutionOutputAt !== null &&
    input.now - input.lastExecutionOutputAt < freshExecutionMs
  ) {
    return false;
  }
  return true;
}

/** Clipboard text read back after `copySelection` → captured screen, or
 *  `null` when the copy produced nothing (sentinel still there, or blank). */
export function screenFromClipboard(
  text: string,
  sentinel: string = CAPTURE_SENTINEL,
): string | null {
  if (text === sentinel || text.trim() === "") return null;
  return text;
}

/** Normalizes a captured screen: strips ANSI, unifies line endings, drops
 *  trailing blank lines (the terminal pads the viewport with them), and keeps
 *  only the last `maxBytes` characters. */
export function normalizeScreenCapture(raw: string, maxBytes: number): string {
  let text = stripAnsi(raw).replace(/\r\n/g, "\n").replace(/[ \t]+$/gm, "");
  text = text.replace(/\n+$/, "");
  if (text.length > maxBytes) text = text.slice(text.length - maxBytes);
  return text;
}

/** A captured screen is the whole visible buffer, so it replaces whatever
 *  shell-integration chunks were collected before it. */
export function mergeScreenCapture(
  prev: string,
  captured: string,
): { output: string; changed: boolean } {
  if (captured === prev) return { output: prev, changed: false };
  return { output: captured, changed: true };
}
