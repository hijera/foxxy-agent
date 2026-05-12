import { expect, test } from "vitest";
import {
  HERO_ACCENT_VERBS,
  hashString,
  pickHeroAccentVerb,
} from "./heroTitleWords";

test("pickHeroAccentVerb is stable for a session id", () => {
  const a = pickHeroAccentVerb("sess-abc", 0);
  const b = pickHeroAccentVerb("sess-abc", 99);
  expect(a).toBe(b);
  expect(HERO_ACCENT_VERBS).toContain(a);
});

test("pickHeroAccentVerb rotates when there is no session id", () => {
  const g0 = pickHeroAccentVerb("", 0);
  const g1 = pickHeroAccentVerb("", 1);
  expect(HERO_ACCENT_VERBS).toContain(g0);
  expect(HERO_ACCENT_VERBS).toContain(g1);
  expect(HERO_ACCENT_VERBS.length).toBeGreaterThan(1);
});

test("hashString is deterministic", () => {
  expect(hashString("x")).toBe(hashString("x"));
  expect(hashString("a")).not.toBe(hashString("b"));
});
