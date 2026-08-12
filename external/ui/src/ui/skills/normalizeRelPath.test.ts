import { expect, test } from "vitest";
import { normalizeRelPath } from "./normalizeRelPath";

test("normalizeRelPath trims ./, slashes, and backslashes", () => {
  expect(normalizeRelPath("./src\\foo.ts")).toBe("src/foo.ts");
  expect(normalizeRelPath("/a/b/")).toBe("a/b");
  expect(normalizeRelPath("src/app/foo.ts")).toBe("src/app/foo.ts");
  expect(normalizeRelPath("")).toBe("");
});
