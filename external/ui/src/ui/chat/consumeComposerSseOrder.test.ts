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

function harness(reader: ReadableStreamDefaultReader<Uint8Array>) {
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
    newId: (p: string) => `${p}${++seq}`,
    applyMemoryPhaseToItems: (prev: TranscriptItem[]) => prev,
    applyMemoryChunkToItems: (prev: TranscriptItem[]) => prev,
    getItems: () => items,
  };
}

let seq = 0;

const text = (s: string) =>
  `data: ${JSON.stringify({ choices: [{ delta: { content: s } }] })}\n\n`;
const reasoning = (s: string) =>
  `data: ${JSON.stringify({ choices: [{ delta: { reasoning_content: s } }] })}\n\n`;
const toolCall = (id: string) =>
  `event: tool_call\ndata: ${JSON.stringify({ toolCallId: id, title: "edit", status: "pending" })}\n\n`;
const DONE = "data: [DONE]\n\n";

describe("consumeComposerSseReader chronological ordering", () => {
  it("keeps thinking, text and actions in arrival order", async () => {
    seq = 0;
    const p = harness(
      readerFromChunks([
        reasoning("pondering"),
        text("I will edit the file:"),
        toolCall("call_1"),
        text("Done, it now compiles."),
        DONE,
      ]),
    );
    await consumeComposerSseReader(p);

    expect(p.getItems().map((x) => x.type)).toEqual([
      "thinking",
      "assistant_message",
      "tool_call",
      "assistant_message",
    ]);
  });

  it("puts text after an action into a new bubble below it", async () => {
    seq = 0;
    const p = harness(
      readerFromChunks([
        text("before"),
        toolCall("call_1"),
        text("after"),
        DONE,
      ]),
    );
    await consumeComposerSseReader(p);

    const items = p.getItems();
    const bubbles = items.filter(
      (x): x is Extract<TranscriptItem, { type: "assistant_message" }> =>
        x.type === "assistant_message",
    );
    expect(bubbles).toHaveLength(2);
    expect(bubbles[0]?.content).toBe("before");
    expect(bubbles[1]?.content).toBe("after");
    // the action sits between them, not above both
    expect(items.map((x) => x.type)).toEqual([
      "assistant_message",
      "tool_call",
      "assistant_message",
    ]);
  });

  it("does not leave an empty bubble when the turn ends on an action", async () => {
    seq = 0;
    const p = harness(
      readerFromChunks([text("running it"), toolCall("call_1"), DONE]),
    );
    const res = await consumeComposerSseReader(p);
    // Callers drain the RAF-batched tool queue once the stream ends (see App.tsx).
    res.flushToolQueue();

    expect(p.getItems().map((x) => x.type)).toEqual([
      "assistant_message",
      "tool_call",
    ]);
    // the live bubble stays the one that actually holds text
    expect(res.finalAssistantId).toBe("a1");
  });

  it("merges consecutive deltas into a single bubble", async () => {
    seq = 0;
    const p = harness(readerFromChunks([text("Hello "), text("world"), DONE]));
    await consumeComposerSseReader(p);

    const bubbles = p
      .getItems()
      .filter(
        (x): x is Extract<TranscriptItem, { type: "assistant_message" }> =>
          x.type === "assistant_message",
      );
    expect(bubbles).toHaveLength(1);
    expect(bubbles[0]?.content).toBe("Hello world");
  });
});
