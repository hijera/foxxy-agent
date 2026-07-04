import { describe, it, expect } from "vitest";
import { detectPlatform } from "../src/binary/binaryResolver";

describe("detectPlatform", () => {
  it("maps win32 x64 to windows-amd64", () => {
    const p = detectPlatform("win32" as NodeJS.Platform, "x64");
    expect(p.goos).toBe("windows");
    expect(p.goarch).toBe("amd64");
    expect(p.binName).toBe("foxxycode.exe");
    expect(p.bundledRelative).toBe("windows-amd64/foxxycode.exe");
  });

  it("maps darwin arm64", () => {
    const p = detectPlatform("darwin" as NodeJS.Platform, "arm64");
    expect(p.goos).toBe("darwin");
    expect(p.goarch).toBe("arm64");
    expect(p.binName).toBe("foxxycode");
    expect(p.bundledRelative).toBe("darwin-arm64/foxxycode");
  });

  it("maps linux x64", () => {
    const p = detectPlatform("linux" as NodeJS.Platform, "x64");
    expect(p.goos).toBe("linux");
    expect(p.goarch).toBe("amd64");
    expect(p.bundledRelative).toBe("linux-amd64/foxxycode");
  });

  it("treats unknown arch as amd64 fallback", () => {
    const p = detectPlatform("linux" as NodeJS.Platform, "ia32");
    expect(p.goarch).toBe("amd64");
  });
});
