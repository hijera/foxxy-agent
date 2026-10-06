/**
 * Tiny pub/sub used to inject a file **`@`**-mention into the active composer from
 * outside React. The IntelliJ plugin calls **`window.foxxycodeUi.insertFileMention(path)`**
 * (via **`cefBrowser.executeJavaScript`**), which forwards here; the composer subscribes
 * and inserts the mention at the caret. Modeled on the module-store pattern used
 * elsewhere in the SPA.
 */

type FileMentionListener = (pathRel: string) => void;

const listeners = new Set<FileMentionListener>();

/**
 * Paths emitted while nothing was listening. **`installFoxxyCodeUiApi()`** runs before
 * React renders, so a drop that lands right after the tool window opens reaches the bus
 * before the composer has mounted and subscribed — without this queue it would be dropped
 * silently, which is why the first drag of a session used to do nothing.
 */
let pending: string[] = [];

/** Cap on the queue so a panel that never mounts a composer cannot grow it forever. */
export const MAX_PENDING_MENTIONS = 32;

function deliver(cb: FileMentionListener, pathRel: string): void {
  try {
    cb(pathRel);
  } catch {
    // A broken listener must not block the others.
  }
}

/** Publishes a workspace-relative path to insert as an **`@`**-mention. */
export function emitFileMention(pathRel: string): void {
  if (listeners.size === 0) {
    pending.push(pathRel);
    if (pending.length > MAX_PENDING_MENTIONS) {
      pending = pending.slice(pending.length - MAX_PENDING_MENTIONS);
    }
    return;
  }
  for (const cb of [...listeners]) {
    deliver(cb, pathRel);
  }
}

/**
 * Subscribes to mention-insert requests. Returns an unsubscribe function. The first
 * subscriber drains whatever was emitted before anyone was listening, in emission order.
 */
export function subscribeFileMention(cb: FileMentionListener): () => void {
  listeners.add(cb);
  if (pending.length > 0) {
    const queued = pending;
    pending = [];
    for (const p of queued) {
      deliver(cb, p);
    }
  }
  return () => {
    listeners.delete(cb);
  };
}
