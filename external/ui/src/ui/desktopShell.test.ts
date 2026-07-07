import { beforeEach, expect, test } from "vitest";
import {
  bootstrapDesktopFlag,
  isDesktopShell,
  readDesktopFromUrl,
} from "./desktopShell";

beforeEach(() => {
  window.sessionStorage.clear();
  window.history.replaceState({}, "", "/");
});

test("readDesktopFromUrl detects ?desktop=1", () => {
  window.history.replaceState({}, "", "/?desktop=1");
  expect(readDesktopFromUrl()).toBe(true);
});

test("readDesktopFromUrl detects marker alongside ?lang", () => {
  window.history.replaceState({}, "", "/?lang=ru&desktop=1");
  expect(readDesktopFromUrl()).toBe(true);
});

test("readDesktopFromUrl false without the marker", () => {
  window.history.replaceState({}, "", "/?lang=ru");
  expect(readDesktopFromUrl()).toBe(false);
});

test("bootstrap latches the flag so isDesktopShell survives hash navigation", () => {
  window.history.replaceState({}, "", "/?desktop=1");
  expect(bootstrapDesktopFlag()).toBe(true);
  // SPA hash routing drops the query string on later navigations.
  window.history.replaceState({}, "", "/#/chat");
  expect(readDesktopFromUrl()).toBe(false);
  expect(isDesktopShell()).toBe(true);
});

test("isDesktopShell is false when the shell was never desktop", () => {
  expect(isDesktopShell()).toBe(false);
  expect(bootstrapDesktopFlag()).toBe(false);
});
