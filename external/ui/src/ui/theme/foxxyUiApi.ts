import { UI_THEME_IDS, type UiThemeMode } from "./themeCookie";
import { readAppliedUiTheme, setUiTheme } from "./uiTheme";
import {
  getLocale,
  onLocaleChange,
  setLocale as setUiI18nLocale,
} from "../i18n/i18n";
import type { UiLocale } from "../i18n/localeCookie";

/**
 * Stable global API for host embeddings (IntelliJ/PhpStorm plugin via JCEF).
 * The plugin calls it with `JBCefBrowser.getCefBrowser().executeJavaScript()`,
 * e.g. on a LafManagerListener event:
 *
 *   window.foxxyUi && window.foxxyUi.setTheme('light')
 *   window.foxxyUi && window.foxxyUi.setLocale('ru')
 *
 * See docs/intellij-embedding.md for the embedding contract.
 */
export type FoxxyUiApi = {
  readonly version: 1;
  /** Applies + persists a theme. Returns false (and changes nothing) on unknown ids. */
  setTheme(theme: string): boolean;
  getTheme(): UiThemeMode;
  /** All valid theme ids, in display order. */
  getThemes(): readonly UiThemeMode[];
  /** Fires on every theme change regardless of source. Returns unsubscribe. */
  onThemeChange(cb: (theme: UiThemeMode) => void): () => void;
  /** Applies + persists UI locale ("en" | "ru"). Returns false on unknown ids. */
  setLocale(locale: string): boolean;
  getLocale(): UiLocale;
  /** Fires on every locale change regardless of source. Returns unsubscribe. */
  onLocaleChange(cb: (locale: UiLocale) => void): () => void;
};

declare global {
  interface Window {
    foxxyUi?: FoxxyUiApi;
  }
}

function isValidTheme(v: string): v is UiThemeMode {
  return (UI_THEME_IDS as string[]).includes(v);
}

export function installFoxxyUiApi(): void {
  if (typeof window === "undefined" || typeof document === "undefined") {
    return;
  }
  if (window.foxxyUi?.version === 1) {
    return; // idempotent
  }

  const themeListeners = new Set<(theme: UiThemeMode) => void>();
  const localeListeners = new Set<(locale: UiLocale) => void>();
  let observer: MutationObserver | null = null;

  // One shared observer on data-theme catches every change source:
  // plugin setTheme(), ThemeToggle, AppearanceModal.
  const syncObserver = () => {
    if (themeListeners.size > 0 && observer === null) {
      observer = new MutationObserver(() => {
        const theme = readAppliedUiTheme();
        for (const cb of themeListeners) {
          cb(theme);
        }
      });
      observer.observe(document.documentElement, {
        attributes: true,
        attributeFilter: ["data-theme"],
      });
    } else if (themeListeners.size === 0 && observer !== null) {
      observer.disconnect();
      observer = null;
    }
  };

  onLocaleChange(() => {
    const locale = getLocale();
    for (const cb of localeListeners) {
      cb(locale);
    }
  });

  window.foxxyUi = {
    version: 1,
    setTheme(theme: string): boolean {
      if (!isValidTheme(theme)) {
        return false;
      }
      setUiTheme(theme);
      return true;
    },
    getTheme(): UiThemeMode {
      return readAppliedUiTheme();
    },
    getThemes(): readonly UiThemeMode[] {
      return [...UI_THEME_IDS];
    },
    onThemeChange(cb: (theme: UiThemeMode) => void): () => void {
      themeListeners.add(cb);
      syncObserver();
      return () => {
        themeListeners.delete(cb);
        syncObserver();
      };
    },
    setLocale(locale: string): boolean {
      return setUiI18nLocale(locale);
    },
    getLocale(): UiLocale {
      return getLocale();
    },
    onLocaleChange(cb: (locale: UiLocale) => void): () => void {
      localeListeners.add(cb);
      return () => {
        localeListeners.delete(cb);
      };
    },
  };
}
