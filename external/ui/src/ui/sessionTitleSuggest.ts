export type SessionTitleRefreshDeps = {
  /** Resolves once the persisted session ID is known (usually after `/v1/responses` headers). */
  sessionIdPromise: Promise<string>;
  /** Reloads session metadata (title) from the backend for the given session. */
  refresh: (sessionId: string) => void;
  /** Delays (ms after the id is known) at which to refresh; defaults cover a few seconds. */
  delaysMs?: number[];
  /** Test seam for scheduling; defaults to window.setTimeout. */
  schedule?: (fn: () => void, ms: number) => void;
};

/**
 * The backend hidden "title" agent generates an LLM session title asynchronously after the first
 * exchange and persists it. This schedules a few backend refreshes so the freshly generated title
 * surfaces in the UI shortly after it lands, without the frontend generating or pinning a title.
 */
export function scheduleSessionTitleRefresh(
  deps: SessionTitleRefreshDeps,
): void {
  const delays = deps.delaysMs ?? [1000, 2500, 5000, 9000];
  const schedule =
    deps.schedule ??
    ((fn: () => void, ms: number) => {
      window.setTimeout(fn, ms);
    });

  void (async () => {
    let sid: string;
    try {
      sid = (await deps.sessionIdPromise).trim();
    } catch {
      return;
    }
    if (!sid) {
      return;
    }
    for (const ms of delays) {
      schedule(() => deps.refresh(sid), ms);
    }
  })();
}
