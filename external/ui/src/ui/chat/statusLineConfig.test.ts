import { afterEach, expect, test } from "vitest";

import {
  getStatusLineEnabled,
  onStatusLineChange,
  readStatusLineFromConfigDoc,
  setStatusLineEnabled,
} from "./statusLineConfig";

afterEach(() => setStatusLineEnabled(true));

test("a missing key never disables the status line", () => {
  expect(readStatusLineFromConfigDoc(null)).toBe(true);
  expect(readStatusLineFromConfigDoc(undefined)).toBe(true);
  expect(readStatusLineFromConfigDoc({})).toBe(true);
  expect(readStatusLineFromConfigDoc({ ui: {} })).toBe(true);
  expect(readStatusLineFromConfigDoc({ ui: null })).toBe(true);
});

test("only an explicit false disables it", () => {
  expect(readStatusLineFromConfigDoc({ ui: { status_line: false } })).toBe(
    false,
  );
  expect(readStatusLineFromConfigDoc({ ui: { status_line: true } })).toBe(true);
  expect(readStatusLineFromConfigDoc({ ui: { status_line: "off" } })).toBe(true);
  expect(readStatusLineFromConfigDoc({ ui: { status_line: 0 } })).toBe(true);
});

test("notifies subscribers only on a real change", () => {
  let calls = 0;
  const unsubscribe = onStatusLineChange(() => {
    calls++;
  });

  setStatusLineEnabled(false);
  expect(getStatusLineEnabled()).toBe(false);
  expect(calls).toBe(1);

  setStatusLineEnabled(false);
  expect(calls).toBe(1);

  setStatusLineEnabled(true);
  expect(calls).toBe(2);

  unsubscribe();
  setStatusLineEnabled(false);
  expect(calls).toBe(2);
});
