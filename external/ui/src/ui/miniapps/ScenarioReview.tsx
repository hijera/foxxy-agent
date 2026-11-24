import { useState } from "react";
import { useT } from "../i18n/I18nProvider";
import type { MiniAppDistillation, ScenarioCandidate } from "./types";

export function ScenarioReview(props: {
  job: MiniAppDistillation;
  onConfirm: (scenario: ScenarioCandidate) => void;
  onCancel: () => void;
  busy: boolean;
}) {
  const { t } = useT();
  const candidates =
    props.job.candidates ?? props.job.scenario_candidates ?? [];
  const [selected, setSelected] = useState(candidates[0]?.id || "");
  const current =
    candidates.find((candidate) => candidate.id === selected) ?? candidates[0];
  return (
    <section
      className="miniapps-workspace"
      aria-labelledby="miniapps-scenario-heading"
    >
      <div className="miniapps-section-head">
        <div>
          <p className="miniapps-eyebrow">{t("miniapps.distillation")}</p>
          <h2 id="miniapps-scenario-heading">{t("miniapps.scenarioTitle")}</h2>
        </div>
        <button
          type="button"
          className="miniapps-quiet"
          onClick={props.onCancel}
        >
          {t("miniapps.cancel")}
        </button>
      </div>
      <p className="miniapps-lead">{t("miniapps.scenarioDescription")}</p>
      {props.job.message ? (
        <p className="miniapps-muted">{props.job.message}</p>
      ) : null}
      <div className="miniapps-scenario-list">
        {candidates.map((candidate, index) => (
          <label
            className={`miniapps-scenario-option${candidate.id === selected ? " is-selected" : ""}`}
            key={candidate.id || String(index)}
          >
            <input
              type="radio"
              name="miniapp-scenario"
              checked={candidate.id === selected}
              onChange={() => setSelected(candidate.id)}
            />
            <span>
              <strong>
                {candidate.task || t("miniapps.untitledScenario")}
              </strong>
              <span>
                {candidate.accepted_outcome || t("miniapps.noOutcome")}
              </span>
            </span>
            {typeof candidate.confidence === "number" ? (
              <small>{Math.round(candidate.confidence * 100)}%</small>
            ) : null}
          </label>
        ))}
      </div>
      {candidates.length === 0 ? (
        <p className="miniapps-muted">{t("miniapps.noScenarios")}</p>
      ) : null}
      <div className="miniapps-action-row">
        <button
          type="button"
          className="miniapps-secondary"
          onClick={props.onCancel}
        >
          {t("miniapps.cancel")}
        </button>
        <button
          type="button"
          className="miniapps-primary"
          disabled={!current || props.busy}
          onClick={() => current && props.onConfirm(current)}
        >
          {props.busy ? t("miniapps.saving") : t("miniapps.confirmScenario")}
        </button>
      </div>
    </section>
  );
}
