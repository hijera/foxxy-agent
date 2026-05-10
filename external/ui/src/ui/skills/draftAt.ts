import { blockquoteLine, inMarkdownFenceBeforeCaret } from "./draftSlash";

/** Draft menu allows spaces inside the **`@`** path filter token. */
const MENU_PATH_CHAR = /[\p{L}\p{N}_.\\/ \-]/u;

/** Characters read as literal path atoms for attachment extraction after **`@`** (no ASCII space here). */
const FILE_PATH_CHAR = /[\p{L}\p{N}_.\\/\-]/u;

export type AtMenuDraft =
  | { open: false }
  | {
      open: true;
      lineStart: number;
      atIdx: number;
      caret: number;
      prefix: string;
    };

/**
 * When the menu prefix already ends in a dotted file segment and is followed by
 * optional spaces then text whose first word does not look like a path segment,
 * the user is typing the message body, not extending the picker filter.
 */
function prefixClosedAfterFileExtensionForAtMenu(prefix: string): boolean {
  const m = /^(.+\.[\p{L}\p{N}]{1,16})\s+(\S[\s\S]*)$/u.exec(prefix);
  if (!m) {
    return false;
  }
  const cont = (m[2] ?? "").trim();
  if (cont === "") {
    return false;
  }
  const word = /^([\p{L}\p{N}_][\p{L}\p{N}_.\-]*)/u.exec(cont);
  const w = word ? (word[1] ?? "") : "";
  if (w.includes("/") || w.includes(".")) {
    return false;
  }
  return true;
}

/**
 * Workspace-relative **`@path`** spans (files only) in document order.
 * Folder navigation tokens end with **`/`** and are omitted.
 */
export function listAtPathSpans(text: string): { start: number; end: number; path: string }[] {
  const out: { start: number; end: number; path: string }[] = [];
  let i = 0;
  const n = text.length;
  while (i < text.length) {
    const j = text.indexOf("@", i);
    if (j < 0) {
      break;
    }
    if (inMarkdownFenceBeforeCaret(text, j + 1)) {
      i = j + 1;
      continue;
    }
    const lineStart = text.lastIndexOf("\n", j - 1) + 1;
    const lineEndIdx = text.indexOf("\n", j);
    const lineEnd = lineEndIdx < 0 ? text.length : lineEndIdx;
    if (blockquoteLine(text.slice(lineStart, lineEnd))) {
      i = j + 1;
      continue;
    }
    if (j > 0 && !/\s/.test(text[j - 1]!)) {
      i = j + 1;
      continue;
    }
    let k = j + 1;
    while (k < n) {
      const ch = text[k]!;
      if (ch === "@" || ch === "\r" || ch === "\n") {
        break;
      }
      if (FILE_PATH_CHAR.test(ch)) {
        k++;
        continue;
      }
      if (/\s/.test(ch)) {
        const tail = text.slice(k + 1);
        const afterWs = tail.replace(/^[ \t]+/, "");
        const word = /^([\p{L}\p{N}_][\p{L}\p{N}_.\-]*)/u.exec(afterWs);
        const wd = word ? (word[1] ?? "") : "";
        if (wd !== "" && (wd.includes("/") || wd.includes("."))) {
          k++;
          continue;
        }
        break;
      }
      break;
    }
    const raw = text.slice(j + 1, k).replace(/\s+$/, "");
    i = k;
    if (!raw || raw.includes("..")) {
      continue;
    }
    if (raw.endsWith("/")) {
      continue;
    }
    out.push({ start: j, end: k, path: raw });
  }
  return out;
}

/**
 * Builds attachment list from **`@path`** occurrences in composer text (files only).
 * Folder navigation tokens end with **`/`** and are skipped.
 */
export function extractAtFileAttachments(text: string): { path: string }[] {
  const out: { path: string }[] = [];
  const seen = new Set<string>();
  for (const sp of listAtPathSpans(text)) {
    if (seen.has(sp.path)) {
      continue;
    }
    seen.add(sp.path);
    out.push({ path: sp.path });
  }
  return out;
}

export function draftExtendsFailedAtPrefix(
  draft: AtMenuDraft,
  failed: { atIdx: number; prefix: string },
): boolean {
  if (!draft.open || draft.atIdx !== failed.atIdx) {
    return false;
  }
  if (failed.prefix === "") {
    return true;
  }
  return (
    draft.prefix === failed.prefix || draft.prefix.startsWith(failed.prefix)
  );
}

/**
 * When the current line ends (before caret) with optional whitespace then **`@`** + optional path token,
 * with **`@`** preceded by line start or ASCII whitespace, returns picker state after **`@`**.
 */
export function atMenuDraftAtCaret(text: string, caret: number): AtMenuDraft {
  if (caret < 0 || caret > text.length) {
    return { open: false };
  }
  if (inMarkdownFenceBeforeCaret(text, caret)) {
    return { open: false };
  }
  const lineStart = text.lastIndexOf("\n", caret - 1) + 1;
  const lineEndIdx = text.indexOf("\n", caret);
  const lineEnd = lineEndIdx < 0 ? text.length : lineEndIdx;
  const line = text.slice(lineStart, lineEnd);
  if (blockquoteLine(line)) {
    return { open: false };
  }
  const caretInLine = caret - lineStart;
  const beforeCaret = line.slice(0, caretInLine);

  for (let i = beforeCaret.length - 1; i >= 0; i--) {
    if (beforeCaret[i] !== "@") {
      continue;
    }
    const after = beforeCaret.slice(i + 1);
    let ok = true;
    for (let c = 0; c < after.length; c++) {
      if (!MENU_PATH_CHAR.test(after[c]!)) {
        ok = false;
        break;
      }
    }
    if (!ok) {
      continue;
    }
    if (i > 0 && !/\s/.test(beforeCaret[i - 1]!)) {
      continue;
    }
    if (prefixClosedAfterFileExtensionForAtMenu(after)) {
      continue;
    }
    const atIdx = lineStart + i;
    const prefix = after;
    return { open: true, lineStart, atIdx, caret, prefix };
  }
  return { open: false };
}
