import { expect, test } from "vitest";
import {
  segmentComposerSlashSpans,
  segmentComposerSlashSpansForcedPlainRange,
  slugSlashesForUserBubbleMarkdown,
  stripFoxxyCodeSkillMarkdownLinks,
} from "./segmentComposerSlashSpans";

test("segmentComposerSlashSpans plain slash mid-line", () => {
  const s = segmentComposerSlashSpans("asdfasf /find-skills asdfasdf");
  expect(s).toEqual([
    { type: "text", value: "asdfasf " },
    { type: "slash", literal: "/find-skills", name: "find-skills" },
    { type: "text", value: " asdfasdf" },
  ]);
});

test("segmentComposerSlashSpans line start slash", () => {
  expect(segmentComposerSlashSpans("/foo")).toEqual([
    { type: "slash", literal: "/foo", name: "foo" },
  ]);
});

test("segmentComposerSlashSpans skips letter before slash", () => {
  expect(segmentComposerSlashSpans("x/foo")).toEqual([
    { type: "text", value: "x/foo" },
  ]);
});

test("segmentComposerSlashSpansForcedPlainRange turns token into plain text", () => {
  expect(segmentComposerSlashSpansForcedPlainRange("/as", 0, 3)).toEqual([
    { type: "text", value: "/as" },
  ]);
});

test("segmentComposerSlashSpansForcedPlainRange mid line", () => {
  expect(
    segmentComposerSlashSpansForcedPlainRange("pre /as suf", 4, 7),
  ).toEqual([
    { type: "text", value: "pre " },
    { type: "text", value: "/as" },
    { type: "text", value: " suf" },
  ]);
});

test("stripFoxxyCodeSkillMarkdownLinks restores plain slash token", () => {
  expect(stripFoxxyCodeSkillMarkdownLinks("a [/demo](foxxycode-skill:demo) b")).toBe(
    "a /demo b",
  );
});

test("stripFoxxyCodeSkillMarkdownLinks leaves mismatched href", () => {
  const s = "a [/demo](foxxycode-skill:other) b";
  expect(stripFoxxyCodeSkillMarkdownLinks(s)).toBe(s);
});

test("slugSlashesForUserBubbleMarkdown for Markdown chip render", () => {
  expect(slugSlashesForUserBubbleMarkdown("hi /demo there")).toBe(
    "hi [/demo](foxxycode-skill:demo) there",
  );
});

test("slugSlashesForUserBubbleMarkdown strips legacy first then chips", () => {
  expect(slugSlashesForUserBubbleMarkdown("x [/a](foxxycode-skill:a) /b")).toBe(
    "x [/a](foxxycode-skill:a) [/b](foxxycode-skill:b)",
  );
});
