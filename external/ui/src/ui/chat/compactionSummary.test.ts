import { describe, expect, it } from "vitest";

import { stripCompactionPreamble } from "./compactionSummary";

describe("stripCompactionPreamble", () => {
  it("strips the coddy engine preamble", () => {
    const body = "- goal: ship the feature\n- next: write tests";
    const raw =
      "The earlier conversation was compacted. Summary of the compacted part:\n\n" +
      body;
    expect(stripCompactionPreamble(raw)).toBe(body);
  });

  it("strips the opencode engine preamble", () => {
    const body = "user wants X; decided Y";
    const raw =
      "Summary of the earlier conversation (older turns were compacted to save context):\n\n" +
      body;
    expect(stripCompactionPreamble(raw)).toBe(body);
  });

  it("returns trimmed content unchanged when no preamble matches", () => {
    expect(stripCompactionPreamble("  plain summary  ")).toBe("plain summary");
  });

  it("tolerates empty input", () => {
    expect(stripCompactionPreamble("")).toBe("");
  });
});
