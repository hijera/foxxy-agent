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
import { MiniAppForm } from "./MiniAppForm";
import { MiniAppResults } from "./MiniAppResults";
import { rememberMiniAppRun, forgetMiniAppRun } from "./runMemory";
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

function commandProfileDetails(details: unknown): {
  binary?: string | undefined;
  argv_preview?: string[] | undefined;
} | null {
  if (!details || typeof details !== "object") return null;
  const record = details as Record<string, unknown>;
  if (record.kind !== "command_profile") return null;
  return {
    binary: typeof record.binary === "string" ? record.binary : undefined,
    argv_preview: Array.isArray(record.argv_preview)
      ? (record.argv_preview as string[])
      : undefined,
  };
}

export function MiniAppRunner(props: {
  app: MiniAppDocument;
  released: boolean;
  onClose: () => void;
  onCompleted?: (run: MiniAppRun) => void;
  initialRun?: MiniAppRun | undefined;
}) {
  const { t } = useT();
  const inputs = props.app.inputs ?? [];
  const [values, setValues] = useState<Record<string, unknown>>(() =>
    Object.fromEntries(inputs.map((input) => [input.id, initialValue(input)])),
  );
  const [run, setRun] = useState<MiniAppRun | null>(props.initialRun ?? null);
  const [events, setEvents] = useState<MiniAppRunEvent[]>([]);
  const [busy, setBusy] = useState(false);
  const [uploading, setUploading] = useState(false);
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
        setRun(result.data);
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
          ["succeeded", "failed", "cancelled", "interrupted"].includes(
            result.data.status || "",
          ) &&
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
      /* The progress stream is an optimisation; polling stays authoritative. */
    }
    return () => {
      cancelled = true;
      window.clearInterval(timer);
      unsubscribe?.();
    };
  }, [runId]);

  const status = (run?.status || "").toLowerCase();
  const waitingConfirmation = status === "waiting_for_confirmation";
  const active =
    !!runId &&
    !["succeeded", "failed", "cancelled", "interrupted"].includes(status);
  const progress = useMemo(() => {
    if (status === "succeeded") return 1;
    const candidate = Number(
      run?.progress ?? events[events.length - 1]?.progress ?? 0,
    );
    if (!Number.isFinite(candidate)) return 0;
    return Math.max(
      0,
      Math.min(1, candidate > 1 ? candidate / 100 : candidate),
    );
  }, [events, run?.progress, status]);

  const visibleEvents = events.reduce<string[]>((labels, event) => {
    const known = [
      "queued",
      "running",
      "waiting_for_input",
      "waiting_for_confirmation",
      "succeeded",
      "failed",
      "cancelled",
      "interrupted",
    ];
    if (!event.status || !known.includes(event.status)) return labels;
    const title = props.app.workflow?.find(
      (step) => step.id === event.step_id,
    )?.title;
    const label =
      (title ? `${title}: ` : "") + t(`miniapps.status.${event.status}`);
    if (labels[labels.length - 1] !== label) labels.push(label);
    return labels;
  }, []);

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
      rememberMiniAppRun(
        props.app.id,
        result.data.id || result.data.run_id || "",
      );
      setProposals(result.data.proposals ?? []);
      proposalRequestedRef.current = false;
      completedNotifiedRef.current = false;
    } else setError(result.message);
    setBusy(false);
  };
  const cancel = async () => {
    if (!runId) return;
    setBusy(true);
    const result = await cancelRun(runId);
    if (result.ok) setRun(result.data);
    else setError(result.message);
    setBusy(false);
  };
  const answer = async (approved: boolean) => {
    if (!runId) return;
    setBusy(true);
    const confirmationId = run?.confirmation?.id;
    const result = await confirmRun(runId, approved, confirmationId);
    if (result.ok) setRun(result.data);
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
            {props.app.display?.title ||
              props.app.metadata?.name ||
              props.app.id}
          </h2>
        </div>
        <button
          type="button"
          className="miniapps-quiet"
          data-testid="miniapps-run-close"
          onClick={() => {
            forgetMiniAppRun(props.app.id);
            props.onClose();
          }}
        >
          {t("miniapps.close")}
        </button>
      </div>
      {/* Outside the branch below: a start that fails never produces a run id,
          so an error rendered only in the running branch would leave the
          operator staring at an unchanged form. */}
      {error ? (
        <p className="miniapps-error" role="alert">
          {error}
        </p>
      ) : null}
      {!runId ? (
        <>
          <p className="miniapps-lead">
            {props.app.display?.description ||
              (props.released
                ? t("miniapps.runDescription")
                : t("miniapps.testDescription"))}
          </p>
          <MiniAppForm
            app={props.app}
            values={values}
            onBusy={setUploading}
            onChange={(id, value) =>
              setValues((current) => ({ ...current, [id]: value }))
            }
          />
          <div className="miniapps-action-row">
            <button
              type="button"
              className="miniapps-primary"
              data-testid="miniapps-run-start"
              disabled={
                busy ||
                uploading ||
                inputs.some(
                  (input) =>
                    input.required &&
                    (values[input.id] === undefined ||
                      values[input.id] === null ||
                      values[input.id] === "" ||
                      (Array.isArray(values[input.id]) &&
                        (values[input.id] as unknown[]).length === 0)),
                )
              }
              onClick={() => void start()}
            >
              {busy
                ? t("miniapps.starting")
                : props.app.display?.submit_label ||
                  (props.released
                    ? t("miniapps.runReleased")
                    : t("miniapps.testRun"))}
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
            <span className="miniapps-muted">{run?.message || ""}</span>
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
              {commandProfileDetails(run?.confirmation?.details) ? (
                <dl className="miniapps-commands-confirmation">
                  <dt>{t("miniapps.commands.confirmBinary")}</dt>
                  <dd>
                    <code>
                      {
                        commandProfileDetails(run?.confirmation?.details)
                          ?.binary
                      }
                    </code>
                  </dd>
                  <dt>{t("miniapps.commands.confirmArgv")}</dt>
                  <dd>
                    <code>
                      {(
                        commandProfileDetails(run?.confirmation?.details)
                          ?.argv_preview ?? []
                      ).join(" ")}
                    </code>
                  </dd>
                </dl>
              ) : null}
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
          {run?.error ? (
            <pre className="miniapps-report">{run.error}</pre>
          ) : null}
          {run ? <MiniAppResults app={props.app} run={run} /> : null}
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
          {visibleEvents.length > 0 ? (
            <ol className="miniapps-events" aria-label={t("miniapps.events")}>
              {visibleEvents.map((label, index) => (
                <li key={index}>{label}</li>
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
              data-testid="miniapps-run-again"
              disabled={active || busy}
              onClick={() => {
                setRun(null);
                forgetMiniAppRun(props.app.id);
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
