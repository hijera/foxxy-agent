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
