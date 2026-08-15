import { describe, expect, it } from "vitest";

import { planSessionStatsApply } from "./sessionTokenTotals";

const stats = {
  tokenUsageTotal: { inputTokens: 100, outputTokens: 20, totalTokens: 120 },
  contextBreakdown: { conversation: 300, estimatedTotal: 400 },
};

describe("planSessionStatsApply", () => {
  it("seeds the baseline when no live stream owns the session", () => {
    const got = planSessionStatsApply(stats, { liveStreamAttached: false });
    expect(got.tokenUsage).toEqual({
      inputTokens: 100,
      outputTokens: 20,
      totalTokens: 120,
    });
    expect(got.contextBreakdown).toEqual(stats.contextBreakdown);
  });

  // The poll runs every 800ms during a turn. Reseeding from a total that already counts
  // the running turn makes the next SSE delta double it, and again on the next poll.
  it("leaves the token baseline alone while a turn is streaming", () => {
    const got = planSessionStatsApply(stats, { liveStreamAttached: true });
    expect(got.tokenUsage).toBeNull();
  });

  // Compaction reports itself only through the breakdown, so the gate above must not
  // reach it: the context ring has to drop mid-turn.
  it("still applies the context breakdown while a turn is streaming", () => {
    const got = planSessionStatsApply(stats, { liveStreamAttached: true });
    expect(got.contextBreakdown).toEqual(stats.contextBreakdown);
  });

  it("reports nothing to apply for an empty payload", () => {
    expect(planSessionStatsApply(null, { liveStreamAttached: false })).toEqual({
      tokenUsage: null,
      contextBreakdown: null,
    });
  });
});
