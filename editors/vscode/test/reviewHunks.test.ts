import { expect, test } from "vitest";
import { reviewHunks, moveReviewHunks, reviewedTextMatches } from "../src/diff/reviewHunks";

test("rejecting one replacement keeps the other block and unrelated editor changes", () => {
  const after = "new one\nkeep\nnew two\n";
  const [one, two] = reviewHunks("old one\nkeep\nold two\n", after);
  expect(after.slice(0, one.start) + one.before + after.slice(one.end)).toBe("old one\nkeep\nnew two\n");
  const prefix = "// mine\n";
  const moved = moveReviewHunks([two], [{ start: 0, end: 0, text: prefix }])[0];
  expect(reviewedTextMatches(moved, prefix + after)).toBe(true);
  expect(moved.start).toBe(two.start + prefix.length);
});

test("invalidates only blocks touched by later edits", () => {
  const [one, two] = reviewHunks("old one\nkeep\nold two\n", "new one\nkeep\nnew two\n");
  expect(moveReviewHunks([one, two], [{ start: 1, end: 3, text: "x" }]))
    .toEqual([{ ...two, start: two.start - 1, end: two.end - 1 }]);
  expect(reviewedTextMatches(one, "NEW one\nkeep\nnew two\n")).toBe(false);
});

test("restores a deletion at EOF and retains CRLF and the final newline", () => {
  const [removed] = reviewHunks("a\r\nb\r\n", "a\r\n");
  expect(removed).toMatchObject({ start: 3, end: 3, before: "b\r\n", after: "" });
  expect(moveReviewHunks([removed], [{ start: 3, end: 3, text: "mine" }])).toEqual([]);
  expect(reviewHunks("a", "a\n")[0]).toMatchObject({ before: "a", after: "a\n" });
});
