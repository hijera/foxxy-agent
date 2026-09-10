import { useCallback, useEffect, useRef, useState } from "react";
import { fetchSessionChanges } from "./api";
import { EMPTY_SESSION_CHANGES, type SessionChanges } from "./types";

/**
 * Loads the session change set and refreshes it when a turn finishes.
 *
 * The set only moves when the agent stops working, so this watches the
 * generating flag falling rather than polling: the same true -> false edge the
 * desktop "plan ready" toast uses. Cheaper than a timer and never stale by more
 * than the time it takes one fetch to return.
 */
export function useSessionChanges(params: {
  sessionId: string;
  enabled: boolean;
  generating: boolean;
}): { changes: SessionChanges; reload: () => void } {
  const { sessionId, enabled, generating } = params;
  const [changes, setChanges] = useState<SessionChanges>(EMPTY_SESSION_CHANGES);
  const [epoch, setEpoch] = useState(0);
  const wasGenerating = useRef(generating);

  const reload = useCallback(() => setEpoch((n) => n + 1), []);

  useEffect(() => {
    const finished = wasGenerating.current && !generating;
    wasGenerating.current = generating;
    if (finished) {
      reload();
    }
  }, [generating, reload]);

  useEffect(() => {
    if (!enabled || !sessionId.trim()) {
      setChanges(EMPTY_SESSION_CHANGES);
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
    })();
    return () => {
      cancelled = true;
    };
  }, [sessionId, enabled, epoch]);

  return { changes, reload };
}
