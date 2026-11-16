import { beforeEach, expect, test } from "vitest";
import {
  projectRootLabel,
  readProjectOnlyPref,
  sessionsProjectCwdParam,
  writeProjectOnlyPref,
} from "./sessionsProjectFilter";

beforeEach(() => {
  window.localStorage.clear();
  for (const cookie of document.cookie.split(";")) {
    document.cookie = `${cookie.split("=")[0]?.trim()}=; Path=/; Max-Age=0`;
  }
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

test("project scope survives an IDE port change with a fresh localStorage origin", () => {
  writeProjectOnlyPref("H:/projects/plain", false);
  // Cookies belong to the host, while a restarted IDE's new port gets an empty
  // localStorage. Model that new origin without deleting the host cookies.
  window.localStorage.clear();
  expect(readProjectOnlyPref("H:/projects/plain", true)).toBe(false);
  expect(readProjectOnlyPref("H:/projects/other", true)).toBe(true);
});

test("an existing project scope preference migrates before the next IDE restart", () => {
  window.localStorage.setItem("foxxycode.sessions.projectOnly:/work/old", "0");
  expect(readProjectOnlyPref("/work/old", true)).toBe(false);
  window.localStorage.clear();
  expect(readProjectOnlyPref("/work/old", true)).toBe(false);
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
