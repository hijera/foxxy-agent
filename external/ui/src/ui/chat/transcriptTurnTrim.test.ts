import { describe, it, expect } from "vitest";
import {
  createTurnReplayTrimGate,
  trimTranscriptForTurnReplay,
} from "./transcriptTurnTrim";
import type { TranscriptItem } from "./types";

const user = (id: string): TranscriptItem => ({
  id,
  type: "user_message",
  content: "go",
});

const assistant = (id: string, streaming?: boolean): TranscriptItem => ({
  id,
  type: "assistant_message",
  content: "partial text:",
  ...(streaming !== undefined ? { streaming } : {}),
});

const tool = (id: string): TranscriptItem => ({
  id,
  type: "tool_call",
  toolCallId: `${id}_tc`,
  status: "pending",
});

describe("trimTranscriptForTurnReplay", () => {
  it("drops partial turn output after the last user message", () => {
    const items = [
      user("u1"),
      assistant("a1"),
      user("u2"),
      assistant("a2"),
      tool("t1"),
    ];
    const out = trimTranscriptForTurnReplay(items);
    expect(out.map((x) => x.id)).toEqual(["u1", "a1", "u2"]);
  });

  it("keeps branch_nav rows attached to the last user message", () => {
    const nav: TranscriptItem = {
      id: "bn1",
      type: "branch_nav",
      userMessageIndex: 1,
      currentIndex: 0,
      total: 2,
      sessions: [],
    };
    const items = [user("u1"), nav, assistant("a1"), tool("t1")];
    const out = trimTranscriptForTurnReplay(items);
    expect(out.map((x) => x.id)).toEqual(["u1", "bn1"]);
  });

  it("returns a copy unchanged when there is no user message", () => {
    const items = [assistant("a1")];
    const out = trimTranscriptForTurnReplay(items);
    expect(out).not.toBe(items);
    expect(out.map((x) => x.id)).toEqual(["a1"]);
  });

  it("keeps everything when the user message is last", () => {
    const items = [user("u1"), assistant("a1"), user("u2")];
    const out = trimTranscriptForTurnReplay(items);
    expect(out.map((x) => x.id)).toEqual(["u1", "a1", "u2"]);
  });
});

describe("createTurnReplayTrimGate", () => {
  const inFlight = [user("u1"), assistant("a1"), tool("t1")];

  it("trims the first mutation and passes later ones through", () => {
    const gate = createTurnReplayTrimGate();
    expect(gate(inFlight).map((x) => x.id)).toEqual(["u1"]);
    // The replay has taken over; its own rows must survive untouched.
    const replayed = [user("u1"), assistant("a2"), tool("t2")];
    expect(gate(replayed).map((x) => x.id)).toEqual(["u1", "a2", "t2"]);
  });

  it("leaves the transcript alone when the relay never sends anything", () => {
    createTurnReplayTrimGate();
    expect(inFlight.map((x) => x.id)).toEqual(["u1", "a1", "t1"]);
  });
});
