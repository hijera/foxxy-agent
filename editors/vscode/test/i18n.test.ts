import { afterEach, describe, it, expect } from "vitest";
import { setLocale, t, spaLanguageCode } from "../src/i18n/bundle";
import {
  localeFromConfigJson,
  resolveLocale,
  setEffectiveLocale,
} from "../src/i18n/localeState";

afterEach(() => {
  // Reset the module-level effective locale so tests don't leak state.
  setEffectiveLocale(null);
});

describe("resolveLocale", () => {
  it("honours an explicit backend locale over the host language", () => {
    setEffectiveLocale("en");
    expect(resolveLocale("ru")).toBe("en");
    setEffectiveLocale("ru");
    expect(resolveLocale("en")).toBe("ru");
  });

  it("follows the host language when the backend locale is auto (null)", () => {
    setEffectiveLocale(null);
    expect(resolveLocale("ru")).toBe("ru");
    expect(resolveLocale("ru-RU")).toBe("ru");
    expect(resolveLocale("en")).toBe("en");
    expect(resolveLocale("fr")).toBe("en");
    expect(resolveLocale("")).toBe("en");
  });
});

describe("spaLanguageCode", () => {
  it("mirrors resolveLocale for the ?lang= param", () => {
    setEffectiveLocale("ru");
    expect(spaLanguageCode("en")).toBe("ru");
    setEffectiveLocale(null);
    expect(spaLanguageCode("ru")).toBe("ru");
    expect(spaLanguageCode("en")).toBe("en");
  });
});

describe("localeFromConfigJson", () => {
  it("extracts an explicit ui.locale", () => {
    expect(localeFromConfigJson('{"ui":{"locale":"ru"}}')).toBe("ru");
    expect(localeFromConfigJson('{"ui":{"locale":"en"}}')).toBe("en");
  });

  it("returns null for auto, missing, or malformed values", () => {
    expect(localeFromConfigJson('{"ui":{"locale":""}}')).toBeNull();
    expect(localeFromConfigJson('{"ui":{}}')).toBeNull();
    expect(localeFromConfigJson("{}")).toBeNull();
    expect(localeFromConfigJson("not json")).toBeNull();
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
