import {
  LIGHT_THEMES,
  UI_THEME_IDS,
  readUiThemeCookie,
  type UiThemeMode,
  writeUiThemeCookie,
} from "./themeCookie";

export const UI_THEME_DEFAULT: UiThemeMode = "dark";

export function resolveUiThemeMode(stored: UiThemeMode | null): UiThemeMode {
  if (stored !== null && (UI_THEME_IDS as string[]).includes(stored)) {
    return stored;
  }
  return UI_THEME_DEFAULT;
}

export function applyUiTheme(mode: UiThemeMode): void {
  if (typeof document === "undefined") {
    return;
  }
  document.documentElement.dataset.theme = mode;
  document.documentElement.style.colorScheme = LIGHT_THEMES.has(mode)
    ? "light"
    : "dark";
}

export function readAppliedUiTheme(): UiThemeMode {
  if (typeof document === "undefined") {
    return UI_THEME_DEFAULT;
  }
  const t = document.documentElement.dataset.theme as string | undefined;
  if (t && (UI_THEME_IDS as string[]).includes(t)) {
    return t as UiThemeMode;
  }
  return UI_THEME_DEFAULT;
}

export function bootstrapUiThemeFromCookie(): UiThemeMode {
  const mode = resolveUiThemeMode(readUiThemeCookie());
  applyUiTheme(mode);
  return mode;
}

export function setUiTheme(mode: UiThemeMode): void {
  writeUiThemeCookie(mode);
  applyUiTheme(mode);
}
