import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { initLocale } from "../i18n/i18n";
import { installFoxxyUiApi } from "./foxxyUiApi";
import { UI_THEME_IDS } from "./themeCookie";

function clearThemeState() {
  delete window.foxxyUi;
  delete document.documentElement.dataset.theme;
  document.documentElement.style.colorScheme = "";
  document.documentElement.lang = "en";
  document.cookie = "coddy_ui_theme=; Path=/; Max-Age=0";
  document.cookie = "coddy_ui_lang=; Path=/; Max-Age=0";
}

describe("window.foxxyUi", () => {
  beforeEach(() => {
    clearThemeState();
    initLocale("en");
    installFoxxyUiApi();
  });

  afterEach(() => {
    clearThemeState();
  });

  it("installs a versioned API and is idempotent", () => {
    expect(window.foxxyUi?.version).toBe(1);
    const first = window.foxxyUi;
    installFoxxyUiApi();
    expect(window.foxxyUi).toBe(first);
  });

  it("setTheme applies data-theme, color-scheme, and the cookie", () => {
    expect(window.foxxyUi!.setTheme("light")).toBe(true);
    expect(document.documentElement.dataset.theme).toBe("light");
    expect(document.documentElement.style.colorScheme).toBe("light");
    expect(document.cookie).toContain("coddy_ui_theme=light");

    expect(window.foxxyUi!.setTheme("nord")).toBe(true);
    expect(document.documentElement.dataset.theme).toBe("nord");
    expect(document.documentElement.style.colorScheme).toBe("dark");
    expect(document.cookie).toContain("coddy_ui_theme=nord");
  });

  it("setTheme rejects unknown ids without mutating anything", () => {
    window.foxxyUi!.setTheme("light");
    expect(window.foxxyUi!.setTheme("bogus")).toBe(false);
    expect(document.documentElement.dataset.theme).toBe("light");
    expect(document.cookie).toContain("coddy_ui_theme=light");
  });

  it("getTheme reads the applied theme", () => {
    expect(window.foxxyUi!.getTheme()).toBe("dark"); // default
    window.foxxyUi!.setTheme("rose-pine");
    expect(window.foxxyUi!.getTheme()).toBe("rose-pine");
  });

  it("getThemes returns all valid ids in display order", () => {
    expect(window.foxxyUi!.getThemes()).toEqual(UI_THEME_IDS);
  });

  it("onThemeChange fires on changes from any source and unsubscribes", async () => {
    const seen: string[] = [];
    const off = window.foxxyUi!.onThemeChange((theme) => seen.push(theme));

    window.foxxyUi!.setTheme("light");
    // MutationObserver callbacks are async (microtask).
    await Promise.resolve();
    expect(seen).toEqual(["light"]);

    // Change from outside the API (e.g. ThemeToggle) is observed too.
    document.documentElement.dataset.theme = "monokai";
    await Promise.resolve();
    expect(seen).toEqual(["light", "monokai"]);

    off();
    window.foxxyUi!.setTheme("dark");
    await Promise.resolve();
    expect(seen).toEqual(["light", "monokai"]);
  });

  it("setLocale applies document.lang and cookie", () => {
    expect(window.foxxyUi!.setLocale("ru")).toBe(true);
    expect(document.documentElement.lang).toBe("ru");
    expect(document.cookie).toContain("coddy_ui_lang=ru");
    expect(window.foxxyUi!.getLocale()).toBe("ru");
  });

  it("setLocale rejects unknown ids", () => {
    window.foxxyUi!.setLocale("en");
    expect(window.foxxyUi!.setLocale("de")).toBe(false);
    expect(window.foxxyUi!.getLocale()).toBe("en");
  });

  it("onLocaleChange fires and unsubscribes", () => {
    const seen: string[] = [];
    const off = window.foxxyUi!.onLocaleChange((l) => seen.push(l));
    window.foxxyUi!.setLocale("ru");
    expect(seen).toEqual(["ru"]);
    off();
    window.foxxyUi!.setLocale("en");
    expect(seen).toEqual(["ru"]);
  });
});
