import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { initLocale } from "../i18n/i18n";
import { installFoxxyCodeUiApi } from "./foxxycodeUiApi";
import { UI_THEME_IDS } from "./themeCookie";
import { subscribeFileMention } from "../skills/fileMentionBus";

function clearThemeState() {
  delete window.foxxycodeUi;
  delete document.documentElement.dataset.theme;
  document.documentElement.style.colorScheme = "";
  document.documentElement.lang = "en";
  document.cookie = "foxxycode_ui_theme=; Path=/; Max-Age=0";
  document.cookie = "foxxycode_ui_lang=; Path=/; Max-Age=0";
}

describe("window.foxxycodeUi", () => {
  beforeEach(() => {
    clearThemeState();
    initLocale("en");
    installFoxxyCodeUiApi();
  });

  afterEach(() => {
    clearThemeState();
  });

  it("installs a versioned API and is idempotent", () => {
    expect(window.foxxycodeUi?.version).toBe(1);
    const first = window.foxxycodeUi;
    installFoxxyCodeUiApi();
    expect(window.foxxycodeUi).toBe(first);
  });

  it("setTheme applies data-theme, color-scheme, and the cookie", () => {
    expect(window.foxxycodeUi!.setTheme("light")).toBe(true);
    expect(document.documentElement.dataset.theme).toBe("light");
    expect(document.documentElement.style.colorScheme).toBe("light");
    expect(document.cookie).toContain("foxxycode_ui_theme=light");

    expect(window.foxxycodeUi!.setTheme("nord")).toBe(true);
    expect(document.documentElement.dataset.theme).toBe("nord");
    expect(document.documentElement.style.colorScheme).toBe("dark");
    expect(document.cookie).toContain("foxxycode_ui_theme=nord");
  });

  it("setTheme rejects unknown ids without mutating anything", () => {
    window.foxxycodeUi!.setTheme("light");
    expect(window.foxxycodeUi!.setTheme("bogus")).toBe(false);
    expect(document.documentElement.dataset.theme).toBe("light");
    expect(document.cookie).toContain("foxxycode_ui_theme=light");
  });

  it("getTheme reads the applied theme", () => {
    expect(window.foxxycodeUi!.getTheme()).toBe("dark"); // default
    window.foxxycodeUi!.setTheme("rose-pine");
    expect(window.foxxycodeUi!.getTheme()).toBe("rose-pine");
  });

  it("getThemes returns all valid ids in display order", () => {
    expect(window.foxxycodeUi!.getThemes()).toEqual(UI_THEME_IDS);
  });

  it("onThemeChange fires on changes from any source and unsubscribes", async () => {
    const seen: string[] = [];
    const off = window.foxxycodeUi!.onThemeChange((theme) => seen.push(theme));

    window.foxxycodeUi!.setTheme("light");
    // MutationObserver callbacks are async (microtask).
    await Promise.resolve();
    expect(seen).toEqual(["light"]);

    // Change from outside the API (e.g. ThemeToggle) is observed too.
    document.documentElement.dataset.theme = "monokai";
    await Promise.resolve();
    expect(seen).toEqual(["light", "monokai"]);

    off();
    window.foxxycodeUi!.setTheme("dark");
    await Promise.resolve();
    expect(seen).toEqual(["light", "monokai"]);
  });

  it("setLocale applies document.lang and cookie", () => {
    expect(window.foxxycodeUi!.setLocale("ru")).toBe(true);
    expect(document.documentElement.lang).toBe("ru");
    expect(document.cookie).toContain("foxxycode_ui_lang=ru");
    expect(window.foxxycodeUi!.getLocale()).toBe("ru");
  });

  it("setLocale rejects unknown ids", () => {
    window.foxxycodeUi!.setLocale("en");
    expect(window.foxxycodeUi!.setLocale("de")).toBe(false);
    expect(window.foxxycodeUi!.getLocale()).toBe("en");
  });

  it("onLocaleChange fires and unsubscribes", () => {
    const seen: string[] = [];
    const off = window.foxxycodeUi!.onLocaleChange((l) => seen.push(l));
    window.foxxycodeUi!.setLocale("ru");
    expect(seen).toEqual(["ru"]);
    off();
    window.foxxycodeUi!.setLocale("en");
    expect(seen).toEqual(["ru"]);
  });

  it("insertFileMention forwards a path to the file-mention bus", () => {
    const seen: string[] = [];
    const off = subscribeFileMention((p) => seen.push(p));
    expect(window.foxxycodeUi!.insertFileMention("src/foo.ts")).toBe(true);
    expect(seen).toEqual(["src/foo.ts"]);
    off();
  });

  it("insertFileMention rejects empty/whitespace without emitting", () => {
    const seen: string[] = [];
    const off = subscribeFileMention((p) => seen.push(p));
    expect(window.foxxycodeUi!.insertFileMention("   ")).toBe(false);
    expect(seen).toEqual([]);
    off();
  });
});
