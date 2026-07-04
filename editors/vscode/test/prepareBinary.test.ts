import { describe, it, expect } from "vitest";
import { goToVscodeTarget, normalizeToGoTarget, TARGETS } from "../scripts/prepare-binary.mjs";

describe("prepare-binary.mjs", () => {
  it("exposes all 5 desktop targets", () => {
    expect(TARGETS.map((t) => `${t.goos}-${t.goarch}`).sort()).toEqual(
      ["darwin-amd64", "darwin-arm64", "linux-amd64", "linux-arm64", "windows-amd64"].sort(),
    );
  });

  it("maps Go targets to VS Code vsce targets", () => {
    expect(goToVscodeTarget("linux", "amd64")).toBe("linux-x64");
    expect(goToVscodeTarget("linux", "arm64")).toBe("linux-arm64");
    expect(goToVscodeTarget("darwin", "amd64")).toBe("darwin-x64");
    expect(goToVscodeTarget("darwin", "arm64")).toBe("darwin-arm64");
    expect(goToVscodeTarget("windows", "amd64")).toBe("win32-x64");
  });

  it("normalizes Go and vsce targets to Go GOOS/GOARCH", () => {
    // vsce targets
    expect(normalizeToGoTarget("win32-x64")).toEqual({ goos: "windows", goarch: "amd64" });
    expect(normalizeToGoTarget("linux-arm64")).toEqual({ goos: "linux", goarch: "arm64" });
    expect(normalizeToGoTarget("darwin-x64")).toEqual({ goos: "darwin", goarch: "amd64" });
    // Go targets pass through unchanged
    expect(normalizeToGoTarget("windows-amd64")).toEqual({ goos: "windows", goarch: "amd64" });
    expect(normalizeToGoTarget("linux-amd64")).toEqual({ goos: "linux", goarch: "amd64" });
    // malformed input throws
    expect(() => normalizeToGoTarget("win32")).toThrow();
  });
});
