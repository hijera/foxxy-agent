import { describe, it, expect } from "vitest";
import * as path from "path";
import { detectPlatform, validateBinary } from "../src/binary/binaryResolver";

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

describe("validateBinary", () => {
  // Any existing file will do: the fake `run` never executes it.
  const existing = __filename;
  const t = (key: string, ...params: unknown[]): string => [key, ...params].join("|");

  type Run = (cmd: string, args: string[]) => Promise<{ stdout: string }>;

  function fakeRun(answers: { version?: string | Error; help?: string | Error }): Run {
    return async (_cmd, args) => {
      const a = args[0] === "-v" ? answers.version : answers.help;
      if (a instanceof Error) throw a;
      return { stdout: a ?? "" };
    };
  }

  it("reports a missing file without running anything", async () => {
    const missing = path.join(__dirname, "nope-" + Date.now());
    let calls = 0;
    const r = await validateBinary(missing, async () => (calls++, { stdout: "" }), t);
    expect(r).toEqual({ ok: false, version: null, message: `binary.error.notFound|${missing}` });
    expect(calls).toBe(0);
  });

  it("reports a failed `-v`", async () => {
    const r = await validateBinary(existing, fakeRun({ version: new Error("ENOENT") }), t);
    expect(r).toEqual({
      ok: false,
      version: null,
      message: `binary.error.executeVersion|${existing}`,
    });
  });

  it("reports a failed `http --help` and keeps the version", async () => {
    const r = await validateBinary(
      existing,
      fakeRun({ version: "1.2.3\n", help: new Error("boom") }),
      t,
    );
    expect(r).toEqual({
      ok: false,
      version: "1.2.3",
      message: `binary.error.executeHelp|${existing}`,
    });
  });

  it("detects a lean build from the help text", async () => {
    for (const help of ["foxxycode http is NOT BUILT into this binary", "HTTP support is not available"]) {
      const r = await validateBinary(existing, fakeRun({ version: "1.2.3", help }), t);
      expect(r).toEqual({ ok: false, version: "1.2.3", message: "binary.error.leanBuild" });
    }
  });

  it("accepts a full build", async () => {
    const r = await validateBinary(
      existing,
      fakeRun({ version: "  1.2.3 ", help: "Usage: foxxycode http [flags]" }),
      t,
    );
    expect(r).toEqual({ ok: true, version: "1.2.3", message: "binary.ok.fullBuild|1.2.3" });
  });
});
