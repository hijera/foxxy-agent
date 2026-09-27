import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { LANG_KEY, THEME_KEY } from "../../scripts/lib/boot.mjs";
import en from "../i18n/en.json";
import ru from "../i18n/ru.json";

export type Lang = "en" | "ru";
export type Theme = "dark" | "light";
export type Messages = typeof en;
export type Localized<T = string> = { en: T; ru: T };

const dictionaries: Record<Lang, Messages> = { en, ru };

// The head boot script (scripts/lib/boot.mjs) has already chosen both on <html>.
function initialLang(): Lang {
  return document.documentElement.lang === "ru" ? "ru" : "en";
}

function initialTheme(): Theme {
  return document.documentElement.dataset.theme === "light" ? "light" : "dark";
}

function store(key: string, value: string) {
  try {
    window.localStorage.setItem(key, value);
  } catch {
    // Private windows and blocked storage: the choice lasts for this page only.
  }
}

function storedTheme(): string | null {
  try {
    return window.localStorage.getItem(THEME_KEY);
  } catch {
    return null;
  }
}

interface Prefs {
  lang: Lang;
  theme: Theme;
  m: Messages;
  setLang: (lang: Lang) => void;
  toggleTheme: () => void;
}

const PrefsContext = createContext<Prefs | null>(null);

export function PrefsProvider({ page, children }: { page: keyof Messages["meta"]; children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(initialLang);
  const [theme, setTheme] = useState<Theme>(initialTheme);
  const m = dictionaries[lang];

  useEffect(() => {
    const root = document.documentElement;
    root.lang = lang;
    const meta = m.meta[page];
    document.title = meta.title;
    document.querySelector('meta[name="description"]')?.setAttribute("content", meta.description);
  }, [lang, m, page]);

  useEffect(() => {
    const root = document.documentElement;
    root.dataset.theme = theme;
    root.style.colorScheme = theme;
  }, [theme]);

  // Until the visitor picks a theme, follow the system one as it changes.
  useEffect(() => {
    if (typeof window.matchMedia !== "function") return;
    const query = window.matchMedia("(prefers-color-scheme: light)");
    const onChange = () => {
      if (storedTheme() === null) setTheme(query.matches ? "light" : "dark");
    };
    query.addEventListener("change", onChange);
    return () => query.removeEventListener("change", onChange);
  }, []);

  const setLang = useCallback((next: Lang) => {
    store(LANG_KEY, next);
    setLangState(next);
    const url = new URL(window.location.href);
    if (url.searchParams.has("lang")) {
      url.searchParams.delete("lang");
      window.history.replaceState(null, "", url);
    }
  }, []);

  const toggleTheme = useCallback(() => {
    setTheme((current) => {
      const next = current === "dark" ? "light" : "dark";
      store(THEME_KEY, next);
      return next;
    });
  }, []);

  const value = useMemo(() => ({ lang, theme, m, setLang, toggleTheme }), [lang, theme, m, setLang, toggleTheme]);
  return <PrefsContext.Provider value={value}>{children}</PrefsContext.Provider>;
}

export function usePrefs(): Prefs {
  const prefs = useContext(PrefsContext);
  if (!prefs) throw new Error("usePrefs outside PrefsProvider");
  return prefs;
}

export function pick<T>(value: Localized<T>, lang: Lang): T {
  return value[lang];
}
