import { expect, test } from "vitest";
import {
  MAX_PENDING_MENTIONS,
  emitFileMention,
  subscribeFileMention,
} from "./fileMentionBus";

/** Drains anything a previous test left queued so each test starts from an empty bus. */
function drainPending(): void {
  subscribeFileMention(() => {})();
}

test("delivers emitted paths to subscribers and unsubscribes", () => {
  drainPending();
  const seen: string[] = [];
  const off = subscribeFileMention((p) => seen.push(p));
  emitFileMention("src/foo.ts");
  expect(seen).toEqual(["src/foo.ts"]);
  off();
  emitFileMention("src/bar.ts");
  expect(seen).toEqual(["src/foo.ts"]);
  drainPending();
});

test("a throwing listener does not block others", () => {
  drainPending();
  const seen: string[] = [];
  const offA = subscribeFileMention(() => {
    throw new Error("boom");
  });
  const offB = subscribeFileMention((p) => seen.push(p));
  emitFileMention("x.ts");
  expect(seen).toEqual(["x.ts"]);
  offA();
  offB();
  drainPending();
});

test("a path emitted before anyone listens reaches the first subscriber", () => {
  drainPending();
  emitFileMention("src/early.ts");
  const seen: string[] = [];
  const off = subscribeFileMention((p) => seen.push(p));
  expect(seen).toEqual(["src/early.ts"]);
  off();
  drainPending();
});

test("the queue is drained once, not replayed to later subscribers", () => {
  drainPending();
  emitFileMention("a.ts");
  emitFileMention("b.ts");
  const first: string[] = [];
  const offFirst = subscribeFileMention((p) => first.push(p));
  expect(first).toEqual(["a.ts", "b.ts"]);
  const second: string[] = [];
  const offSecond = subscribeFileMention((p) => second.push(p));
  expect(second).toEqual([]);
  offFirst();
  offSecond();
  drainPending();
});

test("the queue is capped and keeps the most recent paths", () => {
  drainPending();
  for (let i = 0; i < MAX_PENDING_MENTIONS + 5; i++) {
    emitFileMention(`f${i}.ts`);
  }
  const seen: string[] = [];
  const off = subscribeFileMention((p) => seen.push(p));
  expect(seen).toHaveLength(MAX_PENDING_MENTIONS);
  expect(seen[0]).toBe("f5.ts");
  expect(seen[seen.length - 1]).toBe(`f${MAX_PENDING_MENTIONS + 4}.ts`);
  off();
  drainPending();
});
