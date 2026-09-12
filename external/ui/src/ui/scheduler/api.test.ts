import { afterEach, describe, expect, it, vi } from "vitest";
import {
  schedulerCancelJob,
  schedulerCreateJob,
  schedulerDeleteJob,
  schedulerGetJob,
  schedulerListJobs,
  schedulerPatchJob,
  schedulerPauseJob,
  schedulerResumeJob,
  schedulerRunJob,
} from "./api";

afterEach(() => {
  vi.unstubAllGlobals();
});

// The jobs list is polled every 12s while the scheduler drawer is open, so a
// backend that is down or restarting must produce a normal error result. An
// escaping rejection is what the IDE panels paint as a red error overlay.
describe("scheduler api when the backend is unreachable", () => {
  const calls: Array<[string, () => Promise<{ ok: boolean }>]> = [
    ["schedulerListJobs", () => schedulerListJobs()],
    ["schedulerGetJob", () => schedulerGetJob("j1")],
    ["schedulerCreateJob", () => schedulerCreateJob({} as never)],
    ["schedulerPatchJob", () => schedulerPatchJob("j1", {} as never)],
    ["schedulerDeleteJob", () => schedulerDeleteJob("j1")],
    ["schedulerPauseJob", () => schedulerPauseJob("j1")],
    ["schedulerResumeJob", () => schedulerResumeJob("j1")],
    ["schedulerRunJob", () => schedulerRunJob("j1")],
    ["schedulerCancelJob", () => schedulerCancelJob("j1")],
  ];

  for (const [name, call] of calls) {
    it(`${name} resolves offline instead of rejecting`, async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn(async () => {
          throw new TypeError("Failed to fetch");
        }),
      );
      const res = (await call()) as { ok: boolean; status?: number };
      expect(res.ok).toBe(false);
      expect(res.status).toBe(0);
    });
  }

  // Guards the wrapper against calling itself instead of fetch: recursion there
  // type-checks fine and only shows up as a hung call or a blown stack.
  it("issues exactly one fetch per call", async () => {
    const fetchMock = vi.fn(async () => {
      throw new TypeError("Failed to fetch");
    });
    vi.stubGlobal("fetch", fetchMock);
    await schedulerListJobs();
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});

describe("scheduler api when the backend answers", () => {
  it("returns the parsed body", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ({
        ok: true,
        status: 200,
        json: async () => ({ jobs: [] }),
      })),
    );
    const res = await schedulerListJobs();
    expect(res).toEqual({ ok: true, data: { jobs: [] } });
  });

  it("reports a refused status", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ({
        ok: false,
        status: 503,
        json: async () => ({ error: { message: "scheduler is off" } }),
      })),
    );
    const res = await schedulerListJobs();
    expect(res).toEqual({ ok: false, status: 503, message: "scheduler is off" });
  });
});
