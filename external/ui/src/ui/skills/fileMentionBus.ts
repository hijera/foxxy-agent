/**
 * Tiny pub/sub used to inject a file **`@`**-mention into the active composer from
 * outside React. The IntelliJ plugin calls **`window.foxxycodeUi.insertFileMention(path)`**
 * (via **`cefBrowser.executeJavaScript`**), which forwards here; the composer subscribes
 * and inserts the short chip at the caret. Modeled on the module-store pattern used
 * elsewhere in the SPA.
 */

type FileMentionListener = (pathRel: string) => void;

const listeners = new Set<FileMentionListener>();

/** Publishes a workspace-relative path to insert as an **`@`**-mention. */
export function emitFileMention(pathRel: string): void {
  for (const cb of [...listeners]) {
    try {
      cb(pathRel);
    } catch {
      // A broken listener must not block the others.
    }
  }
}

/** Subscribes to mention-insert requests. Returns an unsubscribe function. */
export function subscribeFileMention(cb: FileMentionListener): () => void {
  listeners.add(cb);
  return () => {
    listeners.delete(cb);
  };
}
