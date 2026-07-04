import { afterEach, beforeEach, describe, expect, it } from "vitest";
import {
  getLocale,
  initLocale,
  setLocale,
  t,
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
});
