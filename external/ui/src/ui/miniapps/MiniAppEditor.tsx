import { useEffect, useState } from "react";
import { useT } from "../i18n/I18nProvider";
import {
  acceptRepairPatch,
  releaseDraft,
  sanitizeDraft,
  updateDraft,
  validateDraft,
} from "./api";
import type {
  MiniAppDocument,
  MiniAppInput,
  MiniAppPatch,
  MiniAppStep,
} from "./types";

function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

function updateAt<T>(items: T[] | undefined, index: number, value: T): T[] {
  const next = [...(items ?? [])];
  next[index] = value;
  return next;
}

function InputEditor(props: {
  inputs: MiniAppInput[];
  onChange: (inputs: MiniAppInput[]) => void;
}) {
  const { t } = useT();
  return (
    <div className="miniapps-editor-list">
      {(props.inputs ?? []).map((input, index) => (
        <div className="miniapps-editor-row" key={`${input.id}-${index}`}>
          <label>
            <span>{t("miniapps.inputId")}</span>
            <input
              value={input.id}
              onChange={(e) =>
                props.onChange(
                  updateAt(props.inputs, index, {
                    ...input,
                    id: e.target.value,
                  }),
                )
              }
            />
          </label>
          <label>
            <span>{t("miniapps.inputTitle")}</span>
            <input
              value={input.title}
              onChange={(e) =>
                props.onChange(
                  updateAt(props.inputs, index, {
                    ...input,
                    title: e.target.value,
                  }),
                )
              }
            />
          </label>
          <label>
            <span>{t("miniapps.inputType")}</span>
            <select
              value={input.type}
              onChange={(e) =>
                props.onChange(
                  updateAt(props.inputs, index, {
                    ...input,
                    type: e.target.value,
                  }),
                )
              }
            >
              <option value="string">{t("miniapps.typeString")}</option>
              <option value="secret">{t("miniapps.typeSecret")}</option>
              <option value="enum">{t("miniapps.typeSelect")}</option>
              <option value="boolean">{t("miniapps.typeBoolean")}</option>
              <option value="date">{t("miniapps.typeDate")}</option>
              <option value="number">{t("miniapps.typeNumber")}</option>
            </select>
          </label>
          <label className="miniapps-checkbox">
            <input
              type="checkbox"
              checked={input.required === true}
              onChange={(e) =>
                props.onChange(
                  updateAt(props.inputs, index, {
                    ...input,
                    required: e.target.checked,
                  }),
                )
              }
            />
            <span>{t("miniapps.required")}</span>
          </label>
          <button
            type="button"
            className="miniapps-icon-action"
            title={t("miniapps.removeInput")}
            aria-label={t("miniapps.removeInput")}
            onClick={() =>
              props.onChange(
                props.inputs.filter((_, itemIndex) => itemIndex !== index),
              )
            }
          >
            ×
          </button>
        </div>
      ))}
      <button
        type="button"
        className="miniapps-secondary"
        onClick={() =>
          props.onChange([
            ...(props.inputs ?? []),
            {
              id: `input_${(props.inputs?.length ?? 0) + 1}`,
              title: t("miniapps.newInput"),
              type: "string",
              required: false,
            },
          ])
        }
      >
        {t("miniapps.addInput")}
      </button>
    </div>
  );
}

function StepEditor(props: {
  steps: MiniAppStep[];
  onChange: (steps: MiniAppStep[]) => void;
}) {
  const { t } = useT();
  return (
    <div className="miniapps-editor-list">
      {(props.steps ?? []).map((step, index) => (
        <div className="miniapps-step-row" key={`${step.id}-${index}`}>
          <div className="miniapps-step-index">{index + 1}</div>
          <div className="miniapps-step-fields">
            <label>
              <span>{t("miniapps.stepId")}</span>
              <input
                value={step.id}
                onChange={(e) =>
                  props.onChange(
                    updateAt(props.steps, index, {
                      ...step,
                      id: e.target.value,
                    }),
                  )
                }
              />
            </label>
            <label>
              <span>{t("miniapps.stepTitle")}</span>
              <input
                value={step.title}
                onChange={(e) =>
                  props.onChange(
                    updateAt(props.steps, index, {
                      ...step,
                      title: e.target.value,
                    }),
                  )
                }
              />
            </label>
            <label>
              <span>{t("miniapps.stepKind")}</span>
              <select
                value={step.kind}
                onChange={(e) =>
                  props.onChange(
                    updateAt(props.steps, index, {
                      ...step,
                      kind: e.target.value,
                    }),
                  )
                }
              >
                <option value="tool">{t("miniapps.kindTool")}</option>
                <option value="llm">{t("miniapps.kindLlm")}</option>
                <option value="agent">{t("miniapps.kindAgent")}</option>
                <option value="confirm">{t("miniapps.kindConfirm")}</option>
                <option value="branch">{t("miniapps.kindBranch")}</option>
                <option value="miniapp">{t("miniapps.kindMiniApp")}</option>
              </select>
            </label>
            {step.kind === "tool" ? (
              <label>
                <span>{t("miniapps.toolName")}</span>
                <input
                  value={step.tool ?? ""}
                  onChange={(e) =>
                    props.onChange(
                      updateAt(props.steps, index, {
                        ...step,
                        tool: e.target.value,
                      }),
                    )
                  }
                />
              </label>
            ) : null}
            {step.kind === "llm" || step.kind === "agent" ? (
              <label>
                <span>{t("miniapps.modelBinding")}</span>
                <input
                  value={step.model_binding ?? ""}
                  onChange={(e) =>
                    props.onChange(
                      updateAt(props.steps, index, {
                        ...step,
                        model_binding: e.target.value,
                      }),
                    )
                  }
                />
              </label>
            ) : null}
          </div>
          <button
            type="button"
            className="miniapps-icon-action"
            title={t("miniapps.removeStep")}
            aria-label={t("miniapps.removeStep")}
            onClick={() =>
              props.onChange(
                props.steps.filter((_, itemIndex) => itemIndex !== index),
              )
            }
          >
            ×
          </button>
        </div>
      ))}
      <button
        type="button"
        className="miniapps-secondary"
        onClick={() =>
          props.onChange([
            ...(props.steps ?? []),
            {
              id: `step_${(props.steps?.length ?? 0) + 1}`,
              kind: "tool",
              title: t("miniapps.newStep"),
            },
          ])
        }
      >
        {t("miniapps.addStep")}
      </button>
    </div>
  );
}

export function MiniAppEditor(props: {
  initial: MiniAppDocument;
  sourceEvidence?: Record<string, unknown> | null;
  releasedVersion?: string | null;
  verifiedRevision?: string | null;
  validatedRevision?: string | null;
  sanitizedRevision?: string | null;
  onValidated?: (revision: string) => void;
  onSanitized?: (revision: string) => void;
  onChecksReset?: () => void;
  onRun: (app: MiniAppDocument, release: boolean) => void;
  onRunReleased?: () => void;
  onSaved?: (app: MiniAppDocument) => void;
}) {
  const { t } = useT();
  const [draft, setDraft] = useState<MiniAppDocument>(() =>
    clone(props.initial),
  );
  const [tab, setTab] = useState<
    "design" | "permissions" | "evidence" | "json"
  >("design");
  const [busy, setBusy] = useState("");
  const [notice, setNotice] = useState<string | null>(null);
  const [validation, setValidation] = useState<Record<string, unknown> | null>(
    null,
  );
  const [sanitization, setSanitization] = useState<Record<
    string,
    unknown
  > | null>(null);
  const [patches, setPatches] = useState<MiniAppPatch[]>([]);
  const [releaseVersion, setReleaseVersion] = useState(
    draft.version || "1.0.0",
  );
  const [releaseApproved, setReleaseApproved] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [savedRevision, setSavedRevision] = useState(
    props.initial.revision || "",
  );
  const [validatedRevision, setValidatedRevision] = useState<string | null>(
    null,
  );
  const [sanitizedRevision, setSanitizedRevision] = useState<string | null>(
    null,
  );
  const [rawJsonText, setRawJsonText] = useState(() =>
    JSON.stringify(props.initial, null, 2),
  );
  const [rawJsonError, setRawJsonError] = useState<string | null>(null);

  useEffect(() => {
    setDraft(clone(props.initial));
    setRawJsonText(JSON.stringify(props.initial, null, 2));
    setRawJsonError(null);
    setDirty(false);
    setSavedRevision(props.initial.revision || "");
    setValidatedRevision(null);
    setSanitizedRevision(null);
    props.onChecksReset?.();
  }, [props.initial]);
  useEffect(() => {
    const candidate =
      props.sourceEvidence?.repair_proposals ??
      (props.initial as Record<string, unknown>).repair_proposals;
    if (Array.isArray(candidate)) setPatches(candidate as MiniAppPatch[]);
  }, [props.initial, props.sourceEvidence]);

  const update = (patch: Partial<MiniAppDocument>) => {
    setDirty(true);
    setValidatedRevision(null);
    props.onChecksReset?.();
    setDraft((current) => {
      const next = { ...current, ...patch };
      setRawJsonText(JSON.stringify(next, null, 2));
      setRawJsonError(null);
      return next;
    });
  };
  const save = async () => {
    if (rawJsonError) {
      setNotice(rawJsonError);
      return;
    }
    setBusy("save");
    setNotice(null);
    const result = await updateDraft(draft.id, draft, draft.revision);
    if (!result.ok) {
      setNotice(result.message);
      setBusy("");
      return;
    }
    const saved = result.data;
    setDraft(saved);
    setRawJsonText(JSON.stringify(saved, null, 2));
    setRawJsonError(null);
    setDirty(false);
    setSavedRevision(saved.revision || "");
    setValidatedRevision(null);
    props.onSaved?.(saved);
    setNotice(t("miniapps.saved"));
    setBusy("");
  };
  const validate = async () => {
    setBusy("validate");
    setNotice(null);
    const result = await validateDraft(draft.id);
    if (result.ok) {
      setValidation(result.data);
      setValidatedRevision(draft.revision || savedRevision);
      props.onValidated?.(draft.revision || savedRevision);
      setNotice(t("miniapps.validationComplete"));
    } else setNotice(result.message);
    setBusy("");
  };
  const sanitize = async () => {
    setBusy("sanitize");
    setNotice(null);
    const result = await sanitizeDraft(draft.id);
    if (result.ok) {
      setSanitization(result.data);
      const report = result.data as { blocking?: boolean; clean?: boolean };
      if (report.blocking !== true && report.clean !== false) {
        setSanitizedRevision(draft.revision || savedRevision);
        props.onSanitized?.(draft.revision || savedRevision);
      }
      setNotice(t("miniapps.sanitizationComplete"));
    } else setNotice(result.message);
    setBusy("");
  };
  const release = async () => {
    setBusy("release");
    setNotice(null);
    const result = await releaseDraft(
      draft.id,
      releaseVersion.trim(),
      releaseApproved,
      savedRevision || draft.revision,
    );
    if (result.ok) {
      setDraft(result.data);
      setRawJsonText(JSON.stringify(result.data, null, 2));
      setRawJsonError(null);
      setDirty(false);
      props.onSaved?.(result.data);
      setNotice(t("miniapps.releaseComplete"));
    } else setNotice(result.message);
    setBusy("");
  };
  const acceptPatch = async (patch: MiniAppPatch, index: number) => {
    const patchId = (patch.id || "").trim();
    if (!patchId) return;
    setBusy("patch");
    setNotice(null);
    const result = await acceptRepairPatch(draft.id, patchId);
    if (result.ok) {
      setDraft(result.data);
      setRawJsonText(JSON.stringify(result.data, null, 2));
      setRawJsonError(null);
      props.onSaved?.(result.data);
      setPatches((current) => current.filter((_, i) => i !== index));
      setNotice(t("miniapps.patchAccepted"));
    } else setNotice(result.message);
    setBusy("");
  };
  const workflow = draft.workflow ?? [];
  const inputs = draft.inputs ?? [];
  const toolPermissions = draft.permissions?.tools ?? [];
  const modelPermissions = draft.permissions?.models ?? [];
  const currentRevision = draft.revision || savedRevision;
  const effectiveSanitizedRevision =
    props.sanitizedRevision ?? sanitizedRevision;
  const checksCurrent =
    !dirty &&
    (props.validatedRevision ?? validatedRevision) === currentRevision &&
    props.verifiedRevision === currentRevision &&
    effectiveSanitizedRevision === currentRevision;
  const updateRawJson = (value: string) => {
    setRawJsonText(value);
    setDirty(true);
    setValidatedRevision(null);
    props.onChecksReset?.();
    try {
      const parsed = JSON.parse(value) as unknown;
      if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
        setRawJsonError(t("miniapps.invalidJson"));
        return;
      }
      setDraft(parsed as MiniAppDocument);
      setRawJsonError(null);
    } catch {
      setRawJsonError(t("miniapps.invalidJson"));
    }
  };

  return (
    <section
      className="miniapps-editor"
      aria-labelledby="miniapps-editor-heading"
    >
      <div className="miniapps-editor-head">
        <div>
          <p className="miniapps-eyebrow">{t("miniapps.editorEyebrow")}</p>
          <h2 id="miniapps-editor-heading">
            {draft.metadata?.name || draft.id}
          </h2>
          <p className="miniapps-muted">
            {t("miniapps.revisionLabel", {
              revision: draft.revision || t("miniapps.unsaved"),
            })}
          </p>
        </div>
        <div className="miniapps-action-row">
          <button
            type="button"
            className="miniapps-secondary"
            disabled={dirty}
            onClick={() => props.onRun(draft, false)}
          >
            {t("miniapps.testRun")}
          </button>
          {props.releasedVersion && props.onRunReleased ? (
            <button
              type="button"
              className="miniapps-primary"
              onClick={props.onRunReleased}
            >
              {t("miniapps.runReleased")}
            </button>
          ) : draft.state === "released" && draft.version ? (
            <button
              type="button"
              className="miniapps-primary"
              onClick={() => props.onRun(draft, true)}
            >
              {t("miniapps.runReleased")}
            </button>
          ) : null}
        </div>
      </div>
      <nav className="miniapps-tabs" aria-label={t("miniapps.editorSections")}>
        {(["design", "permissions", "evidence", "json"] as const).map((id) => (
          <button
            type="button"
            key={id}
            className={tab === id ? "is-active" : ""}
            onClick={() => setTab(id)}
            aria-current={tab === id ? "page" : undefined}
          >
            {t(`miniapps.tab.${id}`)}
          </button>
        ))}
      </nav>
      {tab === "design" ? (
        <div className="miniapps-editor-body">
          <section className="miniapps-editor-section">
            <h3>{t("miniapps.metadata")}</h3>
            <div className="miniapps-form-grid">
              <label>
                <span>{t("miniapps.name")}</span>
                <input
                  value={draft.metadata?.name ?? ""}
                  onChange={(e) =>
                    update({
                      metadata: { ...draft.metadata, name: e.target.value },
                    })
                  }
                />
              </label>
              <label>
                <span>{t("miniapps.goal")}</span>
                <input
                  value={draft.metadata?.goal ?? ""}
                  onChange={(e) =>
                    update({
                      metadata: { ...draft.metadata, goal: e.target.value },
                    })
                  }
                />
              </label>
              <label className="miniapps-form-wide">
                <span>{t("miniapps.description")}</span>
                <textarea
                  rows={3}
                  value={draft.metadata?.description ?? ""}
                  onChange={(e) =>
                    update({
                      metadata: {
                        ...draft.metadata,
                        description: e.target.value,
                      },
                    })
                  }
                />
              </label>
            </div>
          </section>
          <section className="miniapps-editor-section">
            <h3>{t("miniapps.inputs")}</h3>
            <InputEditor
              inputs={inputs}
              onChange={(next) => update({ inputs: next })}
            />
          </section>
          <section className="miniapps-editor-section">
            <h3>{t("miniapps.steps")}</h3>
            <StepEditor
              steps={workflow}
              onChange={(next) => update({ workflow: next })}
            />
          </section>
          <section className="miniapps-editor-section">
            <h3>{t("miniapps.successCriteria")}</h3>
            <textarea
              className="miniapps-codearea"
              rows={5}
              value={
                draft.success?.expectations ??
                draft.success?.expected_result ??
                ""
              }
              onChange={(e) =>
                update({
                  success: { ...draft.success, expectations: e.target.value },
                })
              }
            />
          </section>
        </div>
      ) : null}
      {tab === "permissions" ? (
        <div className="miniapps-editor-body">
          <section className="miniapps-editor-section">
            <h3>{t("miniapps.permissions")}</h3>
            <label>
              <span>{t("miniapps.tools")}</span>
              <textarea
                className="miniapps-codearea"
                rows={4}
                value={toolPermissions.join("\n")}
                onChange={(e) =>
                  update({
                    permissions: {
                      ...draft.permissions,
                      tools: e.target.value
                        .split("\n")
                        .map((x) => x.trim())
                        .filter(Boolean),
                    },
                  })
                }
              />
            </label>
            <label>
              <span>{t("miniapps.models")}</span>
              <textarea
                className="miniapps-codearea"
                rows={3}
                value={modelPermissions.join("\n")}
                onChange={(e) =>
                  update({
                    permissions: {
                      ...draft.permissions,
                      models: e.target.value
                        .split("\n")
                        .map((x) => x.trim())
                        .filter(Boolean),
                    },
                  })
                }
              />
            </label>
            <p className="miniapps-muted">{t("miniapps.permissionsNote")}</p>
          </section>
          <section className="miniapps-editor-section">
            <h3>{t("miniapps.verification")}</h3>
            <div className="miniapps-action-row">
              <button
                type="button"
                className="miniapps-secondary"
                disabled={busy !== "" || dirty}
                onClick={() => void validate()}
              >
                {busy === "validate"
                  ? t("miniapps.running")
                  : t("miniapps.validate")}
              </button>
              <button
                type="button"
                className="miniapps-secondary"
                disabled={busy !== "" || dirty}
                onClick={() => void sanitize()}
              >
                {busy === "sanitize"
                  ? t("miniapps.running")
                  : t("miniapps.sanitize")}
              </button>
            </div>
            {validation ? (
              <pre
                className="miniapps-report"
                data-testid="miniapps-validation-report"
              >
                {JSON.stringify(validation, null, 2)}
              </pre>
            ) : null}
            {sanitization ? (
              <pre
                className="miniapps-report"
                data-testid="miniapps-sanitization-report"
              >
                {JSON.stringify(sanitization, null, 2)}
              </pre>
            ) : null}
            {patches.length > 0 ? (
              <div className="miniapps-patch-list">
                {patches.map((patch, index) => (
                  <div
                    className="miniapps-patch"
                    key={patch.id || String(index)}
                  >
                    <strong>
                      {patch.summary || t("miniapps.repairProposal")}
                    </strong>
                    <p>{patch.reason}</p>
                    <div className="miniapps-action-row">
                      <button
                        type="button"
                        className="miniapps-secondary"
                        disabled={busy !== ""}
                        onClick={() => void acceptPatch(patch, index)}
                      >
                        {busy === "patch"
                          ? t("miniapps.saving")
                          : t("miniapps.acceptPatch")}
                      </button>
                      <button
                        type="button"
                        className="miniapps-secondary"
                        onClick={() =>
                          setPatches((current) =>
                            current.filter((_, i) => i !== index),
                          )
                        }
                      >
                        {t("miniapps.dismiss")}
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            ) : null}
          </section>
        </div>
      ) : null}
      {tab === "evidence" ? (
        <div className="miniapps-editor-body">
          <section className="miniapps-editor-section">
            <h3>{t("miniapps.sourceEvidence")}</h3>
            <p className="miniapps-muted">{t("miniapps.evidencePrivate")}</p>
            <pre className="miniapps-report">
              {JSON.stringify(props.sourceEvidence ?? {}, null, 2)}
            </pre>
          </section>
        </div>
      ) : null}
      {tab === "json" ? (
        <div className="miniapps-editor-body">
          <section className="miniapps-editor-section">
            <h3>{t("miniapps.rawJson")}</h3>
            <textarea
              className="miniapps-codearea miniapps-raw-json"
              rows={24}
              value={rawJsonText}
              onChange={(e) => updateRawJson(e.target.value)}
              aria-label={t("miniapps.rawJson")}
            />
            {rawJsonError ? (
              <p className="miniapps-error" role="alert">
                {rawJsonError}
              </p>
            ) : null}
          </section>
        </div>
      ) : null}
      <div className="miniapps-editor-footer">
        {notice ? (
          <p className="miniapps-status" role="status">
            {notice}
          </p>
        ) : (
          <span />
        )}
        <div className="miniapps-action-row">
          <button
            type="button"
            className="miniapps-secondary"
            disabled={busy !== "" || !!rawJsonError}
            onClick={() => void save()}
          >
            {busy === "save" ? t("miniapps.saving") : t("miniapps.saveDraft")}
          </button>
        </div>
      </div>
      <section
        className="miniapps-release-review"
        aria-labelledby="miniapps-release-heading"
      >
        <div>
          <h3 id="miniapps-release-heading">{t("miniapps.releaseReview")}</h3>
          <p>{t("miniapps.releaseDescription")}</p>
          {!checksCurrent ? (
            <p className="miniapps-muted">
              {t("miniapps.releaseNeedsCurrentChecks")}
            </p>
          ) : null}
        </div>
        <div className="miniapps-release-controls">
          <label>
            <span>{t("miniapps.version")}</span>
            <input
              value={releaseVersion}
              onChange={(e) => setReleaseVersion(e.target.value)}
            />
          </label>
          <label className="miniapps-checkbox">
            <input
              type="checkbox"
              checked={releaseApproved}
              onChange={(e) => setReleaseApproved(e.target.checked)}
            />
            <span>{t("miniapps.releaseApproval")}</span>
          </label>
          <button
            type="button"
            className="miniapps-primary"
            disabled={!releaseApproved || !checksCurrent || busy !== ""}
            onClick={() => void release()}
          >
            {busy === "release"
              ? t("miniapps.releasing")
              : t("miniapps.release")}
          </button>
        </div>
      </section>
    </section>
  );
}
