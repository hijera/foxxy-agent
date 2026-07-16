import { describe, expect, it } from "vitest";
import {
  appendDeferredAssistant,
  deferredAssistantItem,
  emptyDeferredAssistant,
} from "./transcriptDeferredAssistant";
import type { TranscriptItem } from "./types";

describe("deferred assistant turn reconstruction", () => {
  it("keeps all ReAct text in one assistant row after thinking and tools", () => {
    let pending = emptyDeferredAssistant();
    const rows: TranscriptItem[] = [
      {
        id: "th_1_0",
        type: "thinking",
        status: "completed",
        content: "first reasoning",
      },
    ];

    pending = appendDeferredAssistant(
      pending,
      "I will inspect. ",
      "2026-07-16T10:00:00Z",
    );
    rows.push({
      id: "tc_1",
      type: "tool_call",
      toolCallId: "call_1",
      status: "completed",
    });
    rows.push({
      id: "th_1_1",
      type: "thinking",
      status: "completed",
      content: "second reasoning",
    });
    pending = appendDeferredAssistant(
      pending,
      "Here is the result.",
      "2026-07-16T10:00:01Z",
    );

    const assistant = deferredAssistantItem(pending, 1);
    if (assistant) rows.push(assistant);

    expect(rows.map((row) => row.type)).toEqual([
      "thinking",
      "tool_call",
      "thinking",
      "assistant_message",
    ]);
    expect(rows.at(-1)).toMatchObject({
      id: "as_1",
      type: "assistant_message",
      content: "I will inspect. Here is the result.",
      createdAtUtc: "2026-07-16T10:00:01Z",
    });
  });

  it("does not create an empty assistant row", () => {
    expect(deferredAssistantItem(emptyDeferredAssistant(), 1)).toBeNull();
  });
});
