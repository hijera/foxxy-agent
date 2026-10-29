import { afterEach, describe, expect, it, vi } from "vitest";

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.resetModules();
});

describe("remote probe deadline", () => {
  it("aborts an unanswered request without AbortSignal.timeout", async () => {
    vi.useFakeTimers();
    vi.stubGlobal("AbortSignal", {});
    let signal: AbortSignal | undefined;
    vi.stubGlobal("fetch", (_: unknown, init: RequestInit) => {
      signal = init.signal as AbortSignal;
      return new Promise((_, reject) => {
        signal!.addEventListener("abort", () => reject(new Error("aborted")));
      });
    });
    const { probeRemote } = await import("./remoteEnv");
    const pending = expect(
      probeRemote("http://remote/v1/models", {}, 100),
    ).rejects.toThrow("aborted");
    await vi.advanceTimersByTimeAsync(100);
    await pending;
    expect(signal?.aborted).toBe(true);
    expect(vi.getTimerCount()).toBe(0);
  });

  it("clears its deadline as soon as the remote answers", async () => {
    vi.useFakeTimers();
    let signal: AbortSignal | undefined;
    vi.stubGlobal("fetch", (_: unknown, init: RequestInit) => {
      signal = init.signal as AbortSignal;
      return Promise.resolve(new Response("", { status: 200 }));
    });
    const { probeRemote } = await import("./remoteEnv");
    const response = await probeRemote(
      "http://remote/v1/models",
      { Authorization: "Bearer test" },
      100,
    );
    expect(response.status).toBe(200);
    expect(vi.getTimerCount()).toBe(0);
    await vi.advanceTimersByTimeAsync(100);
    expect(signal?.aborted).toBe(false);
  });
});
