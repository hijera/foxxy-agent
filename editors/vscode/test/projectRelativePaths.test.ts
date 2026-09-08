import { describe, it, expect } from "vitest";
import * as path from "path";
import { relativizeWorkspacePaths } from "../src/ide/projectRelativePaths";

const noDirs = { isDirectory: () => false };

describe("relativizeWorkspacePaths (posix)", () => {
  const posix = { ...noDirs, pathImpl: path.posix };

  it("relativizes files under the root as POSIX paths", () => {
    expect(
      relativizeWorkspacePaths("/w", ["/w/src/main/App.ts", "/w/README.md"], posix),
    ).toEqual(["src/main/App.ts", "README.md"]);
  });

  it("drops directories", () => {
    const isDirectory = (p: string) => p.endsWith("/src");
    expect(
      relativizeWorkspacePaths("/w", ["/w/src", "/w/src/a.ts"], { isDirectory, pathImpl: path.posix }),
    ).toEqual(["src/a.ts"]);
  });

  it("drops the root itself and anything outside it", () => {
    expect(
      relativizeWorkspacePaths("/w", ["/w", "/other/x.ts", "/w/../y.ts", "/wider/z.ts"], posix),
    ).toEqual([]);
  });

  it("de-duplicates while keeping input order", () => {
    expect(
      relativizeWorkspacePaths("/w", ["/w/b.ts", "/w/a.ts", "/w/b.ts"], posix),
    ).toEqual(["b.ts", "a.ts"]);
  });

  it("yields nothing for a blank root or an empty selection", () => {
    expect(relativizeWorkspacePaths("", ["/w/a.ts"], posix)).toEqual([]);
    expect(relativizeWorkspacePaths(undefined, ["/w/a.ts"], posix)).toEqual([]);
    expect(relativizeWorkspacePaths("   ", ["/w/a.ts"], posix)).toEqual([]);
    expect(relativizeWorkspacePaths("/w", [], posix)).toEqual([]);
  });

  it("skips blank entries", () => {
    expect(relativizeWorkspacePaths("/w", ["", "  ", "/w/a.ts"], posix)).toEqual(["a.ts"]);
  });
});

describe("relativizeWorkspacePaths (win32)", () => {
  const win = { ...noDirs, pathImpl: path.win32 };

  it("converts backslashes and ignores drive-letter case", () => {
    expect(
      relativizeWorkspacePaths("C:\\w", ["c:\\w\\src\\A.ts", "C:\\w\\lib\\b.ts"], win),
    ).toEqual(["src/A.ts", "lib/b.ts"]);
  });

  it("drops files on another drive", () => {
    expect(relativizeWorkspacePaths("C:\\w", ["D:\\x.ts"], win)).toEqual([]);
  });

  it("drops files outside the root", () => {
    expect(relativizeWorkspacePaths("C:\\w", ["C:\\other\\x.ts", "C:\\w"], win)).toEqual([]);
  });
});
