// Host → SPA bridge for the VS Code webview embed.
//
// IntelliJ's JCEF panel pushes file mentions into the composer by calling
// `window.foxxycodeUi.insertFileMention(path)` through executeJavaScript. The
// VS Code extension has no such hook: its webview hosts the SPA in a
// cross-origin <iframe>, so the only way in is `postMessage`. The webview
// wrapper (editors/vscode/src/webview/panel.ts) forwards the extension's
// request into the frame and this listener hands it to the same file-mention
// bus the JCEF path uses, so the composer treats both hosts alike.
//
// Message contract (frozen, mirrored in panel.ts):
//   { type: "foxxycode:insertFileMention", paths: string[] }
//
// Trust boundary: only messages whose `source` is the parent window are
// honoured. The wrapper's vscode-webview:// origin is not knowable from inside
// the iframe (same reasoning as embedLocaleBridge.ts), and a stray path string
// can only put an @-mention into the composer, never run anything.

import { isEditorEmbed } from "./embedShell";
import { emitFileMention } from "./skills/fileMentionBus";

export const INSERT_FILE_MENTION_MESSAGE_TYPE = "foxxycode:insertFileMention";

/** Cap per message; matches the pending-queue cap in fileMentionBus. */
export const MAX_HOST_MENTION_PATHS = 32;

/** A path longer than this is not a workspace-relative file path. */
const MAX_PATH_LENGTH = 4096;

/** Workspace-relative paths carried by a host message, or [] when it is not one. */
export function hostMentionPaths(data: unknown): string[] {
  if (!data || typeof data !== "object") {
    return [];
  }
  const msg = data as { type?: unknown; paths?: unknown };
  if (msg.type !== INSERT_FILE_MENTION_MESSAGE_TYPE || !Array.isArray(msg.paths)) {
    return [];
  }
  const out: string[] = [];
  for (const p of msg.paths) {
    if (typeof p !== "string") {
      continue;
    }
    const rel = p.trim();
    if (rel === "" || rel.length > MAX_PATH_LENGTH) {
      continue;
    }
    out.push(rel);
    if (out.length >= MAX_HOST_MENTION_PATHS) {
      break;
    }
  }
  return out;
}

/**
 * Install the bridge once at startup (after bootstrapEmbedFlag). Safe to call
 * in any environment; it only activates inside an editor-embedded iframe.
 * Returns the uninstall function (used by tests), or null when inactive.
 */
export function installEmbedHostBridge(): (() => void) | null {
  if (typeof window === "undefined") {
    return null;
  }
  if (!isEditorEmbed() || window.parent === window) {
    return null;
  }
  const onMessage = (ev: MessageEvent) => {
    if (ev.source !== window.parent) {
      return;
    }
    for (const rel of hostMentionPaths(ev.data)) {
      emitFileMention(rel);
    }
  };
  window.addEventListener("message", onMessage);
  return () => window.removeEventListener("message", onMessage);
}
