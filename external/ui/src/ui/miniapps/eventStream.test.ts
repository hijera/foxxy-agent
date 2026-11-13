import { expect, test, vi } from "vitest";
import { readMiniAppEventStream } from "./api";
import type { MiniAppRunEvent } from "./types";

function streamOf(text: string): ReadableStream<Uint8Array> {
  return new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(new TextEncoder().encode(text));
      controller.close();
    },
  });
}

function responseOf(text: string): Response {
  return { ok: true, status: 200, body: streamOf(text) } as unknown as Response;
}

const frame = (seq: number, type: string) =>
  `id: ${seq}\ndata: ${JSON.stringify({ seq, type, status: "running" })}\n\n`;

test("run events are read over fetch and the done frame ends the stream", async () => {
  const seen: MiniAppRunEvent[] = [];
  const controller = new AbortController();
  const fetchImpl = vi.fn(async (_input: RequestInfo | URL) =>
    responseOf(
      frame(1, "run.started") +
        frame(2, "step.succeeded") +
        `event: done\ndata: [DONE]\n\n`,
    ),
  );

  await readMiniAppEventStream("run", "run_1", (event) => seen.push(event), {
    signal: controller.signal,
    fetchImpl: fetchImpl as unknown as typeof fetch,
    sleep: async () => {},
  });

  expect(seen.map((event) => event.type)).toEqual([
    "run.started",
    "step.succeeded",
  ]);
  // EventSource cannot carry the Authorization header the remote-environment
  // shim injects, so this stream must go through fetch like every other one.
  expect(fetchImpl).toHaveBeenCalledTimes(1);
  expect(String(fetchImpl.mock.calls[0]?.[0])).toContain(
    "/foxxycode/miniapp-runs/run_1/events",
  );
});

test("a dropped stream reconnects after the last frame it saw", async () => {
  const seen: MiniAppRunEvent[] = [];
  const controller = new AbortController();
  const urls: string[] = [];
  const fetchImpl = vi.fn(async (input: unknown) => {
    urls.push(String(input));
    if (urls.length === 1) return responseOf(frame(1, "run.started"));
    controller.abort();
    return responseOf(frame(2, "run.succeeded") + `event: done\ndata: [DONE]\n\n`);
  });

  await readMiniAppEventStream("run", "run_1", (event) => seen.push(event), {
    signal: controller.signal,
    fetchImpl: fetchImpl as unknown as typeof fetch,
    sleep: async () => {},
  });

  expect(seen.map((event) => event.type)).toEqual([
    "run.started",
    "run.succeeded",
  ]);
  expect(urls[0]).toContain("after=0");
  expect(urls[1]).toContain("after=1");
});

test("distillation jobs stream from their own route", async () => {
  const controller = new AbortController();
  const fetchImpl = vi.fn(async (_input: RequestInfo | URL) => {
    controller.abort();
    return responseOf(`event: done\ndata: [DONE]\n\n`);
  });

  await readMiniAppEventStream("distillation", "job_1", () => undefined, {
    signal: controller.signal,
    fetchImpl: fetchImpl as unknown as typeof fetch,
    sleep: async () => {},
  });

  expect(String(fetchImpl.mock.calls[0]?.[0])).toContain(
    "/foxxycode/miniapp-distillations/job_1/events",
  );
});

test("a malformed frame is skipped without ending the stream", async () => {
  const seen: MiniAppRunEvent[] = [];
  const controller = new AbortController();
  const fetchImpl = vi.fn(async () =>
    responseOf(
      `id: 1\ndata: {not json\n\n` +
        frame(2, "run.succeeded") +
        `event: done\ndata: [DONE]\n\n`,
    ),
  );

  await readMiniAppEventStream("run", "run_1", (event) => seen.push(event), {
    signal: controller.signal,
    fetchImpl: fetchImpl as unknown as typeof fetch,
    sleep: async () => {},
  });

  expect(seen.map((event) => event.type)).toEqual(["run.succeeded"]);
});
