/**
 * Sessions whose current turn is parked between two attempts at the same model call,
 * because the provider produced no output at all.
 *
 * The backend emits **`event: llm_retry`** (`waiting` / `retrying`) only while a turn
 * actually waits — a call that answers never sends it. The pause is measured in minutes,
 * and nothing in the transcript can express it, so without this the status row would read
 * "waiting for the model" while the agent is deliberately sitting out a saturated hub.
 *
 * A module store rather than a prop through App -> ChatScreen -> MessageList, matching
 * mcpConnectingState.ts and liveConnectionState.ts.
 */

const retrying = new Set<string>();
const listeners = new Set<() => void>();

/** Bumped on every real change so getSnapshot can return a stable primitive. */
let epoch = 0;

function notify(): void {
  epoch++;
  for (const cb of listeners) {
    cb();
  }
}

/** Record whether this session's turn is currently waiting to retry the model. */
export function setLlmRetrying(sessionId: string, value: boolean): void {
  const key = sessionId.trim();
  if (!key) {
    return;
  }
  const changed = value ? !retrying.has(key) : retrying.delete(key);
  if (value) {
    retrying.add(key);
  }
  if (changed) {
    notify();
  }
}

/** True while this session's turn is waiting before re-issuing a silent model call. */
export function isLlmRetrying(sessionId: string): boolean {
  const key = sessionId.trim();
  return key !== "" && retrying.has(key);
}

/** useSyncExternalStore subscribe. */
export function subscribeLlmRetry(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** useSyncExternalStore getSnapshot — a number, never the mutable Set. */
export function snapshotLlmRetry(): number {
  return epoch;
}

/** useSyncExternalStore getServerSnapshot. */
export function serverSnapshotLlmRetry(): number {
  return 0;
}

/** Test helper: drop all flags so cases cannot leak into each other. */
export function resetLlmRetryState(): void {
  if (retrying.size === 0) {
    return;
  }
  retrying.clear();
  notify();
}
