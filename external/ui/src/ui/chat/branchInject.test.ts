import { expect, test } from "vitest";
import { injectBranchNavItems } from "./branchInject";
import type { TranscriptItem } from "./types";

function makeUserMsg(id: string, content = "hello"): TranscriptItem {
  return { id, type: "user_message", content };
}

function makeAssistMsg(id: string): TranscriptItem {
  return { id, type: "assistant_message", content: "ok" };
}

test("returns items unchanged when no branch points", () => {
  const items: TranscriptItem[] = [makeUserMsg("u1"), makeAssistMsg("a1")];
  const result = injectBranchNavItems(items, []);
  expect(result).toBe(items);
});

test("injects branch_nav right after first user message (index 0)", () => {
  const items: TranscriptItem[] = [
    makeUserMsg("u1"),
    makeAssistMsg("a1"),
    makeUserMsg("u2"),
  ];
  const bp = {
    userMessageIndex: 0,
    currentIndex: 1,
    total: 3,
    sessions: [{ sessionId: "s1" }, { sessionId: "s2" }, { sessionId: "s3" }],
  };
  const result = injectBranchNavItems(items, [bp]);
  expect(result).toHaveLength(4);
  expect(result[0]).toEqual(items[0]); // user_message at idx 0
  expect(result[1]?.type).toBe("branch_nav");
  expect(result[1]).toMatchObject({ type: "branch_nav", currentIndex: 1, total: 3 });
  expect(result[2]).toEqual(items[1]); // assistant
  expect(result[3]).toEqual(items[2]); // second user message
});

test("injects branch_nav after second user message (index 1)", () => {
  const items: TranscriptItem[] = [
    makeUserMsg("u1"),
    makeAssistMsg("a1"),
    makeUserMsg("u2"),
    makeAssistMsg("a2"),
  ];
  const bp = {
    userMessageIndex: 1,
    currentIndex: 0,
    total: 2,
    sessions: [{ sessionId: "orig" }, { sessionId: "branch" }],
  };
  const result = injectBranchNavItems(items, [bp]);
  expect(result).toHaveLength(5);
  expect(result[3]?.type).toBe("branch_nav");
  expect((result[3] as any).userMessageIndex).toBe(1);
});

test("injects at multiple branch points", () => {
  const items: TranscriptItem[] = [
    makeUserMsg("u1"),
    makeAssistMsg("a1"),
    makeUserMsg("u2"),
    makeAssistMsg("a2"),
  ];
  const bps = [
    { userMessageIndex: 0, currentIndex: 0, total: 2, sessions: [{ sessionId: "s1" }, { sessionId: "s2" }] },
    { userMessageIndex: 1, currentIndex: 1, total: 2, sessions: [{ sessionId: "s3" }, { sessionId: "s4" }] },
  ];
  const result = injectBranchNavItems(items, bps);
  expect(result).toHaveLength(6);
  const navItems = result.filter((i) => i.type === "branch_nav");
  expect(navItems).toHaveLength(2);
});
