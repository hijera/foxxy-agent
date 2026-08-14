import { useEffect, useState } from "react";
import { useT } from "../i18n/I18nProvider";
import {
  cancelDistillation,
  confirmScenario,
  getDistillation,
  startDistillation,
  subscribeMiniAppEvents,
} from "./api";
import { ScenarioReview } from "./ScenarioReview";
import type { MiniAppDistillation, ScenarioCandidate } from "./types";

export function DistillationWorkspace(props: {
  sessionId: string;
  onComplete: (appId?: string | undefined) => void;
  onCancel: () => void;
}) {
  const { t } = useT();
  const [job, setJob] = useState<MiniAppDistillation | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const jobId = (job?.job_id || job?.id || "").trim();

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      const result = await startDistillation(props.sessionId);
      if (cancelled) return;
      if (result.ok) setJob(result.data);
      else setError(result.message);
    })();
    return () => {
      cancelled = true;
    };
  }, [props.sessionId]);

  useEffect(() => {
    if (!jobId) return undefined;
    let cancelled = false;
    const tick = async () => {
      const result = await getDistillation(jobId);
      if (!cancelled && result.ok)
        setJob((current) => ({ ...(current ?? {}), ...result.data }));
    };
    void tick();
    const timer = window.setInterval(() => void tick(), 900);
    let unsubscribe: (() => void) | undefined;
    try {
      unsubscribe = subscribeMiniAppEvents("distillation", jobId, (event) => {
        if (!cancelled) setJob((current) => ({ ...(current ?? {}), ...event }));
      });
    } catch {
      /* Polling is the fallback for browsers without EventSource. */
    }
    return () => {
      cancelled = true;
      window.clearInterval(timer);
      unsubscribe?.();
    };
  }, [jobId]);

  const status = (job?.status || "pending").toLowerCase();
  const candidates = job?.candidates ?? job?.scenario_candidates ?? [];
  const waiting =
    status === "waiting_for_scenario" ||
    status === "needs_scenario" ||
    candidates.length > 0;
  const complete =
    status === "succeeded" || status === "completed" || !!job?.app_id;
  const completedAppId = job?.app_id;
  const rawProgress = Number(job?.progress ?? 0);
  const progress = Math.max(
    0,
    Math.min(1, rawProgress > 1 ? rawProgress / 100 : rawProgress),
  );

  const cancel = async () => {
    if (!jobId) {
      props.onCancel();
      return;
    }
    setBusy(true);
    const result = await cancelDistillation(jobId);
    if (!result.ok) setError(result.message);
    else
      setJob((current) => ({
        ...(current ?? {}),
        ...result.data,
        status: "cancelled",
      }));
    setBusy(false);
  };
  const confirm = async (scenario: ScenarioCandidate) => {
    setBusy(true);
    setError(null);
    const result = await confirmScenario(jobId, scenario);
    if (result.ok)
      setJob((current) => ({ ...(current ?? {}), ...result.data }));
    else setError(result.message);
    setBusy(false);
  };

  useEffect(() => {
    if (complete) props.onComplete(completedAppId);
  }, [complete, completedAppId, props.onComplete]);

  if (!job)
    return (
      <section className="miniapps-workspace">
        <div className="miniapps-section-head">
          <h2>{t("miniapps.distillationTitle")}</h2>
          <button
            type="button"
            className="miniapps-quiet"
            onClick={props.onCancel}
          >
            {t("miniapps.cancel")}
          </button>
        </div>
        <p className="miniapps-muted">
          {error || t("miniapps.startingDistillation")}
        </p>
      </section>
    );
  if (waiting)
    return (
      <ScenarioReview
        job={job}
        busy={busy}
        onConfirm={(scenario) => void confirm(scenario)}
        onCancel={() => void cancel()}
      />
    );
  if (complete)
    return (
      <section className="miniapps-workspace">
        <p className="miniapps-muted">{t("miniapps.loading")}</p>
      </section>
    );
  return (
    <section
      className="miniapps-workspace"
      aria-labelledby="miniapps-distillation-heading"
    >
      <div className="miniapps-section-head">
        <div>
          <p className="miniapps-eyebrow">{t("miniapps.distillation")}</p>
          <h2 id="miniapps-distillation-heading">
            {t("miniapps.distillationTitle")}
          </h2>
        </div>
        <button
          type="button"
          className="miniapps-quiet"
          disabled={busy}
          onClick={() => void cancel()}
        >
          {t("miniapps.cancel")}
        </button>
      </div>
      <p className="miniapps-lead">
        {job.message || t("miniapps.distillationDescription")}
      </p>
      <div
        className="miniapps-progress"
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(progress * 100)}
      >
        <span style={{ width: `${Math.round(progress * 100)}%` }} />
      </div>
      <p className="miniapps-muted">{job.phase || t("miniapps.preparing")}</p>
      {job.error || error ? (
        <p className="miniapps-error" role="alert">
          {job.error || error}
        </p>
      ) : null}
    </section>
  );
}
