import { expect, test } from "vitest";
import {
  normalizeRelPath,
  uniqueMentionLabel,
  type MentionEntry,
} from "./uniqueMentionLabel";

test("basename when no collision", () => {
  expect(uniqueMentionLabel("src/app/foo.ts", [])).toBe("foo.ts");
});

test("reuses existing label for the same path", () => {
  const existing: MentionEntry[] = [{ label: "foo.ts", pathRel: "src/app/foo.ts" }];
  expect(uniqueMentionLabel("src/app/foo.ts", existing)).toBe("foo.ts");
  // Different casing of separators still counts as the same path.
  expect(uniqueMentionLabel("src\\app\\foo.ts", existing)).toBe("foo.ts");
});

test("extends leftward on duplicate basename", () => {
  const existing: MentionEntry[] = [{ label: "foo.ts", pathRel: "a/foo.ts" }];
  expect(uniqueMentionLabel("b/foo.ts", existing)).toBe("b/foo.ts");
});

test("extends further when parent segment also collides", () => {
  const existing: MentionEntry[] = [
    { label: "foo.ts", pathRel: "a/foo.ts" },
    { label: "b/foo.ts", pathRel: "x/b/foo.ts" },
  ];
  expect(uniqueMentionLabel("y/b/foo.ts", existing)).toBe("y/b/foo.ts");
});

test("normalizeRelPath trims ./, slashes, and backslashes", () => {
  expect(normalizeRelPath("./src\\foo.ts")).toBe("src/foo.ts");
  expect(normalizeRelPath("/a/b/")).toBe("a/b");
});
