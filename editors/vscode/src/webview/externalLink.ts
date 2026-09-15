// The SPA runs in a cross-origin iframe inside a webview sandboxed without allow-popups, so its
// new-tab links cannot open anything by themselves. The SPA posts them instead
// (external/ui/src/ui/embedExternalLinks.ts), the wrapper script in panel.ts relays the message,
// and the extension host opens the URL in the system browser. This file is the host-side check,
// kept free of the vscode module so it runs under plain vitest.

/** Message contract, mirrored in external/ui/src/ui/embedExternalLinks.ts. */
export const OPEN_EXTERNAL_MESSAGE_TYPE = "foxxycode:openExternal";

/**
 * The URL an `openExternal` message asks for, or null when the message is anything else or the
 * URL is not plain http(s): a page must never make the extension open `file:`, `command:` or
 * `vscode:` URIs.
 */
export function externalUrlFromMessage(msg: unknown): string | null {
  if (!msg || typeof msg !== "object") return null;
  const { type, url } = msg as { type?: unknown; url?: unknown };
  if (type !== OPEN_EXTERNAL_MESSAGE_TYPE || typeof url !== "string") return null;
  try {
    const parsed = new URL(url);
    return parsed.protocol === "http:" || parsed.protocol === "https:" ? parsed.href : null;
  } catch {
    return null;
  }
}
