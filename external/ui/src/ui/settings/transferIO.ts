// Browser IO helpers for settings import/export and text-file downloads.
export { copyText } from "../clipboard";

/**
 * Read text from the clipboard. Must be called from a user gesture (click).
 * Returns null when the browser blocks access or the API is unavailable.
 */
export async function readClipboardText(): Promise<string | null> {
  try {
    if (!navigator.clipboard || !navigator.clipboard.readText) {
      return null;
    }
    return await navigator.clipboard.readText();
  } catch {
    return null;
  }
}

/** Trigger a download of `text` as a file named `filename`. */
export function downloadTextFile(
  filename: string,
  text: string,
  mime = "application/json",
): void {
  const blob = new Blob([text], { type: `${mime};charset=utf-8` });
  downloadBlob(filename, blob);
}

/**
 * Trigger a download of a pre-built Blob (e.g. a binary PDF/DOCX response).
 * The Blob already carries its own content type; the anchor's `download`
 * attribute names the file.
 */
export function downloadBlob(filename: string, blob: Blob): void {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.style.display = "none";
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  // Revoke on the next tick so the download has a chance to start.
  window.setTimeout(() => URL.revokeObjectURL(url), 0);
}
