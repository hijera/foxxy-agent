// User-facing error messages for the environment (local / remote) transport layer. Kept as pure
// functions so the send-flow error handling in App.tsx is unit-testable (regression for issue #60,
// where remote failures were dropped into an empty catch and shown as nothing).

import type { FoxxyCodeEnv } from "./remoteEnv";
import { t } from "../i18n/i18n";

function hostOf(env: FoxxyCodeEnv): string {
  return env.mode === "remote" ? env.baseUrl.replace(/^https?:\/\//, "") : "";
}

/** isAbortError reports whether an error is the user's own AbortController.abort() (intentional
 * Stop), which must stay silent — as opposed to a real network/transport failure. */
export function isAbortError(err: unknown): boolean {
  return !!err && (err as { name?: unknown }).name === "AbortError";
}

/** remoteSendErrorMessage builds a message for a fetch() rejection with no Response object:
 * the remote is offline, DNS/TLS failed, the connection was refused, or a cross-origin response
 * was blocked by CORS. */
export function remoteSendErrorMessage(_err: unknown, env: FoxxyCodeEnv): string {
  if (env.mode === "remote") {
    return t("env.error.cannotReach", { host: hostOf(env) });
  }
  return t("env.error.networkLocal");
}

/** remoteHttpErrorMessage builds a message for a readable non-ok HTTP response. 401/403 get a
 * dedicated auth hint pointing at the environment's token instead of a bare status code. */
export function remoteHttpErrorMessage(status: number, env: FoxxyCodeEnv): string {
  if (status === 401 || status === 403) {
    return env.mode === "remote"
      ? t("env.error.unauthorizedRemote", { host: hostOf(env) })
      : t("env.error.unauthorizedLocal", { status });
  }
  return env.mode === "remote"
    ? t("env.error.requestFailedRemote", { host: hostOf(env), status })
    : t("env.error.requestFailedLocal", { status });
}
