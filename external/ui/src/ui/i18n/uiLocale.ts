import {
  readUiLocaleCookie,
  writeUiLocaleCookie,
  mapSystemLocaleToSupported,
  readNavigatorLanguage,
} from "./localeCookie";
import { isUiLocale, UI_LOCALE_DEFAULT, type UiLocale } from "./locales";

// Re-exported: the registry owns the default, but call sites have always read it
// from here.
export { UI_LOCALE_DEFAULT };

export function resolveUiLocale(stored: UiLocale | null): UiLocale {
  if (stored !== null && isUiLocale(stored)) {
    return stored;
  }
  return UI_LOCALE_DEFAULT;
}

export function applyUiLocale(locale: UiLocale): void {
  if (typeof document === "undefined") {
    return;
  }
  document.documentElement.lang = locale;
}

export function readAppliedUiLocale(): UiLocale {
  if (typeof document === "undefined") {
    return UI_LOCALE_DEFAULT;
  }
  const lang = document.documentElement.lang;
  if (isUiLocale(lang)) {
    return lang;
  }
  return UI_LOCALE_DEFAULT;
}

/** Parse ?lang= from the current URL against the registered locales. */
export function readUiLocaleFromUrl(): UiLocale | null {
  if (typeof window === "undefined") {
    return null;
  }
  const m = window.location.search.match(/[?&]lang=([^&]+)/);
  if (!m) {
    return null;
  }
  const raw = decodeURIComponent((m[1] ?? "").replace(/\+/g, " "));
  if (isUiLocale(raw)) {
    return raw;
  }
  return null;
}

export function bootstrapUiLocaleFromUrlOrCookie(): UiLocale {
  const fromUrl = readUiLocaleFromUrl();
  if (fromUrl !== null) {
    writeUiLocaleCookie(fromUrl);
    applyUiLocale(fromUrl);
    return fromUrl;
  }
  const stored = readUiLocaleCookie();
  const mode =
    stored !== null
      ? resolveUiLocale(stored)
      : mapSystemLocaleToSupported(readNavigatorLanguage());
  applyUiLocale(mode);
  return mode;
}

export function setUiLocale(locale: UiLocale): void {
  writeUiLocaleCookie(locale);
  applyUiLocale(locale);
}
