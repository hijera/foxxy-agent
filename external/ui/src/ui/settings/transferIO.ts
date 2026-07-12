// Browser IO helpers for import/export of settings: clipboard read/write and
// text-file download. These are the first such helpers in the SPA; the copy
// logic mirrors the fallback pattern already used in MessageCopyIconButton.tsx.

/** Copy text to the clipboard, falling back to a hidden textarea + execCommand. */
export async function copyText(text: string): Promise<boolean> {
  if (!text) {
    return false;
  }
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    try {
      const ta = document.createElement("textarea");
      ta.value = text;
      ta.style.position = "fixed";
      ta.style.opacity = "0";
      document.body.appendChild(ta);
      ta.focus();
      ta.select();
      const ok = document.execCommand("copy");
      document.body.removeChild(ta);
      return ok;
    } catch {
      return false;
    }
  }
}

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
