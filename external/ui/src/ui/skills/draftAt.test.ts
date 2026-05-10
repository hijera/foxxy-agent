import { expect, test } from "vitest";
import { atMenuDraftAtCaret, extractAtFileAttachments } from "./draftAt";

test("atMenuDraft detects path with spaces before caret", () => {
  const s = "x @readme here.md";
  const caret = s.length;
  const d = atMenuDraftAtCaret(s, caret);
  expect(d).toEqual({
    open: true,
    lineStart: 0,
    atIdx: 2,
    caret,
    prefix: "readme here.md",
  });
});

test("extractAtFileAttachments skips folders and dedupes", () => {
  const s = "see @a/b.txt and @a/ and @a/b.txt";
  expect(extractAtFileAttachments(s)).toEqual([{ path: "a/b.txt" }]);
});

test("atMenuDraft disabled inside fenced code", () => {
  const s = "```\n@\n```";
  const caret = s.indexOf("@") + 1;
  expect(atMenuDraftAtCaret(s, caret).open).toBe(false);
});
