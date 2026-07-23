import { describe, expect, it } from "vitest";
import { messagesEn } from "./messages/en";
import { messagesRu } from "./messages/ru";

// translate() silently falls back to English for a missing Russian key, so a forgotten translation
// looks like working code and only shows up as an English string in a Russian UI (exactly how the
// env-chip menu stayed English after the upstream sync). These tests make the gap fail the suite.
describe("en/ru message parity", () => {
  it("defines the same key set in both locales", () => {
    const en = Object.keys(messagesEn).sort();
    const ru = Object.keys(messagesRu).sort();
    expect(ru.filter((k) => !(k in messagesEn))).toEqual([]);
    expect(en.filter((k) => !(k in messagesRu))).toEqual([]);
  });

  it("has no empty values", () => {
    const blankEn = Object.entries(messagesEn)
      .filter(([, v]) => v.trim() === "")
      .map(([k]) => k);
    const blankRu = Object.entries(messagesRu)
      .filter(([, v]) => v.trim() === "")
      .map(([k]) => k);
    expect(blankEn).toEqual([]);
    expect(blankRu).toEqual([]);
  });

  it("keeps the same {param} placeholders in both locales", () => {
    const slots = (s: string) => [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();
    const mismatched = Object.keys(messagesEn).filter((k) => {
      const ru = messagesRu[k];
      return ru !== undefined && slots(messagesEn[k]!).join() !== slots(ru).join();
    });
    expect(mismatched).toEqual([]);
  });
});
