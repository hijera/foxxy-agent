import { describe, expect, it } from "vitest";
import { contextUsagePercent, contextUsedTokens } from "./contextUsage";

describe("contextUsagePercent", () => {
  it("uses estimatedTotal over max context", () => {
    expect(
      contextUsagePercent(128000, {
        systemPrompt: 851,
        toolDefinitions: 1950,
        rules: 14867,
        skills: 45,
        conversation: 6074,
        estimatedTotal: 23787,
      }),
    ).toBeCloseTo(18.58359375, 4);
  });

  it("returns 0 without breakdown", () => {
    expect(contextUsagePercent(128000, null)).toBe(0);
  });

  it("sums parts when estimatedTotal missing", () => {
    expect(
      contextUsedTokens({
        systemPrompt: 100,
        toolDefinitions: 200,
        conversation: 300,
      }),
    ).toBe(600);
  });
});
