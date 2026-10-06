import { afterEach, expect, test } from "vitest";

import {
  isReconnecting,
  markConnected,
  markReconnecting,
  resetLiveConnectionState,
  snapshotLiveConnection,
  subscribeLiveConnection,
} from "./liveConnectionState";

afterEach(() => resetLiveConnectionState());

test("no session is reconnecting by default", () => {
  expect(isReconnecting("s1")).toBe(false);
  expect(isReconnecting("")).toBe(false);
});

test("marks and clears per session", () => {
  markReconnecting("s1");
  expect(isReconnecting("s1")).toBe(true);
  expect(isReconnecting("s2")).toBe(false);
  markConnected("s1");
  expect(isReconnecting("s1")).toBe(false);
});

test("notifies subscribers and advances the snapshot", () => {
  let calls = 0;
  const unsubscribe = subscribeLiveConnection(() => {
    calls++;
  });
  const before = snapshotLiveConnection();

  markReconnecting("s1");
  expect(calls).toBe(1);
  expect(snapshotLiveConnection()).toBeGreaterThan(before);

  // Re-marking the same session must not churn the snapshot, otherwise every retry
  // attempt would force a re-render.
  markReconnecting("s1");
  expect(calls).toBe(1);

  // Clearing a session that was never marked is a no-op too.
  markConnected("s2");
  expect(calls).toBe(1);

  markConnected("s1");
  expect(calls).toBe(2);

  unsubscribe();
  markReconnecting("s1");
  expect(calls).toBe(2);
});
