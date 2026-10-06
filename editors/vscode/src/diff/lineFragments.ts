import * as diff from "diff";

export interface LineFragment {
  startLine: number; // 0-based, inclusive
  endLine: number; // 0-based, exclusive
  kind: "add" | "del";
}

/** Compute 0-based line ranges to decorate, mirroring the IntelliJ
 *  `ComparisonManager.compareLines` + `useAfterRanges` logic. Pure function —
 *  no vscode dependency, safe to unit-test in isolation. */
export function computeLineFragments(
  before: string,
  after: string,
  useAfterRanges: boolean,
): LineFragment[] {
  const beforeLines = before.split(/\r?\n/);
  const afterLines = after.split(/\r?\n/);
  const changes = diff.diffArrays(beforeLines, afterLines);
  const out: LineFragment[] = [];

  let beforeLine = 0;
  let afterLine = 0;
  for (const change of changes) {
    const len = change.value.length;
    if (change.added) {
      const start = useAfterRanges ? afterLine : beforeLine;
      out.push({ startLine: start, endLine: start + len, kind: "add" });
      afterLine += len;
    } else if (change.removed) {
      if (!useAfterRanges) {
        out.push({ startLine: beforeLine, endLine: beforeLine + len, kind: "del" });
      }
      // On `useAfterRanges`, deletions have no line in the after-side to highlight.
      beforeLine += len;
    } else {
      beforeLine += len;
      afterLine += len;
    }
  }
  return out;
}

/** Splits text into lines the way an editor shows them: CRLF or LF, no BOM. */
export function editorLines(text: string): string[] {
  return text.replace(/^﻿/, "").split(/\r?\n/);
}

/** Line equality that tolerates the event stream's view of a legacy-encoded
 *  file: its bytes are not UTF-8, so every non-ASCII character arrives as one
 *  U+FFFD, while the editor decoded the same character properly. */
export function sameLine(eventLine: string, docLine: string): boolean {
  if (eventLine === docLine) return true;
  if (eventLine.length !== docLine.length || !eventLine.includes("�")) return false;
  for (let i = 0; i < eventLine.length; i++) {
    if (eventLine[i] !== docLine[i] && eventLine[i] !== "�") return false;
  }
  return true;
}

export function sameLines(eventLines: string[], docLines: string[]): boolean {
  return eventLines.length === docLines.length && eventLines.every((l, i) => sameLine(l, docLines[i]));
}

/** Moves fragments computed on `targetLines` (the text an edit event
 *  describes) onto `docLines` (the text the editor holds). Identity when the
 *  two match; otherwise a fragment keeps only the lines the document still
 *  has, split where the document differs. */
export function mapFragmentsToDocument(
  fragments: LineFragment[],
  targetLines: string[],
  docLines: string[],
): LineFragment[] {
  if (sameLines(targetLines, docLines)) return fragments;
  const docLineOf = new Array<number>(targetLines.length).fill(-1);
  let t = 0;
  let d = 0;
  for (const change of diff.diffArrays(targetLines, docLines, { comparator: sameLine })) {
    const len = change.count ?? change.value.length;
    if (change.added) {
      d += len;
    } else if (change.removed) {
      t += len;
    } else {
      for (let i = 0; i < len; i++) docLineOf[t + i] = d + i;
      t += len;
      d += len;
    }
  }
  const out: LineFragment[] = [];
  for (const f of fragments) {
    let run: LineFragment | null = null;
    for (let line = f.startLine; line < Math.min(f.endLine, targetLines.length); line++) {
      const mapped = docLineOf[line];
      if (mapped >= 0 && run && run.endLine === mapped) {
        run.endLine = mapped + 1;
        continue;
      }
      if (run) out.push(run);
      run = mapped >= 0 ? { startLine: mapped, endLine: mapped + 1, kind: f.kind } : null;
    }
    if (run) out.push(run);
  }
  return out;
}
