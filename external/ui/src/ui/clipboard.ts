/** Copy text even when an embedded browser denies the asynchronous Clipboard API. */
export async function copyText(text: string): Promise<boolean> {
  if (!text) return false;
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    const previousFocus = document.activeElement;
    const textarea = document.createElement("textarea");
    textarea.value = text;
    textarea.style.position = "fixed";
    textarea.style.opacity = "0";
    try {
      document.body.appendChild(textarea);
      textarea.focus();
      textarea.select();
      return document.execCommand("copy");
    } catch {
      return false;
    } finally {
      textarea.remove();
      if (previousFocus instanceof HTMLElement && previousFocus.isConnected) previousFocus.focus();
    }
  }
}
