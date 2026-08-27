import { useCallback, useEffect, useSyncExternalStore, useState } from "react";
import { useT } from "../i18n/I18nProvider";
import {
  getSendMode,
  onSendModeChange,
  persistSendModePreference,
  readSendModeFromConfigDoc,
  DEFAULT_SEND_MODE,
  type SendMode,
} from "../i18n/sendModeConfig";
import {
  DEFAULT_STATUS_LINE,
  getStatusLineEnabled,
  onStatusLineChange,
  persistStatusLinePreference,
  readStatusLineFromConfigDoc,
} from "../chat/statusLineConfig";

function asUiObject(doc: Record<string, unknown>): Record<string, unknown> {
  const ui = doc.ui;
  if (ui && typeof ui === "object" && !Array.isArray(ui)) {
    return ui as Record<string, unknown>;
  }
  return {};
}

/**
 * GeneralSendModePicker renders the segmented control that chooses how the main
 * composer submits a message (ui.send_mode). It reads the preference from the
 * loaded config doc when present (mirroring picks back via setDoc so a later
 * footer Save keeps them), else fetches config itself. The language picker
 * follows the same shape but lives in the Appearance tab under the theme grid
 * (`theme/AppearanceModal.tsx`).
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

/**
 * GeneralStatusLinePicker toggles the live status line next to the typing dots
 * (ui.status_line). Mirrors GeneralSendModePicker: reads the preference from the loaded
 * config doc when present (mirroring picks back via setDoc so a later footer Save keeps
 * them), else fetches config itself.
 */
export function GeneralStatusLinePicker(props: {
  doc?: Record<string, unknown>;
  setDoc?: (next: Record<string, unknown>) => void;
}) {
  const { t } = useT();
  const activeEnabled = useSyncExternalStore(
    onStatusLineChange,
    getStatusLineEnabled,
    () => DEFAULT_STATUS_LINE,
  );
  const docLoaded = !!props.doc && Object.keys(props.doc).length > 0;
  const [fetchedEnabled, setFetchedEnabled] = useState(DEFAULT_STATUS_LINE);
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
          setFetchedEnabled(readStatusLineFromConfigDoc(doc));
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

  const enabled = docLoaded
    ? readStatusLineFromConfigDoc(props.doc)
    : fetchedEnabled;
  const loaded = docLoaded || fetchLoaded;

  const { doc, setDoc } = props;
  const pick = useCallback(
    (next: boolean) => {
      setFetchedEnabled(next);
      void persistStatusLinePreference(next);
      if (doc && setDoc && Object.keys(doc).length > 0) {
        setDoc({ ...doc, ui: { ...asUiObject(doc), status_line: next } });
      }
    },
    [doc, setDoc],
  );

  const options: { id: "on" | "off"; value: boolean; label: string }[] = [
    { id: "on", value: true, label: t("settings.statusLine.on") },
    { id: "off", value: false, label: t("settings.statusLine.off") },
  ];
  const effective = loaded ? enabled : activeEnabled;

  return (
    <div
      className="appearance-sheet-body"
      data-testid="general-status-line-picker"
    >
      <p className="appearance-section-label">
        {t("settings.general.statusLine")}
      </p>
      <div
        className="appearance-locale-row"
        role="group"
        aria-label={t("settings.general.statusLine")}
      >
        {options.map((opt) => (
          <button
            key={opt.id}
            type="button"
            className={[
              "appearance-locale-btn",
              effective === opt.value ? "is-active" : "",
            ]
              .filter(Boolean)
              .join(" ")}
            aria-pressed={effective === opt.value}
            data-testid={`status-line-${opt.id}`}
            onClick={() => pick(opt.value)}
          >
            {opt.label}
          </button>
        ))}
      </div>
    </div>
  );
}
