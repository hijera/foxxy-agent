import { expect, test } from "vitest";
import { preserveTranscriptItemIds, stableToolCallItemId } from "./transcriptItemIds";
import type { TranscriptItem } from "./types";

test("stableToolCallItemId", () => {
  expect(stableToolCallItemId("abc")).toBe("tc_abc");
});

test("preserveTranscriptItemIds reuses id for matching tool_call", () => {
  const prev: TranscriptItem[] = [
    {
      id: "tc_keep",
      type: "tool_call",
      toolCallId: "call_1",
      status: "completed",
      resultText: "ok",
    },
  ];
  const merged: TranscriptItem[] = [
    {
      id: "tc_new",
      type: "tool_call",
      toolCallId: "call_1",
      status: "completed",
      resultText: "ok",
    },
  ];
  const out = preserveTranscriptItemIds(merged, prev);
  expect(out[0]?.id).toBe("tc_keep");
});

test("preserveTranscriptItemIds reuses id and expanded for a matching plan_document", () => {
  const prev: TranscriptItem[] = [
    {
      id: "pd_keep",
      type: "plan_document",
      slug: "my-plan",
      name: "My plan",
      overview: "old overview",
      content: "old",
      expanded: true,
    },
  ];
  const merged: TranscriptItem[] = [
    {
      id: "pd_new",
      type: "plan_document",
      slug: "my-plan",
      name: "My plan",
      overview: "new overview",
      content: "new",
      expanded: false,
    },
  ];
  const out = preserveTranscriptItemIds(merged, prev);
  expect(out[0]?.id).toBe("pd_keep");
  expect(out[0]?.type === "plan_document" && out[0].expanded).toBe(true);
  expect(out[0]?.type === "plan_document" && out[0].content).toBe("new");
});

test("preserveTranscriptItemIds does not match a different plan slug", () => {
  const prev: TranscriptItem[] = [
    {
      id: "pd_keep",
      type: "plan_document",
      slug: "plan-a",
      name: "A",
      overview: "",
      content: "",
      expanded: true,
    },
  ];
  const merged: TranscriptItem[] = [
    {
      id: "pd_new",
      type: "plan_document",
      slug: "plan-b",
      name: "B",
      overview: "",
      content: "",
      expanded: false,
    },
  ];
  const out = preserveTranscriptItemIds(merged, prev);
  expect(out[0]?.id).toBe("pd_new");
});
