import { expect, test } from "vitest";
import { finishedTurnContent, plainTurnExcerpt } from "./finishedTurnContent";
import type { TranscriptItem } from "../chat/types";

test("old plans and answers do not leak into a background-woken turn's notification", () => {
  const rows: TranscriptItem[] = [
    { id: "u", type: "user_message", content: "Old request" },
    {
      id: "p",
      type: "plan_document",
      slug: "old",
      name: "Old",
      overview: "",
      content: "",
      expanded: false,
    },
    { id: "a", type: "assistant_message", content: "Old answer" },
    { id: "wake", type: "background_wake", tasks: [] },
    {
      id: "new",
      type: "assistant_message",
      content: "Task results",
      createdAtUtc: "2026-10-13T06:00:00Z",
    },
  ];
  const result = finishedTurnContent(rows);
  expect(result.plans).toEqual([]);
  expect(result.reply).toEqual(rows[4]);
});

test("notification excerpts carry plain text, not markdown syntax", () => {
  expect(
    plainTurnExcerpt(
      "## Summary\nDone. See **the docs** and [the guide](https://example.com).\n- fixed `parser`\n> quoted",
    ),
  ).toBe("Summary\nDone. See the docs and the guide.\nfixed parser\nquoted");
});

test("notification excerpts keep fenced code as text without markers", () => {
  expect(plainTurnExcerpt("before\n```go\nfmt.Println(1)\n```\nafter")).toBe(
    "before\nfmt.Println(1)\nafter",
  );
});

test("notification excerpts cap on a word boundary", () => {
  const text = "word ".repeat(100).trim();
  const excerpt = plainTurnExcerpt(text, 50);
  expect(excerpt.length).toBeLessThanOrEqual(51);
  expect(excerpt.endsWith("…")).toBe(true);
  expect(excerpt.slice(0, -1)).not.toMatch(/\s$/);
});
