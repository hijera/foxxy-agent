// SPA → host bridge for links that open in a new tab, in the VS Code webview embed.
//
// VS Code sandboxes the webview without allow-popups, and the SPA runs in a cross-origin iframe
// inside it, so a `target="_blank"` link (Settings → "API docs", "Website", the agents.md link)
// silently does nothing there: window.open returns null. Instead, a click on such a link is
// claimed here and posted to the parent frame; the webview wrapper
// (editors/vscode/src/webview/panel.ts) forwards it to the extension host, which opens the URL
// in the system browser with vscode.env.openExternal.
//
// Message contract (frozen, mirrored in panel.ts):
//   { type: "foxxycode:openExternal", url: "http(s)://…" }
//
// IntelliJ does not need it: JCEF reports the popup to the plugin (CefLifeSpanHandler), which
// opens the system browser itself. The web UI and the desktop shell open new tabs natively.

import { editorEmbedId } from "./embedShell";

export const OPEN_EXTERNAL_MESSAGE_TYPE = "foxxycode:openExternal";

/**
 * The absolute URL a click on `anchor` should open outside the page, or null when the page
 * handles the link itself: same-tab navigation, downloads, and anything that is not http(s).
 */
export function externalLinkUrl(anchor: HTMLAnchorElement): string | null {
  if (anchor.target !== "_blank" || anchor.hasAttribute("download")) {
    return null;
  }
  const href = anchor.getAttribute("href");
  if (!href) {
    return null;
  }
  try {
    const url = new URL(href, window.location.href);
    return url.protocol === "http:" || url.protocol === "https:" ? url.href : null;
  } catch {
    return null;
  }
}

/**
 * Install the bridge once at startup (after bootstrapEmbedFlag). Active only inside the VS Code
 * embed iframe; returns the uninstall function (used by tests), or null when inactive.
 */
export function installEmbedExternalLinks(): (() => void) | null {
  if (typeof window === "undefined" || editorEmbedId() !== "vscode" || window.parent === window) {
    return null;
  }
  const onClick = (ev: MouseEvent) => {
    if (ev.defaultPrevented || ev.button !== 0) {
      return;
    }
    const target = ev.target instanceof Element ? ev.target : null;
    const anchor = target?.closest("a[href]");
    if (!(anchor instanceof HTMLAnchorElement)) {
      return;
    }
    const url = externalLinkUrl(anchor);
    if (!url) {
      return;
    }
    ev.preventDefault();
    try {
      // The vscode-webview:// wrapper origin is not knowable from inside the iframe; the
      // payload is a public link, so "*" is acceptable (same as the locale bridge).
      window.parent.postMessage({ type: OPEN_EXTERNAL_MESSAGE_TYPE, url }, "*");
    } catch {
      // Best-effort: a failed post must never break the SPA.
    }
  };
  document.addEventListener("click", onClick);
  return () => document.removeEventListener("click", onClick);
}
