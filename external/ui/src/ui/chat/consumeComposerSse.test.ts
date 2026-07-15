import { describe, it, expect } from "vitest";
import { consumeComposerSseReader } from "./consumeComposerSse";
import type { TranscriptItem } from "./types";

function readerFromChunks(
  chunks: string[],
): ReadableStreamDefaultReader<Uint8Array> {
  const enc = new TextEncoder();
  let i = 0;
  return new ReadableStream<Uint8Array>({
    pull(controller) {
      if (i < chunks.length) {
        controller.enqueue(enc.encode(chunks[i++]!));
      } else {
        controller.close();
      }
    },
  }).getReader();
}

function baseParams(reader: ReadableStreamDefaultReader<Uint8Array>) {
  let items: TranscriptItem[] = [];
  return {
    reader,
    dec: new TextDecoder(),
    carry: { buf: "" },
    assistantId: "a1",
    applyStreamItems: (fn: (prev: TranscriptItem[]) => TranscriptItem[]) => {
      items = fn(items);
    },
    setTokenUsage: () => {},
    tokenBaselineRef: { current: { input: 0, output: 0, total: 0 } },
    reasoningDurationMsByContentRef: { current: new Map<string, number>() },
    newId: (p: string) => `${p}-${Math.random().toString(36).slice(2)}`,
    applyMemoryPhaseToItems: (prev: TranscriptItem[]) => prev,
    applyMemoryChunkToItems: (prev: TranscriptItem[]) => prev,
    getItems: () => items,
  };
}

const contentEvent = 'data: {"choices":[{"delta":{"content":"hi"}}]}\n\n';

describe("consumeComposerSseReader endedWithoutDone", () => {
  it("is false when the stream terminates with [DONE]", async () => {
    const p = baseParams(readerFromChunks([contentEvent, "data: [DONE]\n\n"]));
    const res = await consumeComposerSseReader(p);
    expect(res.endedWithoutDone).toBe(false);
    expect(res.streamErrorMessage).toBeNull();
  });

  it("is true when the reader closes before [DONE] (cut mid-turn)", async () => {
    const p = baseParams(readerFromChunks([contentEvent]));
    const res = await consumeComposerSseReader(p);
    expect(res.endedWithoutDone).toBe(true);
    expect(res.streamErrorMessage).toBeNull();
  });

  it("is false when the stream reports an error", async () => {
    const errEvent = 'data: {"error":{"message":"boom"}}\n\n';
    const p = baseParams(readerFromChunks([errEvent]));
    const res = await consumeComposerSseReader(p);
    expect(res.endedWithoutDone).toBe(false);
    expect(res.streamErrorMessage).toBeTruthy();
  });
});
