import {
  useEffect,
  useId,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import { useT } from "../i18n/I18nProvider";
import { SwitchField } from "./SwitchField";
import {
  applyNotificationConfig,
  getNotificationPreferences,
  onNotificationPreferences,
  readNotificationConfig,
} from "../desktop/notificationConfig";
import {
  previewNotificationSound,
  stopNotificationSound,
} from "../desktop/desktopNotifySound";

export function NotificationSettings(props: {
  doc: Record<string, unknown>;
  setDoc: (next: Record<string, unknown>) => void;
}) {
  const { t } = useT();
  const live = useSyncExternalStore(
    onNotificationPreferences,
    getNotificationPreferences,
  );
  const prefs = Object.keys(props.doc).length
    ? readNotificationConfig(props.doc)
    : live;
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const fileRef = useRef<HTMLInputElement>(null);
  const soundGroup = useId();
  const customFile = useRef(prefs.soundFile);
  const systemSound = t(
    window.foxxycodeHostPlaySound
      ? "desktopNotify.systemSound"
      : "desktopNotify.standardSound",
  );
  const fileName =
    prefs.soundFile
      .split(/[\\/]/)
      .at(-1)
      ?.replace(/^[a-f0-9]{64}-/, "") || t("desktopNotify.standardSound");
  useEffect(() => {
    if (Object.keys(props.doc).length) applyNotificationConfig(props.doc);
  }, [props.doc]);
  useEffect(() => {
    if (prefs.soundFile) customFile.current = prefs.soundFile;
  }, [prefs.soundFile]);
  const pick = async (
    key:
      | "notifications"
      | "notification_sound"
      | "notifications_when_focused"
      | "notification_sound_file",
    value: boolean | string,
  ) => {
    setBusy(true);
    setError("");
    try {
      // Fetch the latest document so an immediate preference write preserves unrelated settings.
      const res = await fetch("/foxxycode/config");
      if (!res.ok) throw new Error(String(res.status));
      const doc = (await res.json()) as Record<string, unknown>;
      const next = {
        ...doc,
        ui: { ...((doc.ui ?? {}) as object), [key]: value },
      };
      const saved = await fetch("/foxxycode/config", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(next),
      });
      const result = (await saved.json()) as { ok?: boolean };
      if (!saved.ok || !result.ok) throw new Error(String(saved.status));
      applyNotificationConfig(next);
      props.setDoc({
        ...props.doc,
        ui: { ...((props.doc.ui ?? {}) as object), [key]: value },
      });
    } catch {
      setError(t("settings.saveFailed", { status: "" }));
    } finally {
      setBusy(false);
    }
  };
  const upload = async (file: File) => {
    if (file.size > 5 * 1024 * 1024) {
      setError(t("desktopNotify.soundTooLarge"));
      return;
    }
    setBusy(true);
    setError("");
    try {
      const res = await fetch(
        `/foxxycode/notification-sound?filename=${encodeURIComponent(file.name)}`,
        { method: "POST", body: file },
      );
      const result = (await res.json()) as { file?: string };
      if (!res.ok || !result.file) throw new Error("upload");
      await pick("notification_sound_file", result.file);
    } catch {
      setError(t("desktopNotify.soundUploadFailed"));
    } finally {
      setBusy(false);
    }
  };
  const preview = async () => {
    setError("");
    setBusy(true);
    try {
      await previewNotificationSound(prefs.soundFile);
    } catch {
      setError(t("desktopNotify.soundPlayFailed"));
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="appearance-sheet-body" data-testid="notification-settings">
      <p className="appearance-section-label">
        {t("desktopNotify.regionLabel")}
      </p>
      <p className="settings-field-desc">
        {t("desktopNotify.settingsDescription")}
      </p>
      <div className="notification-settings-controls">
        {(
          [
            {
              key: "notifications",
              enabled: prefs.enabled,
              label: t("desktopNotify.enabled"),
            },
            {
              key: "notifications_when_focused",
              enabled: prefs.whenFocused,
              label: t("desktopNotify.whenFocused"),
            },
            {
              key: "notification_sound",
              enabled: prefs.sound,
              label: t("desktopNotify.sound"),
            },
          ] as const
        ).map((row) => (
          <SwitchField
            key={row.key}
            checked={row.enabled}
            disabled={busy}
            dataTestId={`settings-${row.key}`}
            onChange={(value) => void pick(row.key, value)}
            label={row.label}
          />
        ))}
        <div className="notification-sound-picker">
          <div
            className="notification-sound-options"
            role="radiogroup"
            aria-label={t("desktopNotify.soundSource")}
          >
            <label className="notification-sound-option">
              <input
                type="radio"
                name={soundGroup}
                checked={!prefs.soundFile}
                disabled={busy}
                onChange={() => void pick("notification_sound_file", "")}
              />
              <span>{systemSound}</span>
            </label>
            <label className="notification-sound-option">
              <input
                type="radio"
                name={soundGroup}
                checked={!!prefs.soundFile}
                disabled={busy}
                onChange={() =>
                  customFile.current
                    ? void pick("notification_sound_file", customFile.current)
                    : fileRef.current?.click()
                }
              />
              <span>{t("desktopNotify.customSound")}</span>
            </label>
          </div>
          <p className="settings-field-desc">
            {t("desktopNotify.customSoundDescription")}
          </p>
          {prefs.soundFile ? (
            <span className="notification-sound-name" title={prefs.soundFile}>
              {fileName}
            </span>
          ) : null}
          <input
            ref={fileRef}
            type="file"
            accept=".mp3,.wav,audio/mpeg,audio/wav"
            hidden
            data-testid="notification-sound-file"
            onChange={(e) => {
              const file = e.currentTarget.files?.[0];
              e.currentTarget.value = "";
              if (file) void upload(file);
            }}
          />
          <div className="notification-sound-actions">
            <button
              type="button"
              className="settings-btn"
              disabled={busy}
              onClick={() => fileRef.current?.click()}
            >
              {t("desktopNotify.chooseSound")}
            </button>
            <button
              type="button"
              className="settings-btn"
              disabled={busy}
              data-testid="notification-sound-preview"
              onClick={() => void preview()}
            >
              {t("desktopNotify.previewSound")}
            </button>
            <button
              type="button"
              className="settings-btn"
              onClick={stopNotificationSound}
            >
              {t("desktopNotify.stopSound")}
            </button>
          </div>
        </div>
      </div>
      {error ? (
        <p role="alert" className="settings-error">
          {error}
        </p>
      ) : null}
    </div>
  );
}
