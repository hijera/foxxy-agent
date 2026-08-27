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

// Typed as a plain tuple array rather than `as const`: a readonly union of
// literal tuples makes each row its own call signature, which no single
// callback shape satisfies.
const uncapableStates: Array<[boolean | null, string]> = [
  [null, "not-yet-probed"],
  [false, "HTTP surface unavailable"],
];

test.each(uncapableStates)(
  "hides create action when Mini Apps capability is %s (%s)",
  (linked) => {
    expect(
      isMiniAppSessionEligible({
        miniAppsHttpLinked: linked,
        editorEmbed: false,
        sessionId: "session-1",
        generating: false,
        items,
      }),
    ).toBe(false);
  },
);

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
