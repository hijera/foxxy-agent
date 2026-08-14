import { expect, test } from "vitest";
import { isMiniAppSessionEligible } from "./sessionEligibility";
import type { TranscriptItem } from "../chat/types";

const items: TranscriptItem[] = [
  { id: "assistant", type: "assistant_message", content: "Done" },
  {
    id: "tool",
    type: "tool_call",
    toolCallId: "tool",
    status: "completed",
  },
];

test.each([
  [null, "not-yet-probed"],
  [false, "HTTP surface unavailable"],
] as const)("hides create action when Mini Apps capability is %s", (linked) => {
  expect(
    isMiniAppSessionEligible({
      miniAppsHttpLinked: linked,
      editorEmbed: false,
      sessionId: "session-1",
      generating: false,
      items,
    }),
  ).toBe(false);
});

test("allows a completed tool-driven session when the capability is linked", () => {
  expect(
    isMiniAppSessionEligible({
      miniAppsHttpLinked: true,
      editorEmbed: false,
      sessionId: "session-1",
      generating: false,
      items,
    }),
  ).toBe(true);
});
