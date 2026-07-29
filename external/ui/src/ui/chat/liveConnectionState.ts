/**
 * Sessions whose dropped live stream is currently being re-attached.
 *
 * App.tsx already auto-reconnects (scheduleLiveStreamReconnect / reconnectLiveStreamIfActive)
 * but keeps the state in a ref, which never renders. Rather than thread a prop through
 * App -> ChatScreen -> MessageList, this is a module store consumed with
 * useSyncExternalStore — the same pattern as i18n/sendModeConfig.ts and shellBreakpoint.ts.
 */

const reconnecting = new Set<string>();
const listeners = new Set<() => void>();

/** Bumped on every real change so getSnapshot can return a stable primitive. */
let epoch = 0;

function notify(): void {
  epoch++;
  for (const cb of listeners) {
    cb();
  }
}

/** Mark a session as re-attaching a dropped live stream. */
export function markReconnecting(sessionId: string): void {
  const key = sessionId.trim();
  if (!key || reconnecting.has(key)) {
    return;
  }
  reconnecting.add(key);
  notify();
}

/** Clear the re-attaching flag (stream rejoined, turn finished, or reconnect abandoned). */
export function markConnected(sessionId: string): void {
  const key = sessionId.trim();
  if (!key || !reconnecting.delete(key)) {
    return;
  }
  notify();
}

/** True while this session's live stream is being re-attached. */
export function isReconnecting(sessionId: string): boolean {
  const key = sessionId.trim();
  return key !== "" && reconnecting.has(key);
}

/** useSyncExternalStore subscribe. */
export function subscribeLiveConnection(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** useSyncExternalStore getSnapshot — a number, never the mutable Set. */
export function snapshotLiveConnection(): number {
  return epoch;
}

/** useSyncExternalStore getServerSnapshot. */
export function serverSnapshotLiveConnection(): number {
  return 0;
}

/** Test helper: drop all flags so cases cannot leak into each other. */
export function resetLiveConnectionState(): void {
  if (reconnecting.size === 0) {
    return;
  }
  reconnecting.clear();
  notify();
}
