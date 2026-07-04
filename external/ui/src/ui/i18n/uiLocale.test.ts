import { afterEach, beforeEach, describe, expect, it } from "vitest";
import {
  bootstrapUiLocaleFromUrlOrCookie,
  readUiLocaleFromUrl,
} from "./uiLocale";
import { CODDY_UI_LANG_COOKIE } from "./localeCookie";

describe("uiLocale bootstrap", () => {
  beforeEach(() => {
    document.cookie = `${CODDY_UI_LANG_COOKIE}=; Max-Age=0; Path=/`;
    document.documentElement.lang = "en";
  });

  afterEach(() => {
    document.cookie = `${CODDY_UI_LANG_COOKIE}=; Max-Age=0; Path=/`;
    document.documentElement.lang = "en";
  });

  it("reads lang from URL query", () => {
    Object.defineProperty(window, "location", {
      value: new URL("http://127.0.0.1/?lang=ru"),
      configurable: true,
    });
    expect(readUiLocaleFromUrl()).toBe("ru");
    expect(bootstrapUiLocaleFromUrlOrCookie()).toBe("ru");
    expect(document.documentElement.lang).toBe("ru");
  });

  it("falls back to cookie then default en", () => {
    Object.defineProperty(window, "location", {
      value: new URL("http://127.0.0.1/"),
      configurable: true,
    });
    document.cookie = `${CODDY_UI_LANG_COOKIE}=ru; Path=/`;
    expect(bootstrapUiLocaleFromUrlOrCookie()).toBe("ru");
  });
});
