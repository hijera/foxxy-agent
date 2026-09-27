/**
 * Two signals for the changed-files card that come from outside React.
 *
 * - **Settled**: the server says a session's recorded change set settled or
 *   moved (`event: session_changes` on `GET /foxxycode/events`). App forwards the
 *   event here; the card reads the change set when it arrives, which is the
 *   moment the answer covers the turn that just ended.
 * - **Toggle**: the user asked to show or hide the card. Ctrl+S / Cmd+S in the
 *   page asks, and so does `window.foxxycodeUi.toggleSessionChanges()`, which the
 *   IntelliJ plugin calls when it takes the key from Save All.
 *
 * Module-level pub/sub in the shape of `skills/fileMentionBus.ts`.
 */

type SettledListener = (sessionId: string) => void;
type ToggleListener = () => void;

const settledListeners = new Set<SettledListener>();
const toggleListeners = new Set<ToggleListener>();

/**
 * Requests closer together than this are one press. In IntelliJ a key can reach
 * both the page and the plugin's shortcut; counted twice it would open the card
 * and shut it again in the same instant.
 */
export const TOGGLE_DEDUPE_MS = 150;

const defaultClock = () => performance.now();
let clock: () => number = defaultClock;
let lastToggleAt = Number.NEGATIVE_INFINITY;

function deliver<A extends unknown[]>(cb: (...args: A) => void, ...args: A): void {
  try {
    cb(...args);
  } catch {
    // A broken listener must not block the others.
  }
}

/** Publishes that a session's recorded change set settled or moved. */
export function emitChangesSettled(sessionId: string): void {
  for (const cb of [...settledListeners]) {
    deliver(cb, sessionId);
  }
}

/** Subscribes to settled change sets. Returns unsubscribe. */
export function onChangesSettled(cb: SettledListener): () => void {
  settledListeners.add(cb);
  return () => {
    settledListeners.delete(cb);
  };
}

/**
 * Asks the card to show or hide. Returns false when the request was folded into
 * one made less than TOGGLE_DEDUPE_MS earlier.
 */
export function requestChangesToggle(): boolean {
  const now = clock();
  if (now - lastToggleAt < TOGGLE_DEDUPE_MS) {
    return false;
  }
  lastToggleAt = now;
  for (const cb of [...toggleListeners]) {
    deliver(cb);
  }
  return true;
}

/** Subscribes to toggle requests. Returns unsubscribe. */
export function onChangesToggle(cb: ToggleListener): () => void {
  toggleListeners.add(cb);
  return () => {
    toggleListeners.delete(cb);
  };
}

/** Replaces the clock the dedupe reads (tests only). */
export function setChangesBusClockForTests(now: () => number): void {
  clock = now;
}

/** Drops every listener and restores the clock (tests only). */
export function resetChangesBusForTests(): void {
  settledListeners.clear();
  toggleListeners.clear();
  clock = defaultClock;
  lastToggleAt = Number.NEGATIVE_INFINITY;
}
