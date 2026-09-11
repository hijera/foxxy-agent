import { afterEach, describe, expect, it, vi } from "vitest";
import { fetchJSON } from "./fetchJSON";

afterEach(() => {
  vi.unstubAllGlobals();
});

function stubFetch(impl: (...args: unknown[]) => unknown): void {
  vi.stubGlobal("fetch", vi.fn(impl));
}

describe("fetchJSON", () => {
  it("returns the parsed body for a 200", async () => {
    stubFetch(async () => ({ ok: true, status: 200, json: async () => ({ a: 1 }) }));
    expect(await fetchJSON<{ a: number }>("/foxxycode/x")).toEqual({
      ok: true,
      status: 200,
      data: { a: 1 },
    });
  });

  it("reports a refused HTTP status without reading the body", async () => {
    stubFetch(async () => ({
      ok: false,
      status: 404,
      json: async () => {
        throw new Error("must not be read");
      },
    }));
    expect(await fetchJSON("/foxxycode/x")).toEqual({ ok: false, status: 404 });
  });

  // The regression: a background poll fires ~75 times a minute for the length of a
  // turn, so one dropped keep-alive connection is routine. It must not escape as an
  // unhandled rejection — the IDE panels paint any of those as a red error overlay.
  it("resolves offline instead of rejecting when the connection drops", async () => {
    stubFetch(async () => {
      throw new TypeError("Failed to fetch");
    });
    await expect(fetchJSON("/foxxycode/x")).resolves.toEqual({
      ok: false,
      status: 0,
    });
  });

  it("resolves offline when the body is cut off mid-read", async () => {
    stubFetch(async () => ({
      ok: true,
      status: 200,
      json: async () => {
        throw new TypeError("network error");
      },
    }));
    await expect(fetchJSON("/foxxycode/x")).resolves.toEqual({
      ok: false,
      status: 0,
    });
  });

  // A deliberate AbortController.abort() is the caller's own Stop, not an outage:
  // swallowing it as "offline" would make a cancelled request look like a failure.
  it("still rejects when the caller aborts the request", async () => {
    stubFetch(async () => {
      const err = new Error("The user aborted a request.");
      err.name = "AbortError";
      throw err;
    });
    await expect(fetchJSON("/foxxycode/x")).rejects.toThrow(/aborted/);
  });
});
