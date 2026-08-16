import { describe, expect, it } from "vitest";
import { UI_LOCALES, UI_LOCALE_DEFAULT, UI_LOCALE_IDS } from "./locales";

const defaultMessages = UI_LOCALES[UI_LOCALE_DEFAULT].messages;
const otherLocales = UI_LOCALE_IDS.filter((id) => id !== UI_LOCALE_DEFAULT);

// translate() silently falls back to the default locale for a missing key, so a forgotten
// translation looks like working code and only shows up as an English string in a non-English UI
// (exactly how the env-chip menu stayed English after an upstream sync). These tests make the gap
// fail the suite. They walk the locale registry rather than a hardcoded en/ru pair, so a locale
// added later is covered without touching this file.
describe("message parity across registered locales", () => {
  it.each(otherLocales)("%s defines the same key set as the default locale", (id) => {
    const messages = UI_LOCALES[id].messages;
    expect(Object.keys(messages).filter((k) => !(k in defaultMessages))).toEqual([]);
    expect(Object.keys(defaultMessages).filter((k) => !(k in messages))).toEqual([]);
  });

  it.each(UI_LOCALE_IDS)("%s has no empty values", (id) => {
    const blank = Object.entries(UI_LOCALES[id].messages)
      .filter(([, v]) => v.trim() === "")
      .map(([k]) => k);
    expect(blank).toEqual([]);
  });

  it.each(otherLocales)("%s keeps the same {param} placeholders", (id) => {
    const messages = UI_LOCALES[id].messages;
    const slots = (s: string) => [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();
    const mismatched = Object.keys(defaultMessages).filter((k) => {
      const translated = messages[k];
      return (
        translated !== undefined &&
        slots(defaultMessages[k]!).join() !== slots(translated).join()
      );
    });
    expect(mismatched).toEqual([]);
  });
});
