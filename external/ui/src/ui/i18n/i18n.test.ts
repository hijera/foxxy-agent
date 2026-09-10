import { afterEach, beforeEach, describe, expect, it } from "vitest";
import {
  getLocale,
  initLocale,
  pluralCategories,
  setLocale,
  t,
  translatePlural,
} from "./i18n";
import { FOXXYCODE_UI_LANG_COOKIE } from "./localeCookie";

describe("i18n translate", () => {
  beforeEach(() => {
    document.cookie = `${FOXXYCODE_UI_LANG_COOKIE}=; Max-Age=0; Path=/`;
    initLocale("en");
  });

  afterEach(() => {
    document.cookie = `${FOXXYCODE_UI_LANG_COOKIE}=; Max-Age=0; Path=/`;
    initLocale("en");
  });

  it("returns English by default", () => {
    expect(t("nav.newChatTooltip")).toBe("New Chat");
  });

  it("returns Russian when locale is ru", () => {
    setLocale("ru");
    expect(getLocale()).toBe("ru");
    expect(t("nav.newChatTooltip")).toBe("Новый чат");
  });

  it("interpolates params", () => {
    expect(t("scheduler.jobTitle", { jobId: "daily" })).toBe("Job daily");
    setLocale("ru");
    expect(t("scheduler.jobTitle", { jobId: "daily" })).toBe(
      "Задача daily",
    );
  });

  it("falls back to English for missing ru key", () => {
    setLocale("ru");
    expect(t("nonexistent.key.xyz")).toBe("nonexistent.key.xyz");
  });

  it("setLocale rejects unknown ids", () => {
    expect(setLocale("de")).toBe(false);
    expect(getLocale()).toBe("en");
  });

  it("pluralCategories reports the CLDR categories each locale can produce", () => {
    expect([...pluralCategories("en")].sort()).toEqual(["one", "other"]);
    expect([...pluralCategories("ru")].sort()).toEqual([
      "few",
      "many",
      "one",
      "other",
    ]);
  });

  it("translatePlural picks the category for the active locale", () => {
    setLocale("en");
    expect(translatePlural("prompts.permissionMeta.lines", 1)).toBe("1 line");
    expect(translatePlural("prompts.permissionMeta.lines", 2)).toBe("2 lines");
    expect(translatePlural("prompts.permissionMeta.lines", 40)).toBe("40 lines");
  });

  it("translatePlural declines russian counts by CLDR category, not by count === 1", () => {
    setLocale("ru");
    // one / few / many, including the 11-14 exception and the 21 wrap-around.
    expect(translatePlural("prompts.permissionMeta.lines", 1)).toBe("1 строка");
    expect(translatePlural("prompts.permissionMeta.lines", 3)).toBe("3 строки");
    expect(translatePlural("prompts.permissionMeta.lines", 5)).toBe("5 строк");
    expect(translatePlural("prompts.permissionMeta.lines", 11)).toBe("11 строк");
    expect(translatePlural("prompts.permissionMeta.lines", 21)).toBe("21 строка");
    expect(translatePlural("tasks.chip.total", 2)).toBe("2 фоновые задачи");
  });

  it("translatePlural falls back to english, then to the key", () => {
    setLocale("ru");
    expect(translatePlural("definitely.not.a.family", 3)).toBe(
      "definitely.not.a.family",
    );
  });
});
