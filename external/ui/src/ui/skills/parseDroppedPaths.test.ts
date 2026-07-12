import { expect, test } from "vitest";
import { parseDroppedPaths } from "./parseDroppedPaths";

test("parses a posix file:// uri-list, skipping comment lines", () => {
  const uriList = "# comment\nfile:///home/me/proj/src/foo.ts\n";
  expect(parseDroppedPaths({ uriList })).toEqual(["/home/me/proj/src/foo.ts"]);
});

test("decodes percent-encoding and windows drive uris", () => {
  const uriList = "file:///C:/Users/me/a%20b/foo.ts";
  expect(parseDroppedPaths({ uriList })).toEqual(["C:/Users/me/a b/foo.ts"]);
});

test("handles vscode-file:// wrapper uris", () => {
  const uriList = "vscode-file://vscode-app/home/me/proj/foo.ts";
  expect(parseDroppedPaths({ uriList })).toEqual(["/home/me/proj/foo.ts"]);
});

test("de-duplicates multiple uris in order", () => {
  const uriList = "file:///a/x.ts\nfile:///b/y.ts\nfile:///a/x.ts";
  expect(parseDroppedPaths({ uriList })).toEqual(["/a/x.ts", "/b/y.ts"]);
});

test("falls back to plain absolute paths when uri-list is empty", () => {
  expect(parseDroppedPaths({ uriList: "", plain: "/home/me/foo.ts" })).toEqual([
    "/home/me/foo.ts",
  ]);
  expect(parseDroppedPaths({ plain: "C:\\proj\\foo.ts" })).toEqual([
    "C:\\proj\\foo.ts",
  ]);
});

test("ignores non-file uris and relative plain text", () => {
  expect(parseDroppedPaths({ uriList: "https://example.com/x" })).toEqual([]);
  expect(parseDroppedPaths({ plain: "just some text" })).toEqual([]);
});
