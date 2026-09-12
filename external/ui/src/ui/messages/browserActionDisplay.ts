// Parsing helpers for interactive browser tool (foxxycode_browser_*) results.
//
// The backend tools return a small text block, e.g.:
//   navigated to https://example.com
//   url: https://example.com/
//   screenshot: /home/u/.foxxycode/sessions/s1/assets/browser_123.png
//   console:
//     [log] hello
// This module turns that into structured fields the BrowserAction card renders.

export interface BrowserActionInfo {
  /** First line: a short description of the action performed. */
  action: string;
  /** Resolved page URL, when present. */
  url?: string;
  /** Bare file name of the saved screenshot (no directory), when present. */
  screenshotName?: string;
  screenshotUnavailable?: string;
  /** Console/exception lines captured during the action. */
  console: string[];
  /** Unclassified result lines, with indentation preserved. */
  body: string;
}

/** basename returns the final path segment, handling both / and \ separators. */
function basename(p: string): string {
  const trimmed = p.trim();
  const idx = Math.max(trimmed.lastIndexOf("/"), trimmed.lastIndexOf("\\"));
  return idx >= 0 ? trimmed.slice(idx + 1) : trimmed;
}

/** True when the tool name identifies an interactive browser tool. */
export function isBrowserToolName(name: string | undefined): boolean {
  return (name || "").trim().toLowerCase().startsWith("foxxycode_browser_");
}

/**
 * parseBrowserActionResult extracts structured fields from a browser tool result.
 * Returns null for empty input.
 */
export function parseBrowserActionResult(
  resultText: string | undefined,
): BrowserActionInfo | null {
  const text = (resultText || "").replace(/\r\n/g, "\n").trim();
  if (!text) return null;

  const lines = text.split("\n");
  const info: BrowserActionInfo = { action: "", console: [], body: "" };
  const body: string[] = [];
  let inConsole = false;

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]!;
    if (i === 0) {
      info.action = line.trim();
      continue;
    }
    if (inConsole) {
      const entry = line.trim();
      if (entry) info.console.push(entry);
      continue;
    }
    if (/^url:\s*/i.test(line)) {
      info.url = line.replace(/^url:\s*/i, "").trim();
      continue;
    }
    if (/^screenshot:\s*/i.test(line)) {
      const val = line.replace(/^screenshot:\s*/i, "").trim();
      // "unavailable (...)" means no file was saved.
      if (val && !/^(unavailable|disabled)\b/i.test(val)) {
        info.screenshotName = basename(val);
      } else if (val) {
        info.screenshotUnavailable = val;
      }
      continue;
    }
    if (/^(console|page log)(\s*\([^\n]*\))?:\s*$/i.test(line)) {
      inConsole = true;
      continue;
    }
    body.push(line);
  }
  info.body = body.join("\n").trimEnd();
  return info;
}

/** Malformed or streamed arguments fall back to their raw preview. */
export function browserArgs(text: string | undefined): Record<string, unknown> {
  try {
    const value: unknown = JSON.parse(text || "{}");
    return value && typeof value === "object" && !Array.isArray(value)
      ? (value as Record<string, unknown>)
      : {};
  } catch {
    return {};
  }
}

export function browserOperation(name: string): string {
  return name
    .trim()
    .toLowerCase()
    .replace(/^foxxycode_browser_/, "");
}

export function browserActionLabel(
  name: string,
  argsText: string | undefined,
  status: string,
  t: (key: string) => string,
): string {
  let operation = browserOperation(name);
  const args = browserArgs(argsText);
  if (operation === "inspect") {
    const what =
      typeof args.what === "string" ? args.what.trim().toLowerCase() : "";
    if (["storage", "timing", "memory"].includes(what))
      operation = `inspect_${what}`;
  }
  if (
    operation === "scroll" &&
    typeof args.selector === "string" &&
    args.selector.trim()
  )
    operation = "scroll_to";
  if (operation === "close" && status === "completed") operation = "closed";
  if (operation === "screenshot" && status === "completed")
    operation = "screenshot_done";
  const known = [
    "navigate",
    "evaluate",
    "screenshot",
    "screenshot_done",
    "click",
    "fill",
    "hover",
    "scroll",
    "scroll_to",
    "read_page",
    "page_log",
    "inspect",
    "inspect_storage",
    "inspect_timing",
    "inspect_memory",
    "close",
    "closed",
  ];
  return known.includes(operation) ? t(`messages.browser.${operation}`) : name;
}

export function scrollOffset(value: unknown): number {
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

export function signedOffset(value: number): string {
  return value > 0 ? `+${value}` : `${value}`;
}

/** Builds the HTTP URL that serves a session asset by name. */
export function sessionAssetUrl(sessionId: string, name: string): string {
  return `/foxxycode/sessions/${encodeURIComponent(sessionId)}/assets/${encodeURIComponent(name)}`;
}
