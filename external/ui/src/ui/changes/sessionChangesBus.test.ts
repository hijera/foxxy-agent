import { afterEach, expect, test, vi } from "vitest";
import {
  emitChangesSettled,
  onChangesSettled,
  onChangesToggle,
  requestChangesToggle,
  resetChangesBusForTests,
} from "./sessionChangesBus";

afterEach(() => {
  resetChangesBusForTests();
  vi.restoreAllMocks();
});

test("a settled change set reaches every listener with its session", () => {
  const a: string[] = [];
  const b: string[] = [];
  const offA = onChangesSettled((sid) => a.push(sid));
  onChangesSettled((sid) => b.push(sid));
  emitChangesSettled("s1");
  offA();
  emitChangesSettled("s2");
  expect(a).toEqual(["s1"]);
  expect(b).toEqual(["s1", "s2"]);
});

// In IntelliJ the key can reach the page and the plugin's own shortcut alike;
// two requests that close together are one press, never an open-and-shut.
test("two toggle requests within 150 ms count as one", () => {
  const now = vi.spyOn(performance, "now");
  let toggles = 0;
  onChangesToggle(() => (toggles += 1));

  now.mockReturnValue(1000);
  expect(requestChangesToggle()).toBe(true);
  now.mockReturnValue(1100);
  expect(requestChangesToggle()).toBe(false);
  now.mockReturnValue(1300);
  expect(requestChangesToggle()).toBe(true);
  expect(toggles).toBe(2);
});

test("a listener that throws does not stop the others", () => {
  let reached = false;
  onChangesToggle(() => {
    throw new Error("broken");
  });
  onChangesToggle(() => {
    reached = true;
  });
  requestChangesToggle();
  expect(reached).toBe(true);
});
