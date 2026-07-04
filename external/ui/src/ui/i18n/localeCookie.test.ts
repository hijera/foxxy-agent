import { afterEach, beforeEach, describe, expect, it } from "vitest";
import {
  CODDY_UI_LANG_COOKIE,
  readUiLocaleCookie,
  writeUiLocaleCookie,
} from "./localeCookie";

describe("locale cookie", () => {
  beforeEach(() => {
    document.cookie = `${CODDY_UI_LANG_COOKIE}=; Max-Age=0; Path=/`;
    Object.defineProperty(window, "location", {
      value: new URL("http://127.0.0.1:5173/"),
      configurable: true,
    });
  });

  afterEach(() => {
    document.cookie = `${CODDY_UI_LANG_COOKIE}=; Max-Age=0; Path=/`;
  });

  it("write then read en and ru", () => {
    writeUiLocaleCookie("en");
    expect(readUiLocaleCookie()).toBe("en");
    writeUiLocaleCookie("ru");
    expect(readUiLocaleCookie()).toBe("ru");
  });

  it("invalid cookie value is ignored", () => {
    document.cookie = `${CODDY_UI_LANG_COOKIE}=de; Path=/`;
    expect(readUiLocaleCookie()).toBeNull();
  });
});
