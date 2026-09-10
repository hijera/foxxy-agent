import { afterEach, expect, test, vi } from "vitest";
import {
  consumeComposerSseReader,
  sessionModeFromEvent,
  type ConsumeComposerSseParams,
} from "./consumeComposerSse";

afterEach(() => vi.unstubAllGlobals());

function mockReader(text: string): ReadableStreamDefaultReader<Uint8Array> {
  const chunks = [new TextEncoder().encode(text)];
  let i = 0;
  return {
    read: async () =>
      i < chunks.length
        ? { done: false, value: chunks[i++]! }
        : { done: true, value: undefined },
    cancel: async () => {},
    releaseLock: () => {},
    closed: Promise.resolve(undefined),
  } as unknown as ReadableStreamDefaultReader<Uint8Array>;
}

async function drive(sse: string): Promise<string[]> {
  vi.stubGlobal("requestAnimationFrame", () => 0);
  const seen: string[] = [];
  const params: ConsumeComposerSseParams = {
    reader: mockReader(sse),
    dec: new TextDecoder(),
    carry: { buf: "" },
    assistantId: "a-init",
    applyStreamItems: () => {},
    setTokenUsage: () => {},
    setContextUsage: () => {},
    tokenBaselineRef: { current: { input: 0, output: 0, total: 0 } },
    reasoningDurationMsByContentRef: { current: new Map() },
    newId: (p) => `${p}-0`,
    applyMemoryPhaseToItems: (prev) => prev,
    applyMemoryChunkToItems: (prev) => prev,
    onModeChanged: (mode) => seen.push(mode),
  };
  const res = await consumeComposerSseReader(params);
  res.flushToolQueue();
  return seen;
}

function modeFrame(currentModeId: unknown): string {
  return `event: mode\ndata: ${JSON.stringify({
    sessionUpdate: "current_mode_update",
    currentModeId,
  })}\n\n`;
}

test("a mode frame reports the profile the backend switched to", async () => {
  expect(await drive(modeFrame("agent") + "data: [DONE]\n\n")).toEqual([
    "agent",
  ]);
});

test("a mode frame left unterminated by a dropped stream is still read", async () => {
  // The turn ended without [DONE] (a dropped connection); the last frame sits
  // in the carry buffer and is only seen by the tail parse.
  const unterminated =
    'event: mode\ndata: {"sessionUpdate":"current_mode_update","currentModeId":"agent"}';
  expect(await drive(unterminated)).toEqual(["agent"]);
});

test("a mode frame with no usable id is ignored", async () => {
  expect(await drive(modeFrame("") + "data: [DONE]\n\n")).toEqual([]);
  expect(await drive(modeFrame(42) + "data: [DONE]\n\n")).toEqual([]);
  expect(await drive("event: mode\ndata: not json\n\ndata: [DONE]\n\n")).toEqual(
    [],
  );
});

test("sessionModeFromEvent trims and rejects non-string payloads", () => {
  expect(sessionModeFromEvent(`{"currentModeId":"  plan  "}`)).toBe("plan");
  expect(sessionModeFromEvent(`{"currentModeId":null}`)).toBe("");
  expect(sessionModeFromEvent("{")).toBe("");
});
