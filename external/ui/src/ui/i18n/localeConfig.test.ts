import { afterEach, describe, expect, it } from "vitest";
import { getLocale, initLocale } from "./i18n";
import {
  applyStartupUiLocaleFromConfig,
  applyUiLocalePreference,
  readUiLocaleFromConfigDoc,
  resolveEffectiveUiLocale,
} from "./localeConfig";

describe("localeConfig", () => {
  it("reads ui.locale from config doc", () => {
    expect(readUiLocaleFromConfigDoc({ ui: { locale: "ru" } })).toBe("ru");
    expect(readUiLocaleFromConfigDoc({ ui: { locale: "en" } })).toBe("en");
    expect(readUiLocaleFromConfigDoc({ ui: { locale: "de" } })).toBe("");
    expect(readUiLocaleFromConfigDoc(null)).toBe("");
  });

  it("resolves auto preference from navigator", () => {
    const original = navigator.language;
    Object.defineProperty(navigator, "language", {
      configurable: true,
      value: "ru-RU",
    });
    expect(resolveEffectiveUiLocale("")).toBe("ru");
    expect(resolveEffectiveUiLocale("en")).toBe("en");
    Object.defineProperty(navigator, "language", {
      configurable: true,
      value: original,
    });
  });

  it("applyUiLocalePreference sets document.lang", () => {
    applyUiLocalePreference("ru");
    expect(document.documentElement.lang).toBe("ru");
  });

  describe("applyStartupUiLocaleFromConfig", () => {
    const originalSearch = window.location.search;

    function stubSearch(search: string) {
      Object.defineProperty(window, "location", {
        configurable: true,
        value: { ...window.location, search, protocol: "http:" },
      });
    }

    afterEach(() => {
      stubSearch(originalSearch);
    });

    it("explicit ?lang= wins over config preference", () => {
      stubSearch("?lang=en");
      initLocale("en");
      applyStartupUiLocaleFromConfig("ru");
      expect(getLocale()).toBe("en");
    });

    it("empty config preference keeps the bootstrap locale", () => {
      stubSearch("");
      initLocale("ru");
      applyStartupUiLocaleFromConfig("");
      expect(getLocale()).toBe("ru");
    });

    it("config preference applies without ?lang=", () => {
      stubSearch("");
      initLocale("en");
      applyStartupUiLocaleFromConfig("ru");
      expect(getLocale()).toBe("ru");
    });
  });
});
