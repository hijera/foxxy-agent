/**
 * Helpers for getting back to an agent turn that outlived the page: after an editor
 * webview reload, or when a send is refused because the chat is still working.
 * The backend keeps such a turn running and keeps holding the session turn lock.
 */

/** Message the composer relay sends when there is nothing to attach to. */
export const NO_LIVE_COMPOSER_STREAM = "no active composer stream";

/**
 * True when a relay stream ended because the turn is not (or no longer) live,
 * rather than because the model or a tool failed. Such a stream must not surface
 * as a red error in the transcript - the caller falls back to the persisted rows.
 */
export function isNoLiveTurnRelayError(message: string | null): boolean {
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
