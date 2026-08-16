import {
  isUiLocale,
  UI_LOCALES,
  UI_LOCALE_DEFAULT,
  type UiLocale,
} from "./locales";
import { setUiLocale as applyLocale } from "./uiLocale";

export type TranslateParams = Record<string, string | number>;

let currentLocale: UiLocale = UI_LOCALE_DEFAULT;
const listeners = new Set<() => void>();

function dictFor(locale: UiLocale): Record<string, string> {
  return UI_LOCALES[locale].messages;
}

function interpolate(
  raw: string,
  params?: TranslateParams,
): string {
  if (!params) {
    return raw;
  }
  let out = raw;
  for (const [key, value] of Object.entries(params)) {
    out = out.replace(new RegExp(`\\{${key}\\}`, "g"), String(value));
  }
  return out;
}

/** Current active UI locale. */
export function getLocale(): UiLocale {
  return currentLocale;
}

/** Subscribe to locale changes (e.g. React provider). Returns unsubscribe. */
export function onLocaleChange(cb: () => void): () => void {
  listeners.add(cb);
  return () => {
    listeners.delete(cb);
  };
}

function notifyLocaleChange(): void {
  for (const cb of listeners) {
    cb();
  }
}

/** Initialize locale from bootstrap (call once at startup). */
export function initLocale(locale: UiLocale): void {
  currentLocale = locale;
}

/**
 * Switch locale, persist cookie, update document.lang, notify subscribers.
 * Returns false for unsupported locale ids.
 */
export function setLocale(lang: string): boolean {
  if (!isUiLocale(lang)) {
    return false;
  }
  if (currentLocale !== lang) {
    currentLocale = lang;
    applyLocale(lang);
    notifyLocaleChange();
  } else {
    applyLocale(lang);
  }
  return true;
}

/** Translate a key for the current locale; falls back to English then the key. */
export function translate(
  key: string,
  params?: TranslateParams,
): string {
  const primary = dictFor(currentLocale)[key];
  if (primary !== undefined) {
    return interpolate(primary, params);
  }
  const fallback = dictFor(UI_LOCALE_DEFAULT)[key];
  if (fallback !== undefined) {
    return interpolate(fallback, params);
  }
  return key;
}

/** Shorthand alias used by hooks and non-React code. */
export const t = translate;

/** Theme label helper keyed by theme id. */
export function themeLabel(themeId: string): string {
  const map: Record<string, string> = {
    dark: translate("theme.dark"),
    light: translate("theme.light"),
    midnight: translate("theme.midnight"),
    "solarized-dark": translate("theme.solarizedDark"),
    monokai: translate("theme.monokai"),
    nord: translate("theme.nord"),
    "rose-pine": translate("theme.rosePine"),
  };
  return map[themeId] ?? themeId;
}

/** Hero title verb keys in display order. */
export const HERO_VERB_KEYS = [
  "chat.heroVerb.know",
  "chat.heroVerb.build",
  "chat.heroVerb.find",
  "chat.heroVerb.research",
  "chat.heroVerb.explore",
  "chat.heroVerb.debug",
  "chat.heroVerb.ship",
  "chat.heroVerb.design",
  "chat.heroVerb.learn",
  "chat.heroVerb.automate",
  "chat.heroVerb.refactor",
  "chat.heroVerb.plan",
] as const;

export function heroVerbs(): string[] {
  return HERO_VERB_KEYS.map((k) => translate(k));
}
