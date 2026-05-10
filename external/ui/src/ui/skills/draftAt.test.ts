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

test("atMenuDraft closes after dotted path once user types prose", () => {
  const s = "@http_todo_report.md asdf asdf zxcv";
  const caret = s.length;
  expect(atMenuDraftAtCaret(s, caret).open).toBe(false);
});

test("atMenuDraft stays open trailing space after file pick", () => {
  const s = "@http_todo_report.md ";
  const caret = s.length;
  expect(atMenuDraftAtCaret(s, caret)).toMatchObject({
    open: true,
    prefix: "http_todo_report.md ",
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
