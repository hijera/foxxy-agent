/**
 * Paste-to-chip: pure helpers behind the composer's paste interception.
 *
 * In an editor embed, text pasted into the composer is classified by the
 * backend (**`POST /foxxycode/ide/paste-classify`**) against fragments recently
 * copied in the IDE. A match becomes an inline mention token
 * (**`@path:start-end`** or **`@terminal[:name]`**) instead of the raw text.
 */

import { normalizeRelPath } from "../skills/normalizeRelPath";

export type PasteClassifyResult =
  | { kind: "none" }
  | { kind: "file"; pathRel: string; startLine: number; endLine: number }
  | { kind: "terminal"; terminalName: string };

/** Mirror of the server-side gates so hopeless pastes skip the round-trip. */
export const PASTE_CLASSIFY_MAX_BYTES = 64 * 1024;
export const PASTE_CLASSIFY_MIN_SINGLE_LINE_CHARS = 16;
/** Classification budget; on timeout the paste degrades to plain text. */
export const PASTE_CLASSIFY_TIMEOUT_MS = 300;

/**
 * True when pasted text is worth classifying: editor embeds only (a plain
 * browser has no IDE to have copied from), non-trivial text under the size cap.
 */
export function shouldAttemptPasteClassify(text: string, editorEmbed: boolean): boolean {
  if (!editorEmbed) {
    return false;
  }
  if (text.length > PASTE_CLASSIFY_MAX_BYTES) {
    return false;
  }
  const norm = text.replace(/\r\n/g, "\n").replace(/\n+$/, "");
  if (norm === "" || norm.trim() === "") {
    return false;
  }
  if (!norm.includes("\n") && norm.length < PASTE_CLASSIFY_MIN_SINGLE_LINE_CHARS) {
    return false;
  }
  return true;
}

/**
 * The mention token for a classification, without spacing, or **`null`** when
 * the result does not chip (kind `none`, bad range, empty path).
 */
export function pasteChipToken(result: PasteClassifyResult): string | null {
  if (result.kind === "file") {
    const rel = normalizeRelPath(result.pathRel);
    if (rel === "" || result.startLine < 1 || result.endLine < result.startLine) {
      return null;
    }
    return `@${rel}:${result.startLine}-${result.endLine}`;
  }
  if (result.kind === "terminal") {
    const name = (result.terminalName || "").trim();
    return name !== "" ? `@terminal:${name}` : "@terminal";
  }
  return null;
}

/** Side-map key for a file chip's captured literal (`path:start-end`). */
export function pasteChipLiteralKey(pathRel: string, startLine: number, endLine: number): string {
  return `${normalizeRelPath(pathRel)}:${startLine}-${endLine}`;
}

/**
 * Asks the backend to classify pasted text. Any failure — network, non-200,
 * timeout, malformed body — degrades to `{kind: "none"}` so the paste falls
 * back to plain text.
 */
export async function classifyPastedText(
  text: string,
  sessionId: string,
  fetchImpl: typeof fetch = fetch,
): Promise<PasteClassifyResult> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), PASTE_CLASSIFY_TIMEOUT_MS);
  try {
    const headers: Record<string, string> = { "Content-Type": "application/json" };
    const sid = sessionId.trim();
    if (sid) {
      headers["X-FoxxyCode-Session-ID"] = sid;
    }
    const res = await fetchImpl("/foxxycode/ide/paste-classify", {
      method: "POST",
      headers,
      body: JSON.stringify({ text }),
      signal: controller.signal,
    });
    if (!res.ok) {
      return { kind: "none" };
    }
    const body = (await res.json()) as {
      kind?: string;
      pathRel?: string;
      startLine?: number;
      endLine?: number;
      terminalName?: string;
    };
    if (
      body.kind === "file" &&
      typeof body.pathRel === "string" &&
      typeof body.startLine === "number" &&
      typeof body.endLine === "number"
    ) {
      return {
        kind: "file",
        pathRel: body.pathRel,
        startLine: body.startLine,
        endLine: body.endLine,
      };
    }
    if (body.kind === "terminal") {
      return { kind: "terminal", terminalName: body.terminalName || "" };
    }
    return { kind: "none" };
  } catch {
    return { kind: "none" };
  } finally {
    clearTimeout(timer);
  }
}
