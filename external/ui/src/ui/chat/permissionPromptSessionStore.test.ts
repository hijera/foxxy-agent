import { afterEach, expect, test } from "vitest";

import {
  loadPermissionPromptRecords,
  mergePermissionPromptsIntoTranscript,
  upsertPermissionPromptRecord,
} from "./permissionPromptSessionStore";
import type { TranscriptItem } from "./types";

const SID = "sess_perm_store_test";

afterEach(() => {
  window.localStorage.removeItem(`foxxycode_pp_v1:${SID}`);
});

test("mergePermissionPromptsIntoTranscript restores from localStorage", () => {
  upsertPermissionPromptRecord(SID, {
    toolCallId: "call_store",
    payload: {
      sessionId: SID,
      toolCall: {
        toolCallId: "call_store",
        title: "Run: run_command",
        kind: "shell",
        content: [
          {
            type: "content",
            content: { type: "text", text: 'Arguments: {"command":"ls -la"}' },
          },
        ],
      },
      options: [
        { optionId: "allow", name: "Allow", kind: "allow_once" },
        { optionId: "reject", name: "Reject", kind: "reject_once" },
      ],
    },
  });
  const merged: TranscriptItem[] = [
    {
      type: "tool_call",
      id: "tc_1",
      toolCallId: "call_store",
      status: "pending",
      title: "run_command",
    },
  ];
  const out = mergePermissionPromptsIntoTranscript(merged, SID, {
    requirePermissionForCommands: true,
    requirePermissionForWrites: false,
    permissionMasterKey: false,
    commandAllowlist: [],
  });
  expect(out.some((x) => x.type === "permission_prompt")).toBe(true);
  expect(loadPermissionPromptRecords(SID)).toHaveLength(1);
});

test.each(["cancelled", "completed", "failed"] as const)(
  "a %s tool retires its stored and visible permission, including on reload",
  (status) => {
    const payload = {
      sessionId: SID,
      toolCall: { toolCallId: "call_finished", title: "Run: run_command" },
      options: [{ optionId: "allow", name: "Allow", kind: "allow_once" }],
    };
    upsertPermissionPromptRecord(SID, { toolCallId: "call_finished", payload });
    const tool: TranscriptItem = {
      type: "tool_call",
      id: "tc_finished",
      toolCallId: "call_finished",
      title: "run_command",
      status,
    };
    const out = mergePermissionPromptsIntoTranscript(
      [tool, { type: "permission_prompt", id: "pp_finished", payload }],
      SID,
      null,
    );
    expect(out).toEqual([tool]);
    expect(loadPermissionPromptRecords(SID).every((r) => r.resolved)).toBe(
      true,
    );
    expect(mergePermissionPromptsIntoTranscript([tool], SID, null)).toEqual([
      tool,
    ]);
  },
);
