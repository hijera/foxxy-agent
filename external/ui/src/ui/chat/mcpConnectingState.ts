/**
 * Sessions whose current turn is parked waiting for their configured MCP servers to connect.
 *
 * The backend emits **`event: mcp_phase`** (`connecting` / `ready`) only when a turn actually
 * has to wait — a warm session never sends it. Nothing in the transcript can express that
 * state, so without this the status row would read "waiting for the model" while no model call
 * has been made yet.
 *
 * A module store rather than a prop through App -> ChatScreen -> MessageList, matching
 * liveConnectionState.ts.
 */

const connecting = new Set<string>();
const listeners = new Set<() => void>();

/** Bumped on every real change so getSnapshot can return a stable primitive. */
let epoch = 0;

function notify(): void {
  epoch++;
  for (const cb of listeners) {
    cb();
  }
}

/** Record whether this session's turn is currently waiting on its MCP servers. */
export function setMcpConnecting(sessionId: string, value: boolean): void {
  const key = sessionId.trim();
  if (!key) {
    return;
  }
  const changed = value ? !connecting.has(key) : connecting.delete(key);
  if (value) {
    connecting.add(key);
  }
  if (changed) {
    notify();
  }
}

/** True while this session's turn is waiting for its MCP servers. */
export function isMcpConnecting(sessionId: string): boolean {
  const key = sessionId.trim();
  return key !== "" && connecting.has(key);
}

/** useSyncExternalStore subscribe. */
export function subscribeMcpConnecting(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** useSyncExternalStore getSnapshot — a number, never the mutable Set. */
export function snapshotMcpConnecting(): number {
  return epoch;
}

/** useSyncExternalStore getServerSnapshot. */
export function serverSnapshotMcpConnecting(): number {
  return 0;
}

/** Test helper: drop all flags so cases cannot leak into each other. */
export function resetMcpConnectingState(): void {
  if (connecting.size === 0) {
    return;
  }
  connecting.clear();
  notify();
}
