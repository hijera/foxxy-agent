import { editorEmbedId } from "./embedShell";

/** Frozen SPA → VS Code host request; the wrapper forwards only its own frame. */
export const DOWNLOAD_TEXT_MESSAGE_TYPE = "foxxycode:downloadText";

/** The VS Code host owns saving files; blob downloads in its iframe may be cancelled. */
export function requestHostTextDownload(filename: string, text: string): boolean {
  if (editorEmbedId() !== "vscode" || window.parent === window) return false;
  window.parent.postMessage({ type: DOWNLOAD_TEXT_MESSAGE_TYPE, filename, text }, "*");
  return true;
}
