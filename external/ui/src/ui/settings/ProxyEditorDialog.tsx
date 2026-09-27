import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { createPortal } from "react-dom";

import { useT } from "../i18n/I18nProvider";
import {
  buildProxyUrl,
  maskProxyValue,
  parseProxyUrl,
  PROXY_SCHEMES,
  validProxyPort,
  type ProxyParts,
} from "./proxyUrl";

export type ProxyEditorDialogProps = {
  /** The field's current value, split into the dialog's fields on open. */
  value: string;
  /** The URL built from the fields; "" when the user picks "No proxy". */
  onApply: (value: string) => void;
  onCancel: () => void;
};

const FOCUSABLE =
  "button:not([disabled]), input:not([disabled]), select:not([disabled])";

/**
 * The proxy URL editor behind the "…" button of a proxy field: protocol, host,
 * port, login and password as separate fields, so a password with @ : / # % or a
 * space in it needs no hand-encoding - buildProxyUrl percent-encodes it.
 *
 * The shell is ConfirmDialog's (portal, aria-modal, Escape and Tab caught on
 * window in the capture phase so the settings drawer or the onboarding dialog
 * underneath does not react), widened to hold a form.
 */
export function ProxyEditorDialog(props: ProxyEditorDialogProps) {
  const { value, onApply, onCancel } = props;
  const { t } = useT();
  const [parts, setParts] = useState<ProxyParts>(() => {
    const p = parseProxyUrl(value);
    if (!(PROXY_SCHEMES as readonly string[]).includes(p.scheme)) {
      p.scheme = "http";
    }
    return p;
  });
  const [showPassword, setShowPassword] = useState(false);
  const [triedApply, setTriedApply] = useState(false);
  const dialogRef = useRef<HTMLFormElement | null>(null);
  const hostRef = useRef<HTMLInputElement | null>(null);

  const built = useMemo(() => buildProxyUrl(parts), [parts]);
  const portOk = validProxyPort(parts.port);
  const hostOk = parts.host.trim() !== "";
  const canApply = portOk && hostOk;

  const set = (k: keyof ProxyParts) => (v: string) =>
    setParts((prev) => ({ ...prev, [k]: v }));

  useEffect(() => {
    const opener = document.activeElement as HTMLElement | null;
    const id = window.setTimeout(() => hostRef.current?.focus(), 0);
    return () => {
      window.clearTimeout(id);
      if (opener && opener.isConnected) {
        opener.focus();
      }
    };
  }, []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        e.stopPropagation();
        e.stopImmediatePropagation();
        onCancel();
        return;
      }
      if (e.key !== "Tab") {
        return;
      }
      const card = dialogRef.current;
      if (!card) {
        return;
      }
      const nodes = Array.from(card.querySelectorAll<HTMLElement>(FOCUSABLE));
      const first = nodes[0];
      const last = nodes[nodes.length - 1];
      if (!first || !last) {
        return;
      }
      const active = document.activeElement as HTMLElement | null;
      const inside = card.contains(active);
      if (e.shiftKey ? active === first || !inside : active === last || !inside) {
        e.preventDefault();
        e.stopPropagation();
        (e.shiftKey ? last : first).focus();
      }
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [onCancel]);

  const submit = (e: FormEvent) => {
    e.preventDefault();
    // A portal still bubbles React events to the component tree it came from.
    e.stopPropagation();
    setTriedApply(true);
    if (canApply) {
      onApply(built);
    }
  };

  return createPortal(
    <div
      className="confirm-dialog-backdrop proxy-editor-backdrop"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) {
          onCancel();
        }
      }}
      onClick={(e) => e.stopPropagation()}
    >
      <form
        ref={dialogRef}
        className="confirm-dialog proxy-editor"
        role="dialog"
        aria-modal="true"
        aria-label={t("settings.proxy.dialogTitle")}
        data-testid="proxy-editor"
        onSubmit={submit}
        noValidate
      >
        <h2 className="confirm-dialog-title">{t("settings.proxy.dialogTitle")}</h2>

        <label className="proxy-editor-field">
          <span>{t("settings.proxy.protocol")}</span>
          <select
            className="settings-input"
            value={parts.scheme}
            onChange={(e) => set("scheme")(e.target.value)}
            data-testid="proxy-editor-scheme"
          >
            {PROXY_SCHEMES.map((s) => (
              <option key={s} value={s}>
                {t(`settings.proxy.scheme.${s}`)}
              </option>
            ))}
          </select>
        </label>

        <div className="proxy-editor-hostport proxy-editor-hostport--port">
          <label className="proxy-editor-field">
            <span>{t("settings.proxy.host")}</span>
            <input
              ref={hostRef}
              className="settings-input"
              value={parts.host}
              onChange={(e) => set("host")(e.target.value)}
              placeholder={t("settings.proxy.hostPlaceholder")}
              autoComplete="off"
              spellCheck={false}
              aria-invalid={triedApply && !hostOk}
              data-testid="proxy-editor-host"
            />
          </label>
          <label className="proxy-editor-field">
            <span>{t("settings.proxy.port")}</span>
            <input
              className="settings-input"
              value={parts.port}
              onChange={(e) => set("port")(e.target.value)}
              placeholder={t("settings.proxy.portPlaceholder")}
              inputMode="numeric"
              autoComplete="off"
              aria-invalid={!portOk}
              data-testid="proxy-editor-port"
            />
          </label>
        </div>

        <div className="proxy-editor-hostport">
          <label className="proxy-editor-field">
            <span>{t("settings.proxy.user")}</span>
            <input
              className="settings-input"
              value={parts.user}
              onChange={(e) => set("user")(e.target.value)}
              placeholder={t("settings.proxy.optional")}
              autoComplete="off"
              spellCheck={false}
              data-testid="proxy-editor-user"
            />
          </label>
          <label className="proxy-editor-field">
            <span>{t("settings.proxy.password")}</span>
            <div className="settings-key-row">
              <input
                className="settings-input"
                type={showPassword ? "text" : "password"}
                value={parts.password}
                onChange={(e) => set("password")(e.target.value)}
                placeholder={t("settings.proxy.optional")}
                autoComplete="new-password"
                data-testid="proxy-editor-password"
              />
              <button
                type="button"
                className="settings-key-toggle"
                onClick={() => setShowPassword((v) => !v)}
              >
                {showPassword ? t("settings.hideKey") : t("settings.showKey")}
              </button>
            </div>
          </label>
        </div>

        <p className="proxy-editor-hint">{t("settings.proxy.hint")}</p>

        {!portOk ? (
          <p className="proxy-editor-error" role="alert">
            {t("settings.proxy.portInvalid")}
          </p>
        ) : triedApply && !hostOk ? (
          <p className="proxy-editor-error" role="alert">
            {t("settings.proxy.hostRequired")}
          </p>
        ) : null}

        {built ? (
          <div className="proxy-editor-result">
            <span>{t("settings.proxy.result")}</span>
            <code data-testid="proxy-editor-result">{maskProxyValue(built)}</code>
          </div>
        ) : null}

        <div className="confirm-dialog-actions">
          <button
            type="button"
            className="confirm-dialog-btn confirm-dialog-btn--cancel proxy-editor-clear"
            onClick={() => onApply("")}
          >
            {t("settings.proxy.clear")}
          </button>
          <button
            type="button"
            className="confirm-dialog-btn confirm-dialog-btn--cancel"
            onClick={onCancel}
          >
            {t("confirm.cancel")}
          </button>
          <button
            type="submit"
            className="confirm-dialog-btn confirm-dialog-btn--primary"
            disabled={!portOk}
            data-testid="proxy-editor-apply"
          >
            {t("settings.proxy.apply")}
          </button>
        </div>
      </form>
    </div>,
    document.body,
  );
}
