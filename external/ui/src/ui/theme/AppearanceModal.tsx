import { useCallback, useEffect, useState, useSyncExternalStore } from "react";
import { useT } from "../i18n/I18nProvider";
import { getLocale, onLocaleChange, themeLabel } from "../i18n/i18n";
import { UI_LOCALES, UI_LOCALE_IDS } from "../i18n/locales";
import {
  persistUiLocalePreference,
  readUiLocaleFromConfigDoc,
  type UiLocalePreference,
} from "../i18n/localeConfig";
import {
  UI_THEME_IDS,
  LIGHT_THEMES,
  type UiThemeMode,
} from "./themeCookie";
import { readAppliedUiTheme, setUiTheme } from "./uiTheme";

function subscribeTheme(onStoreChange: () => void): () => void {
  const obs = new MutationObserver(onStoreChange);
  obs.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ["data-theme"],
  });
  return () => obs.disconnect();
}

/** Accent colours shown in each theme's swatch — approximates the CSS --accent + canvas background. */
const SWATCH_COLORS: Record<UiThemeMode, { bg: string; accent: string; text: string }> = {
  dark:             { bg: "#121212", accent: "#9333ea", text: "#ffffff" },
  light:            { bg: "#f8f8fa", accent: "#7c3aed", text: "#18181b" },
  midnight:         { bg: "#0d1117", accent: "#5865f2", text: "#e6edf3" },
  "solarized-dark": { bg: "#002b36", accent: "#268bd2", text: "#839496" },
  monokai:          { bg: "#272822", accent: "#fd971f", text: "#f8f8f2" },
  nord:             { bg: "#2e3440", accent: "#88c0d0", text: "#eceff4" },
  "rose-pine":      { bg: "#191724", accent: "#c4a7e7", text: "#e0def4" },
};

function ThemeSwatch(props: {
  id: UiThemeMode;
  active: boolean;
  onClick: () => void;
  label: string;
}) {
  const { id, active, onClick, label } = props;
  const colors = SWATCH_COLORS[id];
  const isLight = LIGHT_THEMES.has(id);

  return (
    <button
      type="button"
      className={`appearance-swatch${active ? " is-active" : ""}`}
      aria-pressed={active}
      aria-label={label}
      data-testid={`theme-swatch-${id}`}
      onClick={onClick}
    >
      <span
        className="appearance-swatch-preview"
        style={{ background: colors.bg }}
        aria-hidden
      >
        <span
          className="appearance-swatch-bar"
          style={{
            background: isLight
              ? "rgba(0,0,0,0.06)"
              : "rgba(255,255,255,0.06)",
          }}
        />
        <span
          className="appearance-swatch-dot"
          style={{ background: colors.accent }}
        />
        <span
          className="appearance-swatch-lines"
          aria-hidden
        >
          <span style={{ background: colors.text, opacity: 0.45, width: "60%" }} />
          <span style={{ background: colors.text, opacity: 0.25, width: "40%" }} />
          <span style={{ background: colors.accent, opacity: 0.55, width: "50%" }} />
        </span>
      </span>
      <span className="appearance-swatch-label" style={{ color: colors.text, background: colors.bg }}>
        {label}
      </span>
    </button>
  );
}

function asUiObject(doc: Record<string, unknown>): Record<string, unknown> {
  const ui = doc.ui;
  if (ui && typeof ui === "object" && !Array.isArray(ui)) {
    return ui as Record<string, unknown>;
  }
  return {};
}

/**
 * AppearanceLanguagePicker renders the UI language select under the theme grid —
 * the single language switcher for the whole application (browser, desktop, and
 * the VS Code / IntelliJ plugin webviews all follow the persisted config value).
 *
 * Unlike the theme picker beside it, this one is not client-side only: the choice
 * persists to `ui.locale` in config.yaml, which is what the editor plugins read.
 * When the Settings screen has the config doc loaded, the picker reads the
 * preference from it and mirrors picks back via `setDoc`, so a later footer Save
 * does not overwrite ui.locale with a stale value. Without a doc (schema still
 * loading) it falls back to fetching the config itself.
 */
export function AppearanceLanguagePicker(props: {
  // Explicitly `| undefined`: AppearanceModal passes its own optional props
  // straight through, which exactOptionalPropertyTypes treats as a real undefined.
  doc?: Record<string, unknown> | undefined;
  setDoc?: ((next: Record<string, unknown>) => void) | undefined;
}) {
  const { t } = useT();
  const activeLocale = useSyncExternalStore(onLocaleChange, getLocale, () => "en");
  const docLoaded = !!props.doc && Object.keys(props.doc).length > 0;
  const [fetchedPref, setFetchedPref] = useState<UiLocalePreference>("");
  const [fetchLoaded, setFetchLoaded] = useState(false);

  useEffect(() => {
    if (docLoaded) {
      return;
    }
    let cancelled = false;
    void (async () => {
      try {
        const res = await fetch("/foxxycode/config");
        if (!res.ok) {
          return;
        }
        const doc = (await res.json()) as Record<string, unknown>;
        if (!cancelled) {
          setFetchedPref(readUiLocaleFromConfigDoc(doc));
          setFetchLoaded(true);
        }
      } catch {
        if (!cancelled) {
          setFetchLoaded(true);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [docLoaded]);

  const pref = docLoaded ? readUiLocaleFromConfigDoc(props.doc) : fetchedPref;
  const loaded = docLoaded || fetchLoaded;

  const { doc, setDoc } = props;
  const pick = useCallback(
    (next: UiLocalePreference) => {
      setFetchedPref(next);
      void persistUiLocalePreference(next);
      if (doc && setDoc && Object.keys(doc).length > 0) {
        setDoc({ ...doc, ui: { ...asUiObject(doc), locale: next } });
      }
    },
    [doc, setDoc],
  );

  const options: { value: string; label: string }[] = [
    { value: "auto", label: t("appearance.locale.auto") },
    ...UI_LOCALE_IDS.map((id) => ({
      value: id,
      label: t(UI_LOCALES[id].labelKey),
    })),
  ];
  // Until the preference is known the select mirrors the active locale, so it
  // never renders "Auto" over a UI that is visibly in one language.
  const selected = loaded ? pref || "auto" : activeLocale;

  return (
    <div
      className="appearance-lang-block"
      data-testid="appearance-language-picker"
    >
      <label
        className="appearance-section-label"
        htmlFor="appearance-language-select"
      >
        {t("appearance.languageLabel")}
      </label>
      <div className="appearance-language-select-wrap">
        <select
          id="appearance-language-select"
          className="appearance-language-select"
          value={selected}
          data-testid="appearance-language-select"
          onChange={(event) => {
            const next = event.currentTarget.value;
            pick(next === "auto" ? "" : (next as UiLocalePreference));
          }}
        >
          {options.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
      </div>
    </div>
  );
}

/** AppearanceThemePicker renders the theme swatch grid (no panel chrome) so it can
 * be embedded as a Settings tab. Theme selection applies immediately and is
 * client-side only (no config save). The language picker sits right under it and
 * does persist, so the config doc is threaded through to it. */
export function AppearanceThemePicker(props: {
  doc?: Record<string, unknown>;
  setDoc?: (next: Record<string, unknown>) => void;
}) {
  const { t } = useT();
  const current = useSyncExternalStore(
    subscribeTheme,
    readAppliedUiTheme,
    () => "dark" as UiThemeMode,
  );

  const pick = useCallback((id: UiThemeMode) => {
    setUiTheme(id);
  }, []);

  return (
    <div className="appearance-sheet-body" data-testid="appearance-theme-picker">
      <p className="appearance-section-label">{t("settings.themePickerLabel")}</p>
      <div
        className="appearance-swatch-grid"
        role="group"
        aria-label={t("settings.themePickerAriaLabel")}
      >
        {UI_THEME_IDS.map((id) => (
          <ThemeSwatch
            key={id}
            id={id}
            active={current === id}
            label={themeLabel(id)}
            onClick={() => pick(id)}
          />
        ))}
      </div>
      <AppearanceLanguagePicker doc={props.doc} setDoc={props.setDoc} />
    </div>
  );
}
