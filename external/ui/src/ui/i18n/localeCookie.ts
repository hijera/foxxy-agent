import {
  isUiLocale,
  UI_LOCALE_DEFAULT,
  UI_LOCALE_IDS,
  type UiLocale,
} from "./locales";

export const FOXXYCODE_UI_LANG_COOKIE = "foxxycode_ui_lang";

const MAX_AGE_SECONDS = 365 * 24 * 60 * 60;

// Re-exported so the many call sites that already import the locale type and id
// list from here keep working now that the registry owns both.
export { UI_LOCALE_IDS, type UiLocale };

export function readUiLocaleCookie(): UiLocale | null {
  if (typeof document === "undefined") {
    return null;
  }
  const parts = document.cookie.split(";");
  for (const p of parts) {
    const s = p.trim();
    if (!s.startsWith(`${FOXXYCODE_UI_LANG_COOKIE}=`)) {
      continue;
    }
    const v = decodeURIComponent(
      s.slice(FOXXYCODE_UI_LANG_COOKIE.length + 1).trim(),
    );
    if (isUiLocale(v)) {
      return v;
    }
    return null;
  }
  return null;
}

export function mapSystemLocaleToSupported(lang: string): UiLocale {
  const normalized = lang.trim().toLowerCase().replace(/_/g, "-");
  if (isUiLocale(normalized)) {
    return normalized;
  }
  const base = normalized.split("-")[0] ?? "";
  if (isUiLocale(base)) {
    return base;
  }
  return UI_LOCALE_DEFAULT;
}

export function readNavigatorLanguage(): string {
  if (typeof navigator === "undefined") {
    return "en";
  }
  return navigator.language || navigator.languages?.[0] || "en";
}

export function writeUiLocaleCookie(locale: UiLocale): void {
  if (typeof document === "undefined") {
    return;
  }
  const secure =
    typeof window !== "undefined" && window.location.protocol === "https:"
      ? "; Secure"
      : "";
  document.cookie = `${FOXXYCODE_UI_LANG_COOKIE}=${encodeURIComponent(locale)}; Path=/; Max-Age=${MAX_AGE_SECONDS}; SameSite=Lax${secure}`;
}

/**
 * Drop the stored locale so the next bootstrap falls back to navigator.language.
 * "Auto" in the picker means exactly that, and leaving a stale cookie behind
 * would pin the previously chosen language across a reload.
 */
export function clearUiLocaleCookie(): void {
  if (typeof document === "undefined") {
    return;
  }
  document.cookie = `${FOXXYCODE_UI_LANG_COOKIE}=; Path=/; Max-Age=0; SameSite=Lax`;
}
