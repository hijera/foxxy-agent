import { expect, test } from "vitest";
import {
  CODDY_UI_THEME_COOKIE,
  UI_THEME_IDS,
  readUiThemeCookie,
  writeUiThemeCookie,
  type UiThemeMode,
} from "./themeCookie";

test("write then read ui theme cookie for all themes", () => {
  Object.defineProperty(window, "location", {
    value: new URL("http://127.0.0.1:5173/"),
    configurable: true,
  });
  for (const id of UI_THEME_IDS) {
    document.cookie = `${CODDY_UI_THEME_COOKIE}=; Max-Age=0; Path=/`;
    writeUiThemeCookie(id);
    expect(readUiThemeCookie()).toBe(id);
  }
});

test("dark and light round-trip", () => {
  document.cookie = `${CODDY_UI_THEME_COOKIE}=; Max-Age=0; Path=/`;
  Object.defineProperty(window, "location", {
    value: new URL("http://127.0.0.1:5173/"),
    configurable: true,
  });
  writeUiThemeCookie("light");
  expect(readUiThemeCookie()).toBe("light");
  writeUiThemeCookie("dark");
  expect(readUiThemeCookie()).toBe("dark");
});

test("invalid cookie value is ignored", () => {
  document.cookie = `${CODDY_UI_THEME_COOKIE}=sepia; Path=/`;
  expect(readUiThemeCookie()).toBeNull();
});

test("UI_THEME_IDS contains exactly 7 entries", () => {
  expect(UI_THEME_IDS).toHaveLength(7);
  const expected: UiThemeMode[] = [
    "dark",
    "light",
    "midnight",
    "solarized-dark",
    "monokai",
    "nord",
    "rose-pine",
  ];
  expect(UI_THEME_IDS).toEqual(expected);
});
