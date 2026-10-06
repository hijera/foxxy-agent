import { describe, expect, test } from "vitest";
import {
  isUiLocale,
  UI_LOCALES,
  UI_LOCALE_DEFAULT,
  UI_LOCALE_IDS,
} from "./locales";

describe("locale registry", () => {
  test("the id list mirrors the registry keys", () => {
    expect(UI_LOCALE_IDS).toEqual(Object.keys(UI_LOCALES));
    expect(UI_LOCALE_IDS.length).toBeGreaterThan(0);
  });

  test("every entry is self-consistent", () => {
    for (const id of UI_LOCALE_IDS) {
      const def = UI_LOCALES[id];
      expect(def.id).toBe(id);
      // The picker derives its option labels from labelKey, so the convention
      // has to hold for a locale added later without touching the component.
      expect(def.labelKey).toBe(`appearance.locale.${id}`);
      expect(Object.keys(def.messages).length).toBeGreaterThan(0);
    }
  });

  test("isUiLocale accepts registered ids only", () => {
    expect(isUiLocale(UI_LOCALE_DEFAULT)).toBe(true);
    expect(isUiLocale("de")).toBe(false);
    expect(isUiLocale("")).toBe(false);
    // Guards against Object.prototype keys leaking through the hasOwnProperty check.
    expect(isUiLocale("toString")).toBe(false);
  });
});
