import { expect, test } from "vitest";

import {
  mergeStoredQuestionPromptsIntoTranscript,
  pickRicherQuestionToolArgs,
} from "./questionPromptSessionStore";
import type { FoxxyCodeQuestionPayload } from "./questionTypes";
import type { TranscriptItem } from "./types";

test("pickRicherQuestionToolArgs prefers the JSON with more parsed questions", () => {
  const full = JSON.stringify({
    questions: [
      { question: "A?", options: [{ label: "1" }] },
      { question: "B?", options: [{ label: "2" }] },
    ],
  });
  const truncated = '{"questions":[]}';
  expect(pickRicherQuestionToolArgs(full, truncated)).toBe(full);
  expect(pickRicherQuestionToolArgs(truncated, full)).toBe(full);
});

test("mergeStoredQuestionPromptsIntoTranscript inserts after matching tool_call", () => {
  const payload: FoxxyCodeQuestionPayload = {
    sessionId: "sess_x",
    requestId: "q_123",
    toolCallId: "call_abc",
    questions: [{ question: "Hi?", options: [{ label: "Yes" }] }],
  };

  try {
    window.localStorage.setItem(
      "foxxycode_qp_v1:sess_x",
      JSON.stringify([
        {
          requestId: "q_123",
          payload,
          resolved: {
            skipped: false,
            answers: [["Yes"]],
            summaryLine: "Hi? Yes",
          },
        },
      ]),
    );
  } catch {
    return;
  }

  const merged: TranscriptItem[] = [
    {
      id: "t1",
      type: "tool_call",
      toolCallId: "call_abc",
      title: "question",
      status: "completed",
      argsText: "{}",
      resultText: "{}",
    },
  ];

  const out = mergeStoredQuestionPromptsIntoTranscript(merged, "sess_x");
  expect(out.findIndex((x) => x.type === "question_prompt")).toBe(1);
  const qp = out[1];
  expect(qp?.type === "question_prompt" ? qp.resolved?.summaryLine : "").toBe(
    "Hi? Yes",
  );
  try {
    window.localStorage.removeItem("foxxycode_qp_v1:sess_x");
  } catch {
    //
  }
});
