// Editor-embed detection for the VS Code / IntelliJ plugin webviews.
//
// The plugins navigate the SPA to `…/?embed=intellij#/chat`
// (VS Code: editors/vscode/src/webview/panel.ts EMBED_ID;
//  IntelliJ: FoxxyCodeBrowserPanel.kt), so the SPA can adopt a flatter native
// host look and hide host-provided chrome such as the project (cwd) picker —
// inside an editor the working directory is fixed to the open IDE project.
//
// Hash-route changes inside the SPA keep the query string, but to stay robust we
// latch the marker into sessionStorage on first load so `isEditorEmbed()` works
// from anywhere without re-parsing (mirrors desktopShell.ts).

const STORAGE_KEY = "foxxycode.embed";

/** Parse the `?embed=<id>` value from the current URL query, or "" if absent. */
export function readEmbedFromUrl(): string {
  if (typeof window === "undefined") {
    return "";
  }
  const m = window.location.search.match(/[?&]embed=([^&]+)/);
  return m ? decodeURIComponent(m[1]) : "";
}

/**
 * Latch the embed id from the URL into sessionStorage (once per tab) and return
 * whether we are running inside an editor-plugin webview. Safe to call before
 * React mounts, mirroring bootstrapDesktopFlag().
 */
export function bootstrapEmbedFlag(): boolean {
  if (typeof window === "undefined") {
    return false;
  }
  const id = readEmbedFromUrl();
  if (id) {
    try {
      window.sessionStorage.setItem(STORAGE_KEY, id);
    } catch {
      // Ignore storage failures (private mode / disabled storage).
    }
    return true;
  }
  return isEditorEmbed();
}

/** Whether the SPA is running inside an editor-plugin webview (latched flag). */
export function isEditorEmbed(): boolean {
  if (typeof window === "undefined") {
    return false;
  }
  if (readEmbedFromUrl()) {
    return true;
  }
  try {
    if (window.sessionStorage.getItem(STORAGE_KEY)) {
      return true;
    }
  } catch {
    // Ignore storage failures; fall through to the DOM marker.
  }
  // index.html's inline bootstrap sets data-embed before React mounts.
  try {
    return !!document.documentElement.dataset.embed;
  } catch {
    return false;
  }
}
