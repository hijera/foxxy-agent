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

describe("consumeComposerSseReader event ordering", () => {
  it("flushes a queued tool row before publishing its permission prompt", async () => {
    const toolEvent =
      'event: tool_call\ndata: {"toolCallId":"call_1","title":"read","status":"pending"}\n\n';
    const permissionEvent =
      'event: permission\ndata: {"sessionId":"sess_1","toolCall":{"toolCallId":"call_1"}}\n\n';
    const p = baseParams(
      readerFromChunks([toolEvent + permissionEvent + "data: [DONE]\n\n"]),
    );
    let orderAtPermission: string[] = [];

    await consumeComposerSseReader({
      ...p,
      onPermission: () => {
        orderAtPermission = p.getItems().map((item) => item.type);
      },
    });

    expect(orderAtPermission).toEqual(["tool_call"]);
  });
});

describe("design plan SSE event", () => {
  const designPlan =
    'event: plan\ndata: {"sessionUpdate":"plan","entries":[{"content":"Step","status":"pending"}],' +
    '"_meta":{"foxxycode.dev/planSlug":"my-plan","foxxycode.dev/planKind":"design"}}\n\n';

  it("reports the slug of a design plan update", async () => {
    const slugs: string[] = [];
    const p = baseParams(readerFromChunks([designPlan, "data: [DONE]\n\n"]));
    await consumeComposerSseReader({
      ...p,
      onDesignPlan: (slug: string) => slugs.push(slug),
    });
    expect(slugs).toEqual(["my-plan"]);
  });

  it("ignores todo-list plan updates that carry no design meta", async () => {
    const todoPlan =
      'event: plan\ndata: {"sessionUpdate":"plan","entries":[{"content":"Step","status":"pending"}]}\n\n';
    const slugs: string[] = [];
    const p = baseParams(readerFromChunks([todoPlan, "data: [DONE]\n\n"]));
    await consumeComposerSseReader({
      ...p,
      onDesignPlan: (slug: string) => slugs.push(slug),
    });
    expect(slugs).toEqual([]);
  });

  it("reports design plans that arrive in the unterminated tail", async () => {
    const slugs: string[] = [];
    const p = baseParams(readerFromChunks([designPlan.replace(/\n\n$/, "\n")]));
    await consumeComposerSseReader({
      ...p,
      onDesignPlan: (slug: string) => slugs.push(slug),
    });
    expect(slugs).toEqual(["my-plan"]);
  });
});
