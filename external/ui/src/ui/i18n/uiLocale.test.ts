import { describe, expect, it } from "vitest";
import {
  mapSystemLocaleToSupported,
  readNavigatorLanguage,
} from "./localeCookie";
import { bootstrapUiLocaleFromUrlOrCookie } from "./uiLocale";

describe("mapSystemLocaleToSupported", () => {
  it("maps Russian system locales to ru", () => {
    expect(mapSystemLocaleToSupported("ru-RU")).toBe("ru");
    expect(mapSystemLocaleToSupported("ru")).toBe("ru");
  });

  it("falls back to en for other locales", () => {
    expect(mapSystemLocaleToSupported("en-US")).toBe("en");
    expect(mapSystemLocaleToSupported("de-DE")).toBe("en");
  });
});

describe("bootstrapUiLocaleFromUrlOrCookie", () => {
  it("uses navigator language when cookie is absent", () => {
    const original = navigator.language;
    Object.defineProperty(navigator, "language", {
      configurable: true,
      value: "ru-RU",
    });
    document.cookie = "foxxycode_ui_lang=; Path=/; Max-Age=0";
    const locale = bootstrapUiLocaleFromUrlOrCookie();
    Object.defineProperty(navigator, "language", {
      configurable: true,
      value: original,
    });
    expect(locale).toBe("ru");
    expect(document.documentElement.lang).toBe("ru");
  });

  it("readNavigatorLanguage returns a string", () => {
    expect(typeof readNavigatorLanguage()).toBe("string");
  });
});
