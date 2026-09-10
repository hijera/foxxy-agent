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

const pluralRulesByLocale = new Map<UiLocale, Intl.PluralRules>();

function pluralRulesFor(locale: UiLocale): Intl.PluralRules {
  let rules = pluralRulesByLocale.get(locale);
  if (!rules) {
    rules = new Intl.PluralRules(locale);
    pluralRulesByLocale.set(locale, rules);
  }
  return rules;
}

/**
 * CLDR plural categories a locale actually uses: ["one", "other"] for English,
 * ["one", "few", "many", "other"] for Russian. The dictionary contract derives from this
 * -- a plural key must exist in every category its own locale can produce.
 */
export function pluralCategories(locale: UiLocale): string[] {
  return [...pluralRulesFor(locale).resolvedOptions().pluralCategories];
}

/**
 * Translate a count-dependent key. The dictionary holds one entry per CLDR category under
 * `key.category` ("tasks.chip.running.one" / ".few" / ".many" / ".other"), so each locale
 * declines the noun itself instead of the component choosing between two fixed strings.
 * `{count}` is always available to the entry; extra params interpolate as usual.
 */
export function translatePlural(
  key: string,
  count: number,
  params?: TranslateParams,
): string {
  const merged: TranslateParams = { ...params, count };
  const primary = dictFor(currentLocale);
  const fallbackDict = UI_LOCALES[UI_LOCALE_DEFAULT].messages;
  const category = pluralRulesFor(currentLocale).select(count);
  const defaultCategory = pluralRulesFor(UI_LOCALE_DEFAULT).select(count);
  const raw =
    primary[`${key}.${category}`] ??
    primary[`${key}.other`] ??
    fallbackDict[`${key}.${defaultCategory}`] ??
    fallbackDict[`${key}.other`];
  return raw !== undefined ? interpolate(raw, merged) : key;
}

/** Shorthand alias for translatePlural, mirroring `t`. */
export const tp = translatePlural;

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
