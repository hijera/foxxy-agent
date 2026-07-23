import { describe, expect, it } from "vitest";

import { pinPlanDocumentsToTurnEnd } from "./planDocumentPlacement";
import type { TranscriptItem } from "./types";

function user(id: string): TranscriptItem {
  return { id, type: "user_message", content: `u-${id}` };
}

function assistant(id: string): TranscriptItem {
  return { id, type: "assistant_message", content: `a-${id}` };
}

function tool(id: string): TranscriptItem {
  return { id, type: "tool_call", toolCallId: id, status: "completed" };
}

function plan(id: string, slug = id): TranscriptItem {
  return {
    id,
    type: "plan_document",
    slug,
    name: slug,
    overview: "",
    content: "",
    expanded: false,
  };
}

function ids(items: TranscriptItem[]): string[] {
  return items.map((it) => it.id);
}

describe("pinPlanDocumentsToTurnEnd", () => {
  it("moves a mid-turn plan card below the assistant text of that turn", () => {
    const out = pinPlanDocumentsToTurnEnd([
      user("u1"),
      tool("t1"),
      plan("pd1"),
      assistant("a1"),
    ]);
    expect(ids(out)).toEqual(["u1", "t1", "a1", "pd1"]);
  });

  it("keeps each turn's plan card inside its own turn", () => {
    const out = pinPlanDocumentsToTurnEnd([
      user("u1"),
      plan("pd1"),
      assistant("a1"),
      user("u2"),
      plan("pd2"),
      assistant("a2"),
    ]);
    expect(ids(out)).toEqual(["u1", "a1", "pd1", "u2", "a2", "pd2"]);
  });

  it("preserves relative order of several plan cards in one turn", () => {
    const out = pinPlanDocumentsToTurnEnd([
      user("u1"),
      plan("pd1", "first"),
      assistant("a1"),
      plan("pd2", "second"),
      assistant("a2"),
    ]);
    expect(ids(out)).toEqual(["u1", "a1", "a2", "pd1", "pd2"]);
  });

  it("handles plan rows before the first user message", () => {
    const out = pinPlanDocumentsToTurnEnd([plan("pd1"), assistant("a1")]);
    expect(ids(out)).toEqual(["a1", "pd1"]);
  });

  it("returns the same reference when nothing has to move", () => {
    const items = [user("u1"), tool("t1"), assistant("a1"), plan("pd1")];
    expect(pinPlanDocumentsToTurnEnd(items)).toBe(items);
  });

  it("returns the same reference when there are no plan rows", () => {
    const items = [user("u1"), assistant("a1")];
    expect(pinPlanDocumentsToTurnEnd(items)).toBe(items);
  });

  it("returns the same reference for an empty transcript", () => {
    const items: TranscriptItem[] = [];
    expect(pinPlanDocumentsToTurnEnd(items)).toBe(items);
  });

  it("keeps trailing plan cards last while text keeps streaming in", () => {
    const out = pinPlanDocumentsToTurnEnd([
      user("u1"),
      tool("t1"),
      plan("pd1"),
      assistant("a1"),
      tool("t2"),
      assistant("a2"),
    ]);
    expect(ids(out)).toEqual(["u1", "t1", "a1", "t2", "a2", "pd1"]);
  });
});
