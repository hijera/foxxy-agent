// SPA → host locale bridge for the VS Code webview embed.
//
// The VS Code plugin hosts the SPA in a cross-origin <iframe>, so it cannot
// observe window.foxxycodeUi like IntelliJ's JCEF does. Instead, whenever the
// active locale changes (user flips the language picker, or the host calls
// setLocale), we post the effective locale to the parent frame. The webview
// wrapper (editors/vscode/src/webview/panel.ts) forwards it to the extension
// host so plugin chrome (command titles, menus) switches without a reload.
//
// Message contract (frozen, mirrored in panel.ts):
//   { type: "foxxycode:locale", locale: "en" | "ru" }
//
// In IntelliJ's JCEF the SPA is the top-level document (window.parent ===
// window), so this bridge is a no-op there; IntelliJ subscribes through
// window.foxxycodeUi.onLocaleChange instead.

import { isEditorEmbed } from "./embedShell";
import { getLocale, onLocaleChange } from "./i18n/i18n";

export const LOCALE_MESSAGE_TYPE = "foxxycode:locale";

/**
 * Install the bridge once at startup (after bootstrapEmbedFlag). Safe to call
 * in any environment; it only activates inside an editor-embedded iframe.
 * Returns the unsubscribe function (used by tests), or null when inactive.
 */
export function installEmbedLocaleBridge(): (() => void) | null {
  if (typeof window === "undefined") {
    return null;
  }
  if (!isEditorEmbed() || window.parent === window) {
    return null;
  }
  const post = () => {
    try {
      // No sensitive data in the payload, and the vscode-webview:// wrapper
      // origin is not knowable from inside the iframe, so "*" is acceptable.
      window.parent.postMessage(
        { type: LOCALE_MESSAGE_TYPE, locale: getLocale() },
        "*",
      );
    } catch {
      // Best-effort: a failed post must never break the SPA.
    }
  };
  return onLocaleChange(post);
}
