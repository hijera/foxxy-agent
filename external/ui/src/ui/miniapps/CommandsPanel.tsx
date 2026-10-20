import React, { useCallback, useEffect, useRef, useState } from "react";
import { useT } from "../i18n/I18nProvider";
import {
  getMiniAppCommands,
  getCommandInstallJob,
  installMiniAppCommand,
  trustMiniAppCommand,
} from "./api";
import type { MiniAppCommandStatus } from "./types";

/**
 * CommandsPanel lists the command profiles a Mini App depends on: whether the
 * binary is installed here, whether this machine trusts the profile, and the
 * package-manager install options detected for it. Trust and install are
 * explicit operator actions; the panel never triggers either on its own.
 */
export function CommandsPanel(props: { appId: string }) {
  const { t } = useT();
  const [rows, setRows] = useState<MiniAppCommandStatus[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState("");
  const [installTail, setInstallTail] = useState<string | null>(null);
  const cancelled = useRef(false);

  const refresh = useCallback(async () => {
    const result = await getMiniAppCommands(props.appId);
    if (cancelled.current) return;
    if (result.ok) {
      setRows(result.data);
      setError(null);
    } else setError(result.message);
  }, [props.appId]);

  useEffect(() => {
    cancelled.current = false;
    void refresh();
    return () => {
      cancelled.current = true;
    };
  }, [refresh]);

  const trust = async (row: MiniAppCommandStatus) => {
    setBusy(`trust:${row.name}`);
    setError(null);
    const result = await trustMiniAppCommand(props.appId, row.name);
    if (!cancelled.current && !result.ok) setError(result.message);
    await refresh();
    if (!cancelled.current) setBusy("");
  };

  const install = async (row: MiniAppCommandStatus, manager: string) => {
    setBusy(`install:${row.name}`);
    setError(null);
    setInstallTail(null);
    const started = await installMiniAppCommand(props.appId, row.name, manager);
    if (!started.ok) {
      if (!cancelled.current) {
        setError(started.message);
        setBusy("");
      }
      return;
    }
    const jobId = started.data.id ?? "";
    for (;;) {
      if (cancelled.current) return;
      const job = await getCommandInstallJob(jobId);
      if (job.ok) {
        const tail = (job.data.result as { output_tail?: string } | undefined)
          ?.output_tail;
        if (tail) setInstallTail(tail);
        const status = job.data.status ?? "";
        if (status === "succeeded") break;
        if (
          status === "failed" ||
          status === "cancelled" ||
          status === "interrupted"
        ) {
          setError(job.data.error || t("miniapps.commands.installFailed"));
          break;
        }
      }
      await new Promise((resolve) => setTimeout(resolve, 800));
    }
    await refresh();
    if (!cancelled.current) setBusy("");
  };

  if (rows.length === 0 && !error) return null;
  return (
    <section
      className="miniapps-editor-section miniapps-commands"
      aria-labelledby="miniapps-commands-heading"
    >
      <h3 id="miniapps-commands-heading">{t("miniapps.commands.title")}</h3>
      <p className="miniapps-muted">{t("miniapps.commands.description")}</p>
      <div className="miniapps-commands-list" role="list">
        {rows.map((row) => (
          <div
            className="miniapps-commands-row"
            role="listitem"
            key={row.name}
            data-testid={`miniapps-command-${row.name}`}
          >
            <div className="miniapps-commands-main">
              <strong>{row.name}</strong>
              <code>{row.resolved_path || row.binary}</code>
              {row.description ? (
                <span className="miniapps-muted">{row.description}</span>
              ) : null}
            </div>
            <div className="miniapps-commands-meta">
              <span
                className={`miniapps-state miniapps-state--${row.installed ? "released" : "draft"}`}
              >
                {row.installed
                  ? t("miniapps.commands.installed")
                  : t("miniapps.commands.missing")}
              </span>
              <span
                className={`miniapps-state miniapps-state--${row.trusted ? "released" : "draft"}`}
              >
                {row.trusted
                  ? t("miniapps.commands.trusted")
                  : t("miniapps.commands.untrusted")}
              </span>
            </div>
            <div className="miniapps-action-row">
              {row.installed && !row.trusted ? (
                <button
                  type="button"
                  className="miniapps-primary"
                  data-testid={`miniapps-command-trust-${row.name}`}
                  disabled={busy !== ""}
                  title={t("miniapps.commands.trustHint", {
                    binary: row.resolved_path || row.binary,
                  })}
                  onClick={() => void trust(row)}
                >
                  {busy === `trust:${row.name}`
                    ? t("miniapps.running")
                    : t("miniapps.commands.trust")}
                </button>
              ) : null}
              {!row.installed &&
                (row.managers ?? []).map((manager) => (
                  <button
                    type="button"
                    className="miniapps-secondary"
                    key={manager.id}
                    data-testid={`miniapps-command-install-${row.name}-${manager.id}`}
                    disabled={busy !== ""}
                    title={manager.command}
                    onClick={() => void install(row, manager.id)}
                  >
                    {busy === `install:${row.name}`
                      ? t("miniapps.commands.installing")
                      : t("miniapps.commands.installVia", {
                          manager: manager.id,
                        })}
                  </button>
                ))}
              {!row.installed && (row.managers ?? []).length === 0 ? (
                <span className="miniapps-muted">
                  {t("miniapps.commands.noManagers")}
                </span>
              ) : null}
            </div>
          </div>
        ))}
      </div>
      {installTail ? (
        <pre className="miniapps-report">{installTail}</pre>
      ) : null}
      {error ? (
        <p className="miniapps-error" role="alert">
          {error}
        </p>
      ) : null}
    </section>
  );
}
