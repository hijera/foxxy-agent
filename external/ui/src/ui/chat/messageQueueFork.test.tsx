import React from "react";
import { afterEach, describe, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { Composer } from "./Composer";
import {
  consumeComposerSseReader,
  type ConsumeComposerSseParams,
} from "./consumeComposerSse";
import { trimTranscriptForTurnReplay } from "./transcriptTurnTrim";
import type { TranscriptItem } from "./types";
import { setSendMode } from "../i18n/sendModeConfig";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  setSendMode("enter");
});

// The fork re-attaches to a running turn by trimming the loaded transcript back to
// the prompt the turn started from and letting the relay replay the rest. A turn
// that read follow-ups from the queue has more than one user message, and the
// replay sends those again where the turn read them.
describe("trimTranscriptForTurnReplay with queued follow-ups", () => {
  test("the cut goes after the prompt, not after a follow-up the replay brings back", () => {
    const items: TranscriptItem[] = [
      { id: "u0", type: "user_message", content: "an earlier turn" },
      { id: "a0", type: "assistant_message", content: "earlier answer" },
      { id: "u1", type: "user_message", content: "start the work" },
      { id: "a1", type: "assistant_message", content: "reading" },
      { id: "t1", type: "tool_call", toolCallId: "tc1", status: "completed" },
      {
        id: "u2",
        type: "user_message",
        content: "check the Windows path too",
        queued: true,
      },
      { id: "a2", type: "assistant_message", content: "partial" },
    ];
    expect(trimTranscriptForTurnReplay(items).map((x) => x.id)).toEqual([
      "u0",
      "a0",
      "u1",
    ]);
  });
});

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

async function drive(sse: string): Promise<TranscriptItem[]> {
  vi.stubGlobal("requestAnimationFrame", () => 0);
  const items: TranscriptItem[] = [];
  let idc = 0;
  const params: ConsumeComposerSseParams = {
    reader: mockReader(sse),
    dec: new TextDecoder(),
    carry: { buf: "" },
    assistantId: "a-init",
    applyStreamItems: (fn) => {
      const next = fn(items.slice());
      items.length = 0;
      items.push(...next);
    },
    setTokenUsage: () => {},
    setContextUsage: () => {},
    tokenBaselineRef: { current: { input: 0, output: 0, total: 0 } },
    reasoningDurationMsByContentRef: { current: new Map() },
    newId: (p) => `${p}-${idc++}`,
    applyMemoryPhaseToItems: (prev) => prev,
    applyMemoryChunkToItems: (prev) => prev,
  };
  const res = await consumeComposerSseReader(params);
  res.flushToolQueue();
  return items;
}

const text = (content: string) =>
  `data: ${JSON.stringify({ choices: [{ delta: { content } }] })}\n\n`;

describe("a follow-up read by the agent on the turn stream", () => {
  test("lands after the answer so far, and the next answer is a bubble of its own", async () => {
    const items = await drive(
      text("Reading the file.") +
        `event: user_message\ndata: ${JSON.stringify({ sessionUpdate: "user_message_chunk", content: { type: "text", text: "check the tests too" } })}\n\n` +
        text("The tests pass.") +
        "data: [DONE]\n\n",
    );
    expect(items.map((x) => x.type)).toEqual([
      "assistant_message",
      "user_message",
      "assistant_message",
    ]);
    const follow = items[1] as Extract<TranscriptItem, { type: "user_message" }>;
    expect(follow.content).toBe("check the tests too");
    expect(follow.queued).toBe(true);
    expect((items[0] as { content: string }).content).toBe("Reading the file.");
    expect((items[2] as { content: string }).content).toBe("The tests pass.");
  });
});

function renderGenerating(value: string, onQueue: (t: string) => void) {
  return render(
    <Composer
      value={value}
      isEmpty={false}
      mode="agent"
      modes={["agent", "plan"]}
      onModeChange={() => {}}
      onChange={() => {}}
      onSend={() => {}}
      generating={true}
      onStop={() => {}}
      queuedMessages={[]}
      onQueue={onQueue}
      onCancelQueued={() => {}}
    />,
  );
}

// Queueing is a send: it follows ui.send_mode like any other.
describe("the keyboard queues with the key that sends", () => {
  test("ctrl_enter: plain Enter is a newline, Ctrl+Enter queues", () => {
    setSendMode("ctrl_enter");
    const onQueue = vi.fn();
    renderGenerating("one more thing", onQueue);
    const box = screen.getByRole("textbox");

    fireEvent.keyDown(box, { key: "Enter" });
    expect(onQueue).not.toHaveBeenCalled();

    fireEvent.keyDown(box, { key: "Enter", ctrlKey: true });
    expect(onQueue).toHaveBeenCalledWith("one more thing");
  });

  test("off: the keyboard never queues, the button still does", () => {
    setSendMode("off");
    const onQueue = vi.fn();
    renderGenerating("one more thing", onQueue);

    fireEvent.keyDown(screen.getByRole("textbox"), { key: "Enter" });
    fireEvent.keyDown(screen.getByRole("textbox"), {
      key: "Enter",
      ctrlKey: true,
    });
    expect(onQueue).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Queue this message" }));
    expect(onQueue).toHaveBeenCalledWith("one more thing");
  });
});
