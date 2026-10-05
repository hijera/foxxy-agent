import { expect, test } from "vitest";
import {
  baseName,
  dirName,
  fileCountKey,
  fileCountKeySuffix,
} from "./sessionChangesText";

test("Russian plural buckets pick the right message key", () => {
  // 1, 21, 101 take the singular form; 11-14 are the exception that makes a
  // naive "count % 10" rule wrong.
  for (const n of [1, 21, 101, 1001]) {
    expect(fileCountKeySuffix(n)).toBe("One");
  }
  for (const n of [2, 3, 4, 22, 34, 103]) {
    expect(fileCountKeySuffix(n)).toBe("Few");
  }
  for (const n of [0, 5, 9, 11, 12, 13, 14, 25, 111, 112]) {
    expect(fileCountKeySuffix(n)).toBe("Many");
  }
});

test("the key is namespaced so both dictionaries define it", () => {
  expect(fileCountKey(1)).toBe("changes.card.filesOne");
  expect(fileCountKey(3)).toBe("changes.card.filesFew");
  expect(fileCountKey(7)).toBe("changes.card.filesMany");
});

test("paths split into name and folder on either separator", () => {
  expect(baseName("src/ui/App.tsx")).toBe("App.tsx");
  expect(dirName("src/ui/App.tsx")).toBe("src/ui");
  // The server reports workspace-relative paths, which on Windows arrive with
  // backslashes.
  expect(baseName("src\\ui\\App.tsx")).toBe("App.tsx");
  expect(dirName("src\\ui\\App.tsx")).toBe("src/ui");
});

test("a file at the workspace root has no folder part", () => {
  expect(baseName("index.html")).toBe("index.html");
  expect(dirName("index.html")).toBe("");
});
