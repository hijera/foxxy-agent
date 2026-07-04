import { describe, it, expect } from "vitest";
import { pickFreePort } from "../src/process/portUtil";

describe("pickFreePort", () => {
  it("returns a fixed port when valid", async () => {
    expect(await pickFreePort(4242)).toBe(4242);
    expect(await pickFreePort(1)).toBe(1);
    expect(await pickFreePort(65535)).toBe(65535);
  });

  it("rejects out-of-range fixed ports and picks a free one", async () => {
    const p = await pickFreePort(0);
    expect(p).toBeGreaterThan(0);
    expect(p).toBeLessThanOrEqual(65535);
    const p2 = await pickFreePort(70000);
    expect(p2).toBeGreaterThan(0);
    expect(p2).toBeLessThanOrEqual(65535);
    const p3 = await pickFreePort(-1);
    expect(p3).toBeGreaterThan(0);
  });

  it("returns distinct free ports across calls", async () => {
    const a = await pickFreePort(0);
    const b = await pickFreePort(0);
    // Not strictly guaranteed, but in practice always distinct.
    expect(a).not.toBe(b);
  });
});
