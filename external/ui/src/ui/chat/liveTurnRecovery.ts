/**
 * Helpers for getting back to an agent turn that outlived the page: after an editor
 * webview reload, or when a send is refused because the chat is still working.
 * The backend keeps such a turn running and keeps holding the session turn lock.
 */

/** Message the composer relay sends when there is nothing to attach to. */
export const NO_LIVE_COMPOSER_STREAM = "no active composer stream";

/** `error.code` the composer relay uses for "no turn is running for this session". */
export const NO_ACTIVE_STREAM_CODE = "no_active_stream";

/**
 * True when a relay stream ended because the turn is not (or no longer) live,
 * rather than because the model or a tool failed. Such a stream must not surface
 * as a red error in the transcript - the caller falls back to the persisted rows.
 *
 * The code is authoritative; the message check stays for servers that predate it.
 */
export function isNoLiveTurnRelayError(
  code: string | null | undefined,
  message?: string | null,
): boolean {
  if (typeof code === "string" && code.trim() === NO_ACTIVE_STREAM_CODE) {
    return true;
  }
  if (!message) return false;
  return message.trim().toLowerCase().includes(NO_LIVE_COMPOSER_STREAM);
}

export type SessionBusyInfo = {
  /** The send was refused because an agent turn is in flight. */
  busy: boolean;
  /** Session that is working, when the server named one. */
  sessionId: string;
  /** Server-provided text, for surfacing when the UI has nothing better. */
  message: string;
};

const NOT_BUSY: SessionBusyInfo = { busy: false, sessionId: "", message: "" };

function readString(source: Record<string, unknown>, key: string): string {
  const v = source[key];
  return typeof v === "string" ? v.trim() : "";
}

/**
 * Read a 409 from POST /v1/responses (or /v1/chat/completions). The server sends
 * `{"error":{"message","code":"session_busy","sessionId","turnActive"}}`; older
 * builds send the message alone, so the status carries the decision and every
 * other field is optional.
 */
export function parseSessionBusyResponse(
  status: number,
  body: unknown,
): SessionBusyInfo {
  if (status !== 409) return NOT_BUSY;
  if (!body || typeof body !== "object") {
    return { busy: true, sessionId: "", message: "" };
  }
  const err = (body as { error?: unknown }).error;
  if (!err || typeof err !== "object") {
    return { busy: true, sessionId: "", message: "" };
  }
  const rec = err as Record<string, unknown>;
  const code = readString(rec, "code");
  const message = readString(rec, "message");
  if (code !== "" && code !== "session_busy") {
    // A 409 this client does not know how to recover from (e.g. a locked workspace).
    return { busy: false, sessionId: "", message };
  }
  return { busy: true, sessionId: readString(rec, "sessionId"), message };
}

/**
 * Backoff and give-up rules for re-attaching to a running turn.
 *
 * The old rules were fixed counts: five reconnect attempts, then 150 poll ticks
 * (~5 minutes). Both were reset by a window focus event — which never fires for
 * someone sitting and watching an editor panel, the one place a turn most often
 * runs for half an hour (a foreground `spawn_agent` may run up to
 * `subagents.default_timeout_seconds`, 1800 by default). So the panel went
 * quiet while the backend was still working.
 *
 * What replaces them is server truth: keep trying while `/activity` says the
 * turn is alive, and give up only when the server itself stops answering.
 */

/** First reconnect delay; each further attempt doubles it. */
export const LIVE_RECONNECT_BASE_DELAY_MS = 400;

/** Ceiling for the reconnect backoff, so a long turn keeps a slow heartbeat. */
export const LIVE_RECONNECT_MAX_DELAY_MS = 15000;

/** Consecutive failed /activity probes before a session is given up on. */
export const ACTIVITY_FAIL_MAX = 5;

/** Fast poll while the transcript is growing. */
export const DISK_FALLBACK_MIN_MS = 2000;

/** Slow poll once nothing has changed for a while. */
export const DISK_FALLBACK_MAX_MS = 10000;

/** Unchanged polls tolerated before backing off to the slow cadence. */
export const DISK_FALLBACK_QUIET_TICKS = 5;

/** Exponential backoff for reconnect attempt `attempt` (0-based). */
export function liveReconnectDelayMs(attempt: number): number {
  const n = attempt > 0 ? attempt : 0;
  const delay = LIVE_RECONNECT_BASE_DELAY_MS * Math.pow(2, n);
  return Math.min(delay, LIVE_RECONNECT_MAX_DELAY_MS);
}

/**
 * The poll interval, given how many consecutive ticks saw no new messages. A
 * turn that is producing output is followed closely; one that is thinking is
 * checked on rather than watched.
 */
export function diskFallbackDelayMs(quietTicks: number): number {
  return quietTicks >= DISK_FALLBACK_QUIET_TICKS
    ? DISK_FALLBACK_MAX_MS
    : DISK_FALLBACK_MIN_MS;
}

/**
 * Whether a poll tick should reload the transcript.
 *
 * `messageSeq` counts the messages of a live session, so it moves inside a
 * turn. A tick that sees the same value has nothing to fetch. The first tick
 * always reloads (the client has nothing yet), and so does the tick that
 * observes the turn ending, which is where the final answer lands. A server
 * that does not report `messageSeq` at all — an older build, or a session not
 * live in that process — falls back to reloading every tick, which is what
 * this did before.
 */
export function shouldReloadTranscript(opts: {
  firstTick: boolean;
  turnActive: boolean;
  messageSeq: number | undefined;
  lastMessageSeq: number | undefined;
}): boolean {
  if (opts.firstTick || !opts.turnActive) {
    return true;
  }
  if (opts.messageSeq === undefined) {
    return true;
  }
  return opts.messageSeq !== opts.lastMessageSeq;
}

/** Whether to keep polling / retrying at all, given consecutive probe failures. */
export function shouldKeepWatching(activityFailures: number): boolean {
  return activityFailures < ACTIVITY_FAIL_MAX;
}
