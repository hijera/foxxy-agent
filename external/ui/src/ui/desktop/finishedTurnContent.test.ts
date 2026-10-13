import { expect, test } from "vitest";
import { finishedTurnContent } from "./finishedTurnContent";
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
