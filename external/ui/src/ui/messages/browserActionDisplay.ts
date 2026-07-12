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
  /** Console/exception lines captured during the action. */
  console: string[];
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
  const info: BrowserActionInfo = { action: "", console: [] };
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
      if (val && !val.toLowerCase().startsWith("unavailable")) {
        info.screenshotName = basename(val);
      }
      continue;
    }
    if (/^console:\s*$/i.test(line)) {
      inConsole = true;
      continue;
    }
  }

  return info;
}

/** Builds the HTTP URL that serves a session asset by name. */
export function sessionAssetUrl(sessionId: string, name: string): string {
  return `/foxxycode/sessions/${encodeURIComponent(sessionId)}/assets/${encodeURIComponent(name)}`;
}
