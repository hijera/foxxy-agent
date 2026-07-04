import { useCallback, useSyncExternalStore } from "react";
import { useT } from "../i18n/I18nProvider";
import type { UiThemeMode } from "./themeCookie";
import { readAppliedUiTheme, setUiTheme } from "./uiTheme";

function subscribeTheme(onStoreChange: () => void): () => void {
  const obs = new MutationObserver(onStoreChange);
  obs.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ["data-theme"],
  });
  return () => obs.disconnect();
}

export function ThemeToggle() {
  const { t } = useT();
  const mode = useSyncExternalStore(
    subscribeTheme,
    readAppliedUiTheme,
    () => "dark" as UiThemeMode,
  );

  const pick = useCallback((next: UiThemeMode) => {
    setUiTheme(next);
  }, []);

  return (
    <div className="settings-theme-block" data-testid="theme-toggle">
      <span className="settings-label" id="settings-theme-label">
        {t("settings.appearanceLabel")}
      </span>
      <div
        className="settings-theme-segment"
        role="group"
        aria-labelledby="settings-theme-label"
      >
        <button
          type="button"
          className={`settings-theme-option${mode === "dark" ? " is-active" : ""}`}
          data-testid="theme-toggle-dark"
          aria-pressed={mode === "dark"}
          onClick={() => pick("dark")}
        >
          {t("settings.themeDark")}
        </button>
        <button
          type="button"
          className={`settings-theme-option${mode === "light" ? " is-active" : ""}`}
          data-testid="theme-toggle-light"
          aria-pressed={mode === "light"}
          onClick={() => pick("light")}
        >
          {t("settings.themeLight")}
        </button>
      </div>
    </div>
  );
}
