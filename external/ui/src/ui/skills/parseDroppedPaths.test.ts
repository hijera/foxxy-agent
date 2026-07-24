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

// Dragging an editor tab in VS Code puts the ResourceURLs JSON array on the
// DataTransfer; older builds carry it without a text/uri-list entry at all.
test("parses the VS Code ResourceURLs JSON array", () => {
  const resourceUrls = JSON.stringify([
    "file:///home/me/proj/src/tab.ts",
    "file:///C:/proj/other.ts",
  ]);
  expect(parseDroppedPaths({ resourceUrls })).toEqual([
    "/home/me/proj/src/tab.ts",
    "C:/proj/other.ts",
  ]);
});

test("uri-list still wins over ResourceURLs, and both de-duplicate", () => {
  expect(
    parseDroppedPaths({
      uriList: "file:///a/x.ts",
      resourceUrls: JSON.stringify(["file:///a/x.ts", "file:///b/y.ts"]),
    }),
  ).toEqual(["/a/x.ts", "/b/y.ts"]);
});

test("malformed or non-file ResourceURLs payloads are ignored", () => {
  expect(parseDroppedPaths({ resourceUrls: "not json" })).toEqual([]);
  expect(parseDroppedPaths({ resourceUrls: JSON.stringify({}) })).toEqual([]);
  expect(
    parseDroppedPaths({ resourceUrls: JSON.stringify(["untitled:Untitled-1"]) }),
  ).toEqual([]);
});

test("ResourceURLs does not suppress the plain-text fallback when it yields nothing", () => {
  expect(
    parseDroppedPaths({ resourceUrls: "not json", plain: "/home/me/foo.ts" }),
  ).toEqual(["/home/me/foo.ts"]);
});
