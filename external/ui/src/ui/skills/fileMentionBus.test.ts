import { expect, test } from "vitest";
import { emitFileMention, subscribeFileMention } from "./fileMentionBus";

test("delivers emitted paths to subscribers and unsubscribes", () => {
  const seen: string[] = [];
  const off = subscribeFileMention((p) => seen.push(p));
  emitFileMention("src/foo.ts");
  expect(seen).toEqual(["src/foo.ts"]);
  off();
  emitFileMention("src/bar.ts");
  expect(seen).toEqual(["src/foo.ts"]);
});

test("a throwing listener does not block others", () => {
  const seen: string[] = [];
  const offA = subscribeFileMention(() => {
    throw new Error("boom");
  });
  const offB = subscribeFileMention((p) => seen.push(p));
  emitFileMention("x.ts");
  expect(seen).toEqual(["x.ts"]);
  offA();
  offB();
});
