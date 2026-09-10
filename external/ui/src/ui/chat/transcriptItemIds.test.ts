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

test("preserveTranscriptItemIds keeps a locally known todoPlan when the server row lacks it", () => {
  const todoPlan = [
    { content: "Inspect cards", status: "completed" },
    { content: "Render preview", status: "in_progress" },
  ];
  const prev: TranscriptItem[] = [
    {
      id: "tc_keep",
      type: "tool_call",
      toolCallId: "call_1",
      status: "in_progress",
      todoPlan,
    },
  ];
  const merged: TranscriptItem[] = [
    {
      id: "tc_new",
      type: "tool_call",
      toolCallId: "call_1",
      status: "completed",
      resultText: "updated item 1",
    },
  ];
  const out = preserveTranscriptItemIds(merged, prev);
  expect(out[0]?.id).toBe("tc_keep");
  expect(out[0]?.type === "tool_call" && out[0].status).toBe("completed");
  expect(out[0]?.type === "tool_call" && out[0].todoPlan).toEqual(todoPlan);
});

test("preserveTranscriptItemIds prefers the server todoPlan when the server row carries one", () => {
  const prev: TranscriptItem[] = [
    {
      id: "tc_keep",
      type: "tool_call",
      toolCallId: "call_1",
      status: "completed",
      todoPlan: [{ content: "stale", status: "pending" }],
    },
  ];
  const merged: TranscriptItem[] = [
    {
      id: "tc_new",
      type: "tool_call",
      toolCallId: "call_1",
      status: "completed",
      todoPlan: [{ content: "fresh", status: "completed" }],
    },
  ];
  const out = preserveTranscriptItemIds(merged, prev);
  expect(out[0]?.type === "tool_call" && out[0].todoPlan).toEqual([
    { content: "fresh", status: "completed" },
  ]);
});

test("preserveTranscriptItemIds treats an empty server todoPlan as missing", () => {
  const prev: TranscriptItem[] = [
    {
      id: "tc_keep",
      type: "tool_call",
      toolCallId: "call_1",
      status: "completed",
      todoPlan: [{ content: "known", status: "pending" }],
    },
  ];
  const merged: TranscriptItem[] = [
    {
      id: "tc_new",
      type: "tool_call",
      toolCallId: "call_1",
      status: "completed",
      todoPlan: [],
    },
  ];
  const out = preserveTranscriptItemIds(merged, prev);
  expect(out[0]?.type === "tool_call" && out[0].todoPlan).toEqual([
    { content: "known", status: "pending" },
  ]);
});

test("preserveTranscriptItemIds does not borrow todoPlan from a different tool call", () => {
  const prev: TranscriptItem[] = [
    {
      id: "tc_keep",
      type: "tool_call",
      toolCallId: "call_1",
      status: "completed",
      todoPlan: [{ content: "known", status: "pending" }],
    },
  ];
  const merged: TranscriptItem[] = [
    {
      id: "tc_new",
      type: "tool_call",
      toolCallId: "call_2",
      status: "completed",
    },
  ];
  const out = preserveTranscriptItemIds(merged, prev);
  expect(out[0]?.id).toBe("tc_new");
  expect(out[0]?.type === "tool_call" && out[0].todoPlan).toBeUndefined();
});
