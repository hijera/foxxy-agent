/** One `edit_proposed` / `edit_applied` event from the foxxycode `GET /foxxycode/ide/events`
 *  SSE stream. Mirrors the Go `ideEvent` struct in `external/httpserver/ideevents.go` and
 *  `editors/intellij/.../diff/FoxxyCodeEditEvent.kt`. */
export interface EditEvent {
  type: "edit_proposed" | "edit_applied" | string;
  toolCallId: string;
  sessionId: string;
  /** Absolute file path. */
  path: string;
  before: string;
  after: string;
}

export function isProposed(ev: EditEvent): boolean {
  return ev.type === "edit_proposed";
}

export function isApplied(ev: EditEvent): boolean {
  return ev.type === "edit_applied";
}

/** Parse one SSE `data:` payload into an EditEvent, or `null` if the payload
 *  is not a valid event object. Mirrors `FoxxyCodeIdeEventClient.parse`. */
export function parseEditEvent(payload: string): EditEvent | null {
  try {
    const o = JSON.parse(payload);
    if (typeof o !== "object" || o === null) return null;
    const type = typeof o.type === "string" ? o.type : "";
    if (type === "") return null;
    const str = (k: string): string =>
      o[k] !== undefined && o[k] !== null ? String(o[k]) : "";
    return {
      type,
      toolCallId: str("toolCallId"),
      sessionId: str("sessionId"),
      path: str("path"),
      before: str("before"),
      after: str("after"),
    };
  } catch {
    return null;
  }
}
