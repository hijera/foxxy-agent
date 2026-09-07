import { describe, it, expect } from "vitest";
import {
  CAPTURE_MIN_INTERVAL_MS,
  CAPTURE_SENTINEL,
  FRESH_EXECUTION_MS,
  mergeScreenCapture,
  normalizeScreenCapture,
  screenFromClipboard,
  shouldCaptureScreen,
} from "../src/ide/terminalCapturePolicy";

const base = {
  enabled: true,
  hasActiveTerminal: true,
  capturing: false,
  now: 100_000,
  lastCaptureAt: null,
  lastExecutionOutputAt: null,
};

describe("shouldCaptureScreen", () => {
  it("captures a fresh terminal when enabled", () => {
    expect(shouldCaptureScreen(base)).toBe(true);
  });

  it("never captures when disabled, without an active terminal, or while capturing", () => {
    expect(shouldCaptureScreen({ ...base, enabled: false })).toBe(false);
    expect(shouldCaptureScreen({ ...base, hasActiveTerminal: false })).toBe(false);
    expect(shouldCaptureScreen({ ...base, capturing: true })).toBe(false);
  });

  it("throttles repeated captures of the same terminal", () => {
    const recent = base.now - CAPTURE_MIN_INTERVAL_MS + 1;
    expect(shouldCaptureScreen({ ...base, lastCaptureAt: recent })).toBe(false);
    const old = base.now - CAPTURE_MIN_INTERVAL_MS;
    expect(shouldCaptureScreen({ ...base, lastCaptureAt: old })).toBe(true);
  });

  it("prefers fresh shell-integration output over the clipboard round-trip", () => {
    const fresh = base.now - FRESH_EXECUTION_MS + 1;
    expect(shouldCaptureScreen({ ...base, lastExecutionOutputAt: fresh })).toBe(false);
    const stale = base.now - FRESH_EXECUTION_MS;
    expect(shouldCaptureScreen({ ...base, lastExecutionOutputAt: stale })).toBe(true);
  });
});

describe("screenFromClipboard", () => {
  it("treats the untouched sentinel and blank text as nothing captured", () => {
    expect(screenFromClipboard(CAPTURE_SENTINEL)).toBeNull();
    expect(screenFromClipboard("")).toBeNull();
    expect(screenFromClipboard("  \n\t")).toBeNull();
  });

  it("returns real clipboard text", () => {
    expect(screenFromClipboard("$ ls\nREADME.md\n")).toBe("$ ls\nREADME.md\n");
  });
});

describe("normalizeScreenCapture", () => {
  it("strips ANSI, unifies CRLF, and trims trailing padding", () => {
    const raw = "\u001b[32m$ ls\u001b[0m\r\nREADME.md   \r\n\r\n\r\n";
    expect(normalizeScreenCapture(raw, 1024)).toBe("$ ls\nREADME.md");
  });

  it("keeps only the tail when the screen exceeds the cap", () => {
    const raw = "0123456789";
    expect(normalizeScreenCapture(raw, 4)).toBe("6789");
  });
});

describe("mergeScreenCapture", () => {
  it("replaces the buffer with the captured screen", () => {
    expect(mergeScreenCapture("old", "new")).toEqual({ output: "new", changed: true });
  });

  it("reports no change when the screen is identical", () => {
    expect(mergeScreenCapture("same", "same")).toEqual({ output: "same", changed: false });
  });
});
