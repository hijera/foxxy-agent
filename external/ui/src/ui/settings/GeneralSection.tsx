import { useCallback, useEffect, useSyncExternalStore, useState } from "react";
import { useT } from "../i18n/I18nProvider";
import {
  persistUiLocalePreference,
  readUiLocaleFromConfigDoc,
  type UiLocalePreference,
} from "../i18n/localeConfig";
import { getLocale, onLocaleChange } from "../i18n/i18n";
import {
  getSendMode,
  onSendModeChange,
  persistSendModePreference,
  readSendModeFromConfigDoc,
  DEFAULT_SEND_MODE,
  type SendMode,
} from "../i18n/sendModeConfig";

function asUiObject(doc: Record<string, unknown>): Record<string, unknown> {
  const ui = doc.ui;
  if (ui && typeof ui === "object" && !Array.isArray(ui)) {
    return ui as Record<string, unknown>;
  }
  return {};
}

/**
 * GeneralLocalePicker renders the UI language segmented control — the single
 * language switcher for the whole application (browser, desktop, and the
 * VS Code / IntelliJ plugin webviews all follow the persisted config value).
 *
 * When the Settings screen has the config doc loaded, the picker reads the
 * preference from it and mirrors picks back via `setDoc`, so a later footer
 * Save does not overwrite ui.locale with a stale value. Without a doc (schema
 * still loading) it falls back to fetching the config itself.
 */
export function GeneralLocalePicker(props: {
  doc?: Record<string, unknown>;
  setDoc?: (next: Record<string, unknown>) => void;
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

  const options: { id: UiLocalePreference; label: string }[] = [
    { id: "", label: t("settings.locale.auto") },
    { id: "en", label: t("settings.locale.en") },
    { id: "ru", label: t("settings.locale.ru") },
  ];

  return (
    <div className="appearance-sheet-body" data-testid="general-locale-picker">
      <p className="appearance-section-label">{t("settings.general.locale")}</p>
      <div
        className="appearance-locale-row"
        role="group"
        aria-label={t("settings.general.locale")}
      >
        {options.map((opt) => (
          <button
            key={opt.id || "auto"}
            type="button"
            className={[
              "appearance-locale-btn",
              loaded && pref === opt.id ? "is-active" : "",
              !loaded && activeLocale === opt.id ? "is-active" : "",
            ]
              .filter(Boolean)
              .join(" ")}
            aria-pressed={loaded ? pref === opt.id : activeLocale === opt.id}
            data-testid={`locale-pref-${opt.id || "auto"}`}
            onClick={() => pick(opt.id)}
          >
            {opt.label}
          </button>
        ))}
      </div>
    </div>
  );
}

/**
 * GeneralSendModePicker renders the segmented control that chooses how the main
 * composer submits a message (ui.send_mode). Mirrors GeneralLocalePicker: reads
 * the preference from the loaded config doc when present (mirroring picks back
 * via setDoc so a later footer Save keeps them), else fetches config itself.
 */
export function GeneralSendModePicker(props: {
  doc?: Record<string, unknown>;
  setDoc?: (next: Record<string, unknown>) => void;
}) {
  const { t } = useT();
  const activeMode = useSyncExternalStore(
    onSendModeChange,
    getSendMode,
    () => DEFAULT_SEND_MODE,
  );
  const docLoaded = !!props.doc && Object.keys(props.doc).length > 0;
  const [fetchedMode, setFetchedMode] = useState<SendMode>(DEFAULT_SEND_MODE);
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
          setFetchedMode(readSendModeFromConfigDoc(doc));
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

  const mode = docLoaded ? readSendModeFromConfigDoc(props.doc) : fetchedMode;
  const loaded = docLoaded || fetchLoaded;

  const { doc, setDoc } = props;
  const pick = useCallback(
    (next: SendMode) => {
      setFetchedMode(next);
      void persistSendModePreference(next);
      if (doc && setDoc && Object.keys(doc).length > 0) {
        setDoc({ ...doc, ui: { ...asUiObject(doc), send_mode: next } });
      }
    },
    [doc, setDoc],
  );

  const options: { id: SendMode; label: string }[] = [
    { id: "enter", label: t("settings.sendMode.enter") },
    { id: "ctrl_enter", label: t("settings.sendMode.ctrlEnter") },
    { id: "off", label: t("settings.sendMode.off") },
  ];
  const effective = loaded ? mode : activeMode;

  return (
    <div className="appearance-sheet-body" data-testid="general-send-mode-picker">
      <p className="appearance-section-label">{t("settings.general.sendMode")}</p>
      <div
        className="appearance-locale-row"
        role="group"
        aria-label={t("settings.general.sendMode")}
      >
        {options.map((opt) => (
          <button
            key={opt.id}
            type="button"
            className={[
              "appearance-locale-btn",
              effective === opt.id ? "is-active" : "",
            ]
              .filter(Boolean)
              .join(" ")}
            aria-pressed={effective === opt.id}
            data-testid={`send-mode-${opt.id}`}
            onClick={() => pick(opt.id)}
          >
            {opt.label}
          </button>
        ))}
      </div>
    </div>
  );
}
