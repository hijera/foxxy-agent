import { useT } from "../i18n/I18nProvider";
import type { MiniAppRun } from "./types";

export function RunHistory(props: {
  runs: MiniAppRun[];
  onSelect?: (run: MiniAppRun) => void;
}) {
  const { t } = useT();
  return (
    <section
      className="miniapps-history"
      aria-labelledby="miniapps-history-heading"
    >
      <div className="miniapps-section-head">
        <h3 id="miniapps-history-heading">{t("miniapps.runHistory")}</h3>
        <span className="miniapps-muted">{props.runs.length}</span>
      </div>
      {props.runs.length === 0 ? (
        <p className="miniapps-muted">{t("miniapps.noRuns")}</p>
      ) : (
        <div className="miniapps-history-list">
          {props.runs.map((run, index) => (
            <button
              type="button"
              className="miniapps-history-row"
              key={run.run_id || run.id || String(index)}
              onClick={() => props.onSelect?.(run)}
            >
              <span
                className={`miniapps-status-dot miniapps-status-dot--${run.status || "pending"}`}
                aria-hidden="true"
              />
              <span>
                <strong>
                  {t(`miniapps.status.${run.status || "pending"}`)}
                </strong>
                <small>
                  {run.version
                    ? `${t("miniapps.version")} ${run.version}`
                    : t("miniapps.draftRun")}
                </small>
              </span>
            </button>
          ))}
        </div>
      )}
    </section>
  );
}
