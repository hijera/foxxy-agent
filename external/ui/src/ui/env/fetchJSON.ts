// Shared JSON transport for the SPA's own API calls. Wraps window.fetch (and so the
// remote shim in remoteEnv.ts) and folds every failure into one result shape.
//
// The point of this module is that it never rejects on a network failure. Callers are
// overwhelmingly fire-and-forget — `void refresh(...)` from a setInterval, `void (async
// () => ...)()` from a useEffect — and the IDE panels install an `unhandledrejection`
// listener that paints a red "FoxxyCode UI error" overlay over the bottom of the chat
// for anything that escapes. A single dropped keep-alive connection on 127.0.0.1 is
// routine (the session-stats poll alone runs every 800 ms for the whole of a turn), so
// letting one reject turned a survivable hiccup into a panel that looked crashed.
//
// Same contract as tasks/api.ts: **status 0 means the server gave us no usable answer**
// — refused, reset, or the body cut off mid-read — as opposed to a real HTTP status.

import { isAbortError } from "./remoteErrors";

export type FetchJSONResult<T> = { ok: boolean; status: number; data?: T };

export async function fetchJSON<T>(
  path: string,
  init?: RequestInit,
): Promise<FetchJSONResult<T>> {
  try {
    const res = await fetch(path, init);
    const status = res.status;
    if (!res.ok) {
      return { ok: false, status };
    }
    const data = (await res.json()) as T;
    return { ok: true, status, data };
  } catch (err) {
    // An AbortController.abort() is the caller's own Stop, not an outage: keep it
    // distinguishable so a cancelled request is never mistaken for a failed one.
    if (isAbortError(err)) throw err;
    return { ok: false, status: 0 };
  }
}
