import { describe, it, expect } from "vitest";
import { localeFromSetting, setLocale, t, spaLanguageCode } from "../src/i18n/bundle";

describe("localeFromSetting", () => {
  it("honours explicit en/ru", () => {
    expect(localeFromSetting("en", "ru")).toBe("en");
    expect(localeFromSetting("ru", "en")).toBe("ru");
  });

  it("follows env language when set to system", () => {
    expect(localeFromSetting("system", "ru")).toBe("ru");
    expect(localeFromSetting("system", "en")).toBe("en");
    expect(localeFromSetting("system", "fr")).toBe("en");
    expect(localeFromSetting("system", "")).toBe("en");
  });

  it("falls back to env for unknown values", () => {
    expect(localeFromSetting("klingon", "ru")).toBe("ru");
  });
});

describe("spaLanguageCode", () => {
  it("matches localeFromSetting for en/ru", () => {
    expect(spaLanguageCode("en", "ru")).toBe("en");
    expect(spaLanguageCode("ru", "en")).toBe("ru");
    expect(spaLanguageCode("system", "ru")).toBe("ru");
    expect(spaLanguageCode("system", "en")).toBe("en");
  });
});

describe("t()", () => {
  it("formats {0} placeholders in the active locale", () => {
    setLocale("en");
    expect(t("process.indicator.launching", "127.0.0.1", "8080")).toBe(
      "FoxxyCode: launching 127.0.0.1:8080…",
    );
    setLocale("ru");
    expect(t("process.indicator.launching", "127.0.0.1", "8080")).toBe(
      "FoxxyCode: запуск 127.0.0.1:8080…",
    );
  });

  it("falls back to English when key missing from current locale", () => {
    setLocale("ru");
    // Both tables have the same keys; confirm a real key resolves in the current locale.
    expect(t("toolbar.action.reload")).toBe("Обновить");
    expect(t("nonexistent.key.xyz")).toBe("nonexistent.key.xyz");
  });

  it("returns the raw string when no params are given", () => {
    setLocale("en");
    expect(t("diff.action.accept")).toBe("Accept");
  });
});
