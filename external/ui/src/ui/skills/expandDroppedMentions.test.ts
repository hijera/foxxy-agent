import { expect, test } from "vitest";
import { expandDroppedMentions } from "./expandDroppedMentions";
import type { MentionEntry } from "./uniqueMentionLabel";

const map: MentionEntry[] = [
  { label: "foo.ts", pathRel: "src/app/foo.ts" },
  { label: "b/bar.ts", pathRel: "pkg/b/bar.ts" },
];

test("expands a short label to its full relative path", () => {
  expect(expandDroppedMentions("look at @foo.ts please", map)).toBe(
    "look at @src/app/foo.ts please",
  );
});

test("expands multiple mentions including extended labels", () => {
  expect(expandDroppedMentions("@foo.ts and @b/bar.ts", map)).toBe(
    "@src/app/foo.ts and @pkg/b/bar.ts",
  );
});

test("leaves unmapped @tokens untouched", () => {
  expect(expandDroppedMentions("@other.ts stays", map)).toBe("@other.ts stays");
});

test("does not touch @label inside a fenced code block", () => {
  const src = "```\n@foo.ts\n```";
  expect(expandDroppedMentions(src, map)).toBe(src);
});

test("no-op when there are no entries", () => {
  expect(expandDroppedMentions("@foo.ts", [])).toBe("@foo.ts");
});
