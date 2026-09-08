import { httpGet, httpPost } from "../util/http";
import {
  parseClientConfig,
  type AutocompleteClientConfig,
} from "./clientConfig";

export interface CompletionRequest {
  prefix: string;
  suffix: string;
  path: string;
  language: string;
}

function join(baseUrl: string, path: string): string {
  return (baseUrl.endsWith("/") ? baseUrl : `${baseUrl}/`) + path;
}

/** Reads the client-facing settings; null when the backend is unreachable or answers badly. */
export async function fetchClientConfig(
  baseUrl: string,
  timeoutMs = 3000,
): Promise<AutocompleteClientConfig | null> {
  try {
    const res = await httpGet(join(baseUrl, "foxxycode/completion/config"), timeoutMs);
    if (res.status !== 200) return null;
    return parseClientConfig(res.body);
  } catch {
    return null;
  }
}

/** Reports what happened to a suggestion (shown, accepted, dismissed, cache_hit). Fire-and-forget:
 *  the counters behind GET /foxxycode/completion/stats are diagnostics, never something the
 *  editor waits on. */
export function sendFeedback(baseUrl: string, event: string): void {
  void httpPost(join(baseUrl, "foxxycode/completion/feedback"), {
    body: JSON.stringify({ event }),
    timeoutMs: 2000,
  }).catch(() => undefined);
}

export interface CompletionResult {
  /** Text to insert at the caret; empty when there is nothing to show. */
  text: string;
  /** Set when the provider rate-limited the request: how long automatic requests should pause. */
  pauseMs?: number;
}

const DEFAULT_PAUSE_SECONDS = 10;
const MAX_PAUSE_SECONDS = 60;

/** How long to pause automatic requests after a 429: the Retry-After header in seconds when it
 *  is a sane number, a default otherwise, and never more than a minute so a bad header cannot
 *  switch the feature off for good. Mirrors `SuggestionText.retryAfterSeconds` in IntelliJ. */
export function retryAfterSeconds(header: string | undefined): number {
  const parsed = Number.parseInt((header ?? "").trim(), 10);
  if (!Number.isFinite(parsed) || parsed <= 0) return DEFAULT_PAUSE_SECONDS;
  return Math.min(parsed, MAX_PAUSE_SECONDS);
}

/** Asks for the text to insert at the caret.
 *
 *  Returns an empty text for every failure as well as for "nothing to suggest": a suggestion
 *  that cannot be fetched is simply not drawn, never an error the user has to dismiss mid-typing.
 *  Aborting via `signal` is the normal path, not a fault - it happens on every keystroke that
 *  supersedes an older request. A 429 additionally carries the pause the caller should observe. */
export async function fetchCompletion(
  baseUrl: string,
  req: CompletionRequest,
  timeoutMs: number,
  signal: AbortSignal,
): Promise<CompletionResult> {
  try {
    const res = await httpPost(join(baseUrl, "foxxycode/completion"), {
      body: JSON.stringify(req),
      timeoutMs,
      signal,
    });
    if (res.status === 429) {
      return { text: "", pauseMs: retryAfterSeconds(res.retryAfter) * 1000 };
    }
    if (res.status !== 200) return { text: "" };
    const parsed = JSON.parse(res.body) as { completion?: unknown };
    return { text: typeof parsed.completion === "string" ? parsed.completion : "" };
  } catch {
    return { text: "" };
  }
}
