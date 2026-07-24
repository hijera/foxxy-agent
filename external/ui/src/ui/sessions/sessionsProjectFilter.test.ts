import { beforeEach, expect, test } from "vitest";
import {
  projectRootLabel,
  readProjectOnlyPref,
  sessionsProjectCwdParam,
  writeProjectOnlyPref,
} from "./sessionsProjectFilter";

beforeEach(() => {
  window.localStorage.clear();
});

test("scoped request carries the host project root", () => {
  expect(
    sessionsProjectCwdParam({ projectOnly: true, projectRoot: "/work/proj" }),
  ).toBe("/work/proj");
});

test("toggle off drops the scope", () => {
  expect(
    sessionsProjectCwdParam({ projectOnly: false, projectRoot: "/work/proj" }),
  ).toBeNull();
});

test("an unknown project root never scopes the list", () => {
  expect(sessionsProjectCwdParam({ projectOnly: true })).toBeNull();
  expect(
    sessionsProjectCwdParam({ projectOnly: true, projectRoot: "   " }),
  ).toBeNull();
});

test("preference defaults to the caller's fallback until stored", () => {
  expect(readProjectOnlyPref("/work/proj", true)).toBe(true);
  expect(readProjectOnlyPref("/work/proj", false)).toBe(false);
});

test("preference round-trips and overrides the fallback", () => {
  writeProjectOnlyPref("/work/proj", false);
  expect(readProjectOnlyPref("/work/proj", true)).toBe(false);
  writeProjectOnlyPref("/work/proj", true);
  expect(readProjectOnlyPref("/work/proj", false)).toBe(true);
});

test("preference is per project root", () => {
  writeProjectOnlyPref("/work/proj", false);
  expect(readProjectOnlyPref("/work/other", true)).toBe(true);
});

test("without a project root the preference is not persisted", () => {
  writeProjectOnlyPref("", false);
  expect(window.localStorage.length).toBe(0);
  expect(readProjectOnlyPref("", true)).toBe(true);
});

test("label is the last folder segment on both separator flavours", () => {
  expect(projectRootLabel("/work/proj")).toBe("proj");
  expect(projectRootLabel("C:\\work\\proj")).toBe("proj");
  expect(projectRootLabel("/work/proj/")).toBe("proj");
  expect(projectRootLabel("")).toBe("");
});
