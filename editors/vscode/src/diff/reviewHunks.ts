import { diffArrays } from "diff";

export interface ReviewHunk {
  start: number;
  end: number;
  before: string;
  after: string;
}

export interface ReviewChange {
  start: number;
  end: number;
  text: string;
}

export function editorReviewText(text: string, eol: string): string {
  return text.replace(/^\uFEFF/, "").replace(/\r\n?|\n/g, eol);
}

/** Offsets refer to the applied text, including line endings. Deleted blocks
 *  have an empty range at the point where their old lines belong. */
export function reviewHunks(before: string, after: string): ReviewHunk[] {
  const lines = (text: string) => text.match(/[^\r\n]*(?:\r\n|\r|\n)|[^\r\n]+$/g) ?? [];
  const budget = { maxEditLength: 1000, timeout: 100 };
  const parts = diffArrays(lines(before), lines(after), budget);
  if (!parts) return [];
  const hunks: ReviewHunk[] = [];
  let offset = 0;
  let pending: ReviewHunk | undefined;
  for (const part of parts) {
    const text = part.value.join("");
    if (!part.added && !part.removed) {
      pending = undefined;
      offset += text.length;
      continue;
    }
    if (!pending) {
      pending = { start: offset, end: offset, before: "", after: "" };
      hunks.push(pending);
    }
    if (part.added) {
      offset += text.length;
      pending.end = offset;
      pending.after += text;
    } else {
      pending.before += text;
    }
  }
  return hunks;
}

/** Track positions across actual document events. An edit inside a reviewed
 *  range or at a deletion's empty anchor invalidates only that decision. */
export function moveReviewHunks<T extends ReviewHunk>(
  hunks: readonly T[],
  changes: readonly ReviewChange[],
): T[] {
  let moved = [...hunks];
  for (const { start, end, text } of [...changes].sort((a, b) => b.start - a.start)) {
    const delta = text.length - (end - start);
    moved = moved.flatMap((hunk) => {
      if (hunk.start === hunk.end && start === hunk.start) return [];
      if (end <= hunk.start) return [{ ...hunk, start: hunk.start + delta, end: hunk.end + delta }];
      if (start >= hunk.end) return [hunk];
      return [];
    });
  }
  return moved;
}

export function reviewHunksOverlap(a: ReviewHunk, b: ReviewHunk): boolean {
  if (a.start === a.end || b.start === b.end) return a.start <= b.end && b.start <= a.end;
  return a.start < b.end && b.start < a.end;
}

export function reviewedTextMatches(hunk: ReviewHunk, text: string): boolean {
  return hunk.start >= 0 && hunk.end <= text.length && text.slice(hunk.start, hunk.end) === hunk.after;
}
