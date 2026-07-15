import { describe, it, expect } from "vitest";
import { permissionPromptInsertIndex } from "./permissionPromptPlacement";
import type { TranscriptItem } from "./types";

const user = (id: string): TranscriptItem => ({
  id,
  type: "user_message",
  content: "do it",
});

const tool = (id: string, tcid: string): TranscriptItem => ({
  id,
  type: "tool_call",
  toolCallId: tcid,
  status: "pending",
});

const assistant = (
  id: string,
  content: string,
  streaming?: boolean,
): TranscriptItem => ({
  id,
  type: "assistant_message",
  content,
  ...(streaming !== undefined ? { streaming } : {}),
});

describe("permissionPromptInsertIndex", () => {
  it("inserts below the streaming bubble when the tool row sits above it (live order)", () => {
    // Live stream splices tool rows before the composing bubble:
    // [user, tool, streaming-bubble("I'll run it:")]
    const items = [user("u1"), tool("t1", "call_1"), assistant("a1", "I'll run it:", true)];
    expect(permissionPromptInsertIndex(items, "call_1")).toBe(3);
  });

  it("inserts right after the tool row when no streaming bubble exists (disk order)", () => {
    // Reload path: [user, assistant(text, not streaming), tool]
    const items = [user("u1"), assistant("a1", "I'll run it:"), tool("t1", "call_1")];
    expect(permissionPromptInsertIndex(items, "call_1")).toBe(3);
  });

  it("keeps the prompt after its tool row when the tool follows the bubble", () => {
    const items = [user("u1"), assistant("a1", "intro", true), tool("t1", "call_1")];
    expect(permissionPromptInsertIndex(items, "call_1")).toBe(3);
  });

  it("appends at the end when neither the tool row nor a streaming bubble exists", () => {
    const items = [user("u1"), assistant("a1", "done")];
    expect(permissionPromptInsertIndex(items, "call_missing")).toBe(2);
  });

  it("ignores non-streaming assistant bubbles from previous turns", () => {
    const items = [
      assistant("a0", "previous answer"),
      user("u1"),
      tool("t1", "call_1"),
    ];
    expect(permissionPromptInsertIndex(items, "call_1")).toBe(3);
  });
});
