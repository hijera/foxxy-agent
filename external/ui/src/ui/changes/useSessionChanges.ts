import { useCallback, useEffect, useRef, useState } from "react";
import { fetchSessionChanges } from "./api";
import { onChangesSettled, onChangesToggle } from "./sessionChangesBus";
import { EMPTY_SESSION_CHANGES, type SessionChanges } from "./types";

/**
 * How long the card waits for the server's word after a turn ends before it
 * reads the change set anyway: the event stream can be down, and a turn run
 * through another door (ACP, the console) announces nothing.
 */
export const SETTLE_FALLBACK_MS = 4000;

/** A burst of tool calls is read once, this long after the last of them. */
export const TOOL_ACTIVITY_DEBOUNCE_MS = 400;

/**
 * Loads the session change set and decides when the card is on screen.
 *
 * - While the agent works the card steps aside: the set is still moving, and a
 *   card describing the turns before would read as out of date.
 * - When the turn ends the card waits for `session_changes` from the server -
 *   the moment the turn's diff is on disk - then reads and shows. Reading on the
 *   end of the stream instead raced the capture and could show the old set.
 * - Ctrl+S / Cmd+S (or the IntelliJ plugin, through the bus) shows or hides it
 *   at any time. Opened mid-turn it reads what the server has: the finished
 *   turns plus what the running one has written so far, and it reads again
 *   after every finished tool call while it stays open. Hidden, it reads nothing.
 */
export function useSessionChanges(params: {
  sessionId: string;
  enabled: boolean;
  generating: boolean;
  /** Count of finished tool calls in the transcript. */
  toolActivity?: number;
}): {
  changes: SessionChanges;
  reload: () => void;
  /** Whether the card is on screen. */
  shown: boolean;
  /** Whether the user put it there, rather than the end of a turn. */
  manual: boolean;
} {
  const { sessionId, enabled, generating } = params;
  const toolActivity = params.toolActivity ?? 0;
  const [changes, setChanges] = useState<SessionChanges>(EMPTY_SESSION_CHANGES);
  const [epoch, setEpoch] = useState(0);
  const [shown, setShown] = useState(!generating);
  const [manual, setManual] = useState(false);

  const generatingRef = useRef(generating);
  const wasGenerating = useRef(generating);
  const enabledRef = useRef(enabled);
  enabledRef.current = enabled;
  const shownRef = useRef(shown);
  shownRef.current = shown;
  // The server announced this session's set since the current turn started.
  const settledSinceStart = useRef(false);
  // The turn ended and the card is waiting for the announcement.
  const pending = useRef(false);
  // Reveal the card once the next read lands, so it never flashes the old set.
  const showAfterFetch = useRef(false);
  // A card mounted mid-turn is hidden, and has nothing to read yet.
  const skipFirstFetch = useRef(generating);
  const fallbackTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const reload = useCallback(() => setEpoch((n) => n + 1), []);

  const clearFallback = useCallback(() => {
    if (fallbackTimer.current !== null) {
      clearTimeout(fallbackTimer.current);
      fallbackTimer.current = null;
    }
  }, []);

  const revealFresh = useCallback(() => {
    pending.current = false;
    clearFallback();
    showAfterFetch.current = true;
    reload();
  }, [clearFallback, reload]);

  // The turn boundaries.
  useEffect(() => {
    generatingRef.current = generating;
    const was = wasGenerating.current;
    wasGenerating.current = generating;
    if (!was && generating) {
      settledSinceStart.current = false;
      pending.current = false;
      clearFallback();
      setShown(false);
      setManual(false);
    } else if (was && !generating) {
      if (settledSinceStart.current) {
        revealFresh();
      } else {
        pending.current = true;
        clearFallback();
        fallbackTimer.current = setTimeout(revealFresh, SETTLE_FALLBACK_MS);
      }
    }
  }, [generating, clearFallback, revealFresh]);

  // Another chat in the same view starts from its own state.
  const firstSession = useRef(true);
  useEffect(() => {
    if (firstSession.current) {
      firstSession.current = false;
      return;
    }
    settledSinceStart.current = false;
    pending.current = false;
    clearFallback();
    setManual(false);
    setShown(!generatingRef.current);
  }, [sessionId, clearFallback]);

  // The server's word that the set settled.
  useEffect(
    () =>
      onChangesSettled((sid) => {
        if (sid !== sessionId) {
          return;
        }
        if (generatingRef.current) {
          // A short turn can be stored before this client sees its stream end.
          settledSinceStart.current = true;
          return;
        }
        if (pending.current) {
          revealFresh();
          return;
        }
        if (shownRef.current) {
          // A rollback from another window, a late announcement: stay true.
          reload();
        }
      }),
    [sessionId, revealFresh, reload],
  );

  // Ctrl+S / Cmd+S, and the IntelliJ plugin's shortcut.
  useEffect(
    () =>
      onChangesToggle(() => {
        if (!enabledRef.current) {
          return;
        }
        // A deliberate choice wins over the automatic reveal still pending.
        pending.current = false;
        clearFallback();
        if (shownRef.current) {
          setShown(false);
          setManual(false);
          return;
        }
        setManual(true);
        showAfterFetch.current = true;
        reload();
      }),
    [clearFallback, reload],
  );

  // Open during a turn, the card follows each finished tool call.
  const lastActivity = useRef(toolActivity);
  useEffect(() => {
    const moved = toolActivity !== lastActivity.current;
    lastActivity.current = toolActivity;
    if (!moved || !generating || !shownRef.current) {
      return;
    }
    const timer = setTimeout(reload, TOOL_ACTIVITY_DEBOUNCE_MS);
    return () => clearTimeout(timer);
  }, [toolActivity, generating, reload]);

  useEffect(() => () => clearFallback(), [clearFallback]);

  useEffect(() => {
    if (!enabled || !sessionId.trim()) {
      setChanges(EMPTY_SESSION_CHANGES);
      return;
    }
    if (skipFirstFetch.current) {
      skipFirstFetch.current = false;
      return;
    }
    let cancelled = false;
    void (async () => {
      const res = await fetchSessionChanges(sessionId);
      if (cancelled) {
        return;
      }
      // A failed read leaves the previous set in place rather than blanking the
      // card: a restarting server should not look like "nothing changed".
      if (res.ok) {
        setChanges(res.data);
      }
      if (showAfterFetch.current) {
        showAfterFetch.current = false;
        setShown(true);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [sessionId, enabled, epoch]);

  return { changes, reload, shown, manual };
}
