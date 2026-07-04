import { describe, it, expect } from "vitest";
import { computeLineFragments } from "../src/diff/lineFragments";

describe("computeLineFragments", () => {
  it("reports added lines on useAfterRanges", () => {
    const before = "a\nb\nc";
    const after = "a\nb\nNEW\nc";
    const frags = computeLineFragments(before, after, true);
    // `after` lines: 0=a, 1=b, 2=NEW, 3=c. The added range should be [2,3).
    expect(frags).toEqual([{ startLine: 2, endLine: 3, kind: "add" }]);
  });

  it("reports deleted lines when useAfterRanges is false", () => {
    const before = "a\nb\nOLD\nc";
    const after = "a\nb\nc";
    const frags = computeLineFragments(before, after, false);
    // `before` lines: 0=a, 1=b, 2=OLD, 3=c. The deleted range should be [2,3).
    expect(frags).toEqual([{ startLine: 2, endLine: 3, kind: "del" }]);
  });

  it("skips deletions when useAfterRanges is true", () => {
    const before = "a\nb\nOLD\nc";
    const after = "a\nb\nc";
    const frags = computeLineFragments(before, after, true);
    expect(frags).toEqual([]);
  });

  it("returns no fragments for identical content", () => {
    const s = "x\ny\nz";
    expect(computeLineFragments(s, s, true)).toEqual([]);
    expect(computeLineFragments(s, s, false)).toEqual([]);
  });
});
