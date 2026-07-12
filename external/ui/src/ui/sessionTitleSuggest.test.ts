import { expect, test } from "vitest";
import { scheduleSessionTitleRefresh } from "./sessionTitleSuggest";

test("refreshes the session at each delay once the id is known", async () => {
  const scheduled: Array<{ ms: number; run: () => void }> = [];
  const refreshed: string[] = [];

  scheduleSessionTitleRefresh({
    sessionIdPromise: Promise.resolve("sess_x"),
    refresh: (sid) => refreshed.push(sid),
    delaysMs: [10, 20, 30],
    schedule: (fn, ms) => scheduled.push({ ms, run: fn }),
  });

  // Wait for the async id resolution to schedule the refreshes.
  await Promise.resolve();
  await Promise.resolve();

  expect(scheduled.map((s) => s.ms)).toEqual([10, 20, 30]);

  // Fire the scheduled callbacks: each refreshes the resolved session id.
  scheduled.forEach((s) => s.run());
  expect(refreshed).toEqual(["sess_x", "sess_x", "sess_x"]);
});

test("does nothing when the session id resolves empty", async () => {
  const scheduled: number[] = [];
  const refreshed: string[] = [];

  scheduleSessionTitleRefresh({
    sessionIdPromise: Promise.resolve("   "),
    refresh: (sid) => refreshed.push(sid),
    delaysMs: [10],
    schedule: (fn, ms) => {
      scheduled.push(ms);
      fn();
    },
  });

  await Promise.resolve();
  await Promise.resolve();

  expect(scheduled).toEqual([]);
  expect(refreshed).toEqual([]);
});

test("does nothing when the session id promise rejects", async () => {
  const scheduled: number[] = [];

  scheduleSessionTitleRefresh({
    sessionIdPromise: Promise.reject(new Error("no id")),
    refresh: () => scheduled.push(-1),
    delaysMs: [10],
    schedule: (_fn, ms) => scheduled.push(ms),
  });

  await Promise.resolve();
  await Promise.resolve();

  expect(scheduled).toEqual([]);
});
