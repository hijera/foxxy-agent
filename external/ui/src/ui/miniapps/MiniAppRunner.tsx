import { useEffect, useMemo, useRef, useState } from "react";
import { useT } from "../i18n/I18nProvider";
import {
  acceptRepairPatch,
  cancelRun,
  confirmRun,
  createRepairProposals,
  createReleaseRun,
  createTestRun,
  getRun,
  subscribeMiniAppEvents,
} from "./api";
import type {
  MiniAppDocument,
  MiniAppInput,
  MiniAppPatch,
  MiniAppRun,
  MiniAppRunEvent,
} from "./types";

function initialValue(input: MiniAppInput): unknown {
  if (input.default !== undefined) return input.default;
  if (input.type === "boolean") return false;
  return "";
}

function formatValue(input: MiniAppInput, value: unknown): string {
  if (input.type === "boolean") return value === true ? "true" : "false";
  if (value === null || value === undefined) return "";
  return String(value);
}

export function MiniAppRunner(props: {
  app: MiniAppDocument;
  released: boolean;
  onClose: () => void;
  onCompleted?: (run: MiniAppRun) => void;
}) {
  const { t } = useT();
  const inputs = props.app.inputs ?? [];
  const [values, setValues] = useState<Record<string, unknown>>(() =>
    Object.fromEntries(inputs.map((input) => [input.id, initialValue(input)])),
  );
  const [run, setRun] = useState<MiniAppRun | null>(null);
  const [events, setEvents] = useState<MiniAppRunEvent[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [proposals, setProposals] = useState<MiniAppPatch[]>([]);
  const proposalRequestedRef = useRef(false);
  const completedNotifiedRef = useRef(false);

  // Test/release POSTs return an AsyncJob. Keep polling that job id while it
  // runs; its run_id is only a pointer to the persisted run after completion.
  const runId = (run?.id || run?.run_id || "").trim();
  useEffect(() => {
    if (!runId) return undefined;
    let cancelled = false;
    const tick = async () => {
      const result = await getRun(runId);
      if (!cancelled && result.ok) {
        setRun((current) => ({ ...(current ?? {}), ...result.data }));
        if (Array.isArray(result.data.proposals))
          setProposals(result.data.proposals);
        if (result.data.report && !proposalRequestedRef.current) {
          proposalRequestedRef.current = true;
          const proposalResult = await createRepairProposals(
            props.app.id,
            result.data.report,
          );
          if (!cancelled && proposalResult.ok) {
            setProposals(
              proposalResult.data.patches ?? proposalResult.data.items ?? [],
            );
          }
        }
        if (
          result.data.status === "succeeded" &&
          !completedNotifiedRef.current
        ) {
          completedNotifiedRef.current = true;
          props.onCompleted?.(result.data);
        }
      }
    };
    void tick();
    const timer = window.setInterval(() => void tick(), 1000);
    let unsubscribe: (() => void) | undefined;
    try {
      unsubscribe = subscribeMiniAppEvents("run", runId, (event) => {
        if (!cancelled)
          setEvents((current) => {
            const key =
              event.seq !== undefined
                ? `seq:${event.seq}`
                : event.id
                  ? `id:${event.id}`
                  : null;
            if (key) {
              const duplicate = current.some((item) => {
                const itemKey =
                  item.seq !== undefined
                    ? `seq:${item.seq}`
                    : item.id
                      ? `id:${item.id}`
                      : null;
                return itemKey === key;
              });
              if (duplicate) return current;
            }
            return [...current, event];
          });
      });
    } catch {
      /* EventSource is optional; polling remains authoritative. */
    }
    return () => {
      cancelled = true;
      window.clearInterval(timer);
      unsubscribe?.();
    };
  }, [runId]);

  const status = (run?.status || "").toLowerCase();
  const waitingConfirmation =
    status === "waiting_for_confirmation" || !!run?.confirmation;
  const active =
    !!runId &&
    !["succeeded", "failed", "cancelled", "interrupted"].includes(status);
  const progress = useMemo(() => {
    const candidate = Number(
      run?.progress ?? events[events.length - 1]?.progress ?? 0,
    );
    if (!Number.isFinite(candidate)) return 0;
    return Math.max(
      0,
      Math.min(1, candidate > 1 ? candidate / 100 : candidate),
    );
  }, [events, run?.progress]);

  const updateValue = (input: MiniAppInput, raw: string | boolean) => {
    let value: unknown = raw;
    if (input.type === "number") value = raw === "" ? "" : Number(raw);
    if (input.type === "boolean") value = raw === true || raw === "true";
    setValues((current) => ({ ...current, [input.id]: value }));
  };

  const start = async () => {
    setBusy(true);
    setError(null);
    setEvents([]);
    const result =
      props.released && props.app.version
        ? await createReleaseRun(props.app.id, props.app.version, values)
        : await createTestRun(props.app.id, values);
    if (result.ok) {
      setRun(result.data);
      setProposals(result.data.proposals ?? []);
      proposalRequestedRef.current = false;
      completedNotifiedRef.current = result.data.status === "succeeded";
    } else setError(result.message);
    setBusy(false);
  };
  const cancel = async () => {
    if (!runId) return;
    setBusy(true);
    const result = await cancelRun(runId);
    if (result.ok)
      setRun((current) => ({ ...(current ?? {}), ...result.data }));
    else setError(result.message);
    setBusy(false);
  };
  const answer = async (approved: boolean) => {
    if (!runId) return;
    setBusy(true);
    const confirmationId = run?.confirmation?.id;
    const result = await confirmRun(runId, approved, confirmationId);
    if (result.ok)
      setRun((current) => ({ ...(current ?? {}), ...result.data }));
    else setError(result.message);
    setBusy(false);
  };

  return (
    <section
      className="miniapps-runner"
      aria-labelledby="miniapps-runner-heading"
    >
      <div className="miniapps-section-head">
        <div>
          <p className="miniapps-eyebrow">
            {props.released
              ? t("miniapps.releasedRun")
              : t("miniapps.verificationRun")}
          </p>
          <h2 id="miniapps-runner-heading">
            {props.app.metadata?.name || props.app.id}
          </h2>
        </div>
        <button
          type="button"
          className="miniapps-quiet"
          onClick={props.onClose}
        >
          {t("miniapps.close")}
        </button>
      </div>
      {!runId ? (
        <>
          <p className="miniapps-lead">
            {props.released
              ? t("miniapps.runDescription")
              : t("miniapps.testDescription")}
          </p>
          <div className="miniapps-run-form">
            {inputs.length === 0 ? (
              <p className="miniapps-muted">{t("miniapps.noInputs")}</p>
            ) : null}
            {inputs.map((input) => {
              const value = values[input.id];
              const control =
                input.type === "secret"
                  ? "password"
                  : input.type === "date"
                    ? "date"
                    : input.type === "number"
                      ? "number"
                      : "text";
              const label = input.title || input.id;
              return (
                <label key={input.id}>
                  <span>
                    {label}
                    {input.required ? <em aria-hidden="true"> *</em> : null}
                  </span>
                  {input.type === "boolean" ? (
                    <span className="miniapps-checkbox">
                      <input
                        aria-label={label}
                        type="checkbox"
                        checked={value === true}
                        onChange={(e) => updateValue(input, e.target.checked)}
                      />
                      <span>{t("miniapps.enabled")}</span>
                    </span>
                  ) : (input.type === "enum" || input.type === "select") &&
                    input.validation?.enum ? (
                    <select
                      aria-label={label}
                      value={formatValue(input, value)}
                      onChange={(e) => updateValue(input, e.target.value)}
                    >
                      <option value="">{t("miniapps.chooseValue")}</option>
                      {input.validation.enum.map((option) => (
                        <option key={String(option)} value={String(option)}>
                          {String(option)}
                        </option>
                      ))}
                    </select>
                  ) : (
                    <input
                      aria-label={label}
                      type={control}
                      value={formatValue(input, value)}
                      placeholder={input.ui?.placeholder}
                      onChange={(e) => updateValue(input, e.target.value)}
                    />
                  )}
                </label>
              );
            })}
          </div>
          <div className="miniapps-action-row">
            <button
              type="button"
              className="miniapps-primary"
              disabled={busy}
              onClick={() => void start()}
            >
              {busy
                ? t("miniapps.starting")
                : props.released
                  ? t("miniapps.runReleased")
                  : t("miniapps.testRun")}
            </button>
          </div>
        </>
      ) : (
        <>
          <div className="miniapps-run-status">
            <span
              className={`miniapps-status-dot miniapps-status-dot--${status || "pending"}`}
              aria-hidden="true"
            />
            <strong>
              {status
                ? t(`miniapps.status.${status}`)
                : t("miniapps.status.pending")}
            </strong>
            <span className="miniapps-muted">
              {run?.message || run?.phase || ""}
            </span>
          </div>
          <div
            className="miniapps-progress"
            role="progressbar"
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={Math.round(progress * 100)}
          >
            <span style={{ width: `${Math.round(progress * 100)}%` }} />
          </div>
          {waitingConfirmation ? (
            <div className="miniapps-confirmation" role="alert">
              <h3>{t("miniapps.confirmationTitle")}</h3>
              <p>
                {run?.confirmation?.message ||
                  t("miniapps.confirmationDescription")}
              </p>
              <div className="miniapps-action-row">
                <button
                  type="button"
                  className="miniapps-secondary"
                  disabled={busy}
                  onClick={() => void answer(false)}
                >
                  {t("miniapps.reject")}
                </button>
                <button
                  type="button"
                  className="miniapps-primary"
                  disabled={busy}
                  onClick={() => void answer(true)}
                >
                  {t("miniapps.approve")}
                </button>
              </div>
            </div>
          ) : null}
          {error ? (
            <p className="miniapps-error" role="alert">
              {error}
            </p>
          ) : null}
          {run?.error ? (
            <pre className="miniapps-report">{run.error}</pre>
          ) : null}
          {run?.result !== undefined || run?.outputs !== undefined ? (
            <div className="miniapps-result">
              <h3>{t("miniapps.result")}</h3>
              <pre className="miniapps-report">
                {JSON.stringify(run.result ?? run.outputs, null, 2)}
              </pre>
            </div>
          ) : null}
          {proposals.length > 0 ? (
            <section
              className="miniapps-patch-list"
              aria-label={t("miniapps.repairProposal")}
            >
              <h3>{t("miniapps.repairProposal")}</h3>
              {proposals.map((proposal, index) => (
                <div
                  className="miniapps-patch"
                  key={proposal.id || String(index)}
                >
                  <strong>
                    {proposal.summary || t("miniapps.repairProposal")}
                  </strong>
                  <p>{proposal.reason}</p>
                  <button
                    type="button"
                    className="miniapps-secondary"
                    disabled={busy}
                    onClick={() => {
                      const proposalId = (proposal.id || "").trim();
                      if (!proposalId) return;
                      setBusy(true);
                      void acceptRepairPatch(props.app.id, proposalId).then(
                        (result) => {
                          if (result.ok)
                            setProposals((current) =>
                              current.filter(
                                (_, itemIndex) => itemIndex !== index,
                              ),
                            );
                          else setError(result.message);
                          setBusy(false);
                        },
                      );
                    }}
                  >
                    {t("miniapps.acceptPatch")}
                  </button>
                </div>
              ))}
            </section>
          ) : null}
          {events.length > 0 ? (
            <ol className="miniapps-events" aria-label={t("miniapps.events")}>
              {events.map((event, index) => (
                <li key={event.id || String(index)}>
                  <span>
                    {event.message || event.type || t("miniapps.stepUpdate")}
                  </span>
                  {event.step_id ? <code>{event.step_id}</code> : null}
                </li>
              ))}
            </ol>
          ) : null}
          <div className="miniapps-action-row">
            {active ? (
              <button
                type="button"
                className="miniapps-secondary"
                disabled={busy}
                onClick={() => void cancel()}
              >
                {t("miniapps.cancelRun")}
              </button>
            ) : null}
            <button
              type="button"
              className="miniapps-secondary"
              onClick={() => {
                setRun(null);
                setEvents([]);
              }}
            >
              {t("miniapps.rerun")}
            </button>
          </div>
        </>
      )}
    </section>
  );
}
