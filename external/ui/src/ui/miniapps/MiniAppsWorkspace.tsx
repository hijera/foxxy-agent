import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useT } from "../i18n/I18nProvider";
import {
  createMiniApp,
  distillSession,
  editMiniAppWithAssistant,
  generateExpectedResult,
  getDistillation,
  getMiniAppDraft,
  getMiniAppSource,
  listMiniApps,
  putMiniAppDraft,
  releaseMiniApp,
  runMiniApp,
  setMiniAppModelBinding,
  testMiniApp,
} from "./api";
import type {
  MiniAppAuthoringTurn,
  MiniAppCatalogEntry,
  MiniAppCondition,
  MiniAppDocument,
  MiniAppInput,
  MiniAppRun,
  MiniAppStep,
  SourceEvidence,
} from "./types";

type EditorMode = "form" | "json" | "run";

function normalizeMiniAppDocument(app: MiniAppDocument): MiniAppDocument {
  const wire = app as Partial<MiniAppDocument>;
  return {
    ...app,
    inputs: Array.isArray(wire.inputs) ? wire.inputs : [],
    workflow: Array.isArray(wire.workflow) ? wire.workflow : [],
    outputs: Array.isArray(wire.outputs) ? wire.outputs : [],
  };
}

function blankMiniApp(): MiniAppDocument {
  const suffix = Date.now().toString(36);
  return {
    schema_version: "1.0.0",
    kind: "foxxycode.miniapp",
    id: `miniapp-${suffix}`,
    state: "draft",
    metadata: {
      name: "New mini app",
      description: "Reusable operator workflow.",
      goal: "Produce the reviewed result.",
      tags: [],
    },
    requirements: {},
    permissions: {},
    inputs: [],
    workflow: [
      {
        id: "produce-result",
        kind: "program",
        title: "Produce result",
        language: "foxxy-vm/1",
        entry: "main",
        functions: {
          main: [{ op: "const", arg: "Edit this workflow." }, { op: "return" }],
        },
        limits: { instructions: 100, stack_depth: 16, call_depth: 4 },
      },
    ],
    success: {
      mode: "all",
      checks: [{ kind: "step", step: "produce-result", status: "succeeded" }],
    },
    outputs: [
      {
        id: "result",
        type: "text",
        value: { $ref: "steps.produce-result.outputs.result" },
        renderer: "text",
      },
    ],
    display: { layout: "form-result" },
    runtime: {
      log_scope: "global",
      operator_event_level: "status",
      diagnostic_tool_events: "sanitized",
      persist_agent_reasoning: false,
    },
  };
}

function nextPortableID(prefix: string, existing: string[]) {
  const used = new Set(existing);
  let index = existing.length + 1;
  while (used.has(`${prefix}-${index}`)) {
    index += 1;
  }
  return `${prefix}-${index}`;
}

function newMiniAppInput(
  existing: MiniAppInput[],
  title: string,
): MiniAppInput {
  const id = nextPortableID(
    "input",
    existing.map((input) => input.id),
  );
  return {
    id,
    type: "string",
    title,
    ui: { control: "text", order: existing.length + 1 },
  };
}

function newMiniAppStep(
  existing: MiniAppStep[],
  kind = "program",
  title = "Step",
): MiniAppStep {
  const id = nextPortableID(
    "step",
    existing.map((step) => step.id),
  );
  const base = { id, kind, title };
  switch (kind) {
    case "program":
      return {
        ...base,
        language: "foxxy-vm/1",
        entry: "main",
        functions: {
          main: [{ op: "const", arg: "Edit this step." }, { op: "return" }],
        },
        limits: { instructions: 100, stack_depth: 16, call_depth: 4 },
      };
    case "script":
      return { ...base, script_language: "python", script: "print('result')" };
    case "command":
      return { ...base, command: "echo", args: ["result"] };
    case "agent":
      return {
        ...base,
        prompt: "Produce the declared result.",
        model_binding: "primary",
      };
    case "api":
      return {
        ...base,
        request: { method: "GET", url: "https://api.example.com" },
      };
    case "file":
      return { ...base, operation: "read", path: { $ref: "inputs.path" } };
    case "confirm":
      return { ...base, message: "Allow this operation?" };
    case "branch":
      return { ...base, then: [], else: [] };
    case "miniapp":
      return { ...base, app_id: "other-app", app_version: "1.0.0" };
    default:
      return base;
  }
}

function inputValue(
  input: MiniAppInput,
  values: Record<string, unknown>,
): unknown {
  if (Object.prototype.hasOwnProperty.call(values, input.id)) {
    return values[input.id];
  }
  return input.default ?? (input.type === "boolean" ? false : "");
}

function parseInputValue(input: MiniAppInput, value: string | boolean) {
  if (input.type === "boolean") {
    return Boolean(value);
  }
  if (input.type === "number" || input.type === "integer") {
    return value === "" ? null : Number(value);
  }
  return value;
}

function conditionValue(
  raw: unknown,
  values: Record<string, unknown>,
): unknown {
  if (
    typeof raw === "object" &&
    raw !== null &&
    "$ref" in raw &&
    typeof (raw as { $ref?: unknown }).$ref === "string"
  ) {
    const ref = String((raw as { $ref: string }).$ref);
    return ref.startsWith("inputs.")
      ? values[ref.slice("inputs.".length)]
      : undefined;
  }
  return raw;
}

function evaluateInputCondition(
  condition: MiniAppCondition | undefined,
  values: Record<string, unknown>,
  fallback: boolean,
): boolean {
  if (!condition) {
    return fallback;
  }
  const args = condition.args || [];
  if (condition.op === "and") {
    return args.every((item) => evaluateInputCondition(item, values, false));
  }
  if (condition.op === "or") {
    return args.some((item) => evaluateInputCondition(item, values, false));
  }
  if (condition.op === "not") {
    return !evaluateInputCondition(args[0], values, false);
  }
  const left = conditionValue(
    condition.value === undefined ? condition.left : condition.value,
    values,
  );
  const right = conditionValue(condition.right, values);
  switch (condition.op) {
    case "eq":
      return JSON.stringify(left) === JSON.stringify(right);
    case "ne":
      return JSON.stringify(left) !== JSON.stringify(right);
    case "exists":
      return left !== undefined && left !== null;
    case "empty":
      return left === undefined || left === null || left === "";
    case "contains":
      return Array.isArray(left)
        ? left.some((item) => JSON.stringify(item) === JSON.stringify(right))
        : String(left ?? "").includes(String(right ?? ""));
    case "gt":
      return Number(left) > Number(right);
    case "gte":
      return Number(left) >= Number(right);
    case "lt":
      return Number(left) < Number(right);
    case "lte":
      return Number(left) <= Number(right);
    default:
      return false;
  }
}

export function MiniAppsWorkspace(props: {
  open: boolean;
  currentSessionId: string;
  distillRequestEpoch?: number;
  availableModels?: string[];
  onClose: () => void;
}) {
  const { t } = useT();
  const [items, setItems] = useState<MiniAppCatalogEntry[]>([]);
  const [selectedId, setSelectedId] = useState("");
  const [draft, setDraft] = useState<MiniAppDocument | null>(null);
  const [raw, setRaw] = useState("");
  const [mode, setMode] = useState<EditorMode>("form");
  const [query, setQuery] = useState("");
  const [busy, setBusy] = useState(false);
  const [generatingExpectation, setGeneratingExpectation] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [runInputs, setRunInputs] = useState<Record<string, unknown>>({});
  const [confirmations, setConfirmations] = useState<Record<string, boolean>>(
    {},
  );
  const [runResult, setRunResult] = useState<MiniAppRun | null>(null);
  const [releaseVersion, setReleaseVersion] = useState("1.0.0");
  const [source, setSource] = useState<SourceEvidence | null>(null);
  const [selectedStepId, setSelectedStepId] = useState("");
  const [stepJSON, setStepJSON] = useState("");
  const [authoringMessage, setAuthoringMessage] = useState("");
  const [authoringHistory, setAuthoringHistory] = useState<
    MiniAppAuthoringTurn[]
  >([]);
  const [authoringOperations, setAuthoringOperations] = useState<string[]>([]);
  const [authoringBusy, setAuthoringBusy] = useState(false);
  const lastDistillRequestRef = useRef(0);
  const selectedEntry = useMemo(
    () => items.find((item) => item.id === selectedId),
    [items, selectedId],
  );
  const selectedStepIndex = useMemo(
    () => draft?.workflow.findIndex((step) => step.id === selectedStepId) ?? -1,
    [draft?.workflow, selectedStepId],
  );
  const selectedStep =
    draft && selectedStepIndex >= 0
      ? draft.workflow[selectedStepIndex]
      : undefined;
  const selectedLogicalModel =
    draft?.requirements?.model_bindings?.find(
      (binding) => binding.id === "primary",
    )?.logical_model ||
    draft?.requirements?.model_bindings?.[0]?.logical_model ||
    "";
  const modelOptions = useMemo(() => {
    const choices = new Set(props.availableModels || []);
    if (selectedLogicalModel) {
      choices.add(selectedLogicalModel);
    }
    return [...choices];
  }, [props.availableModels, selectedLogicalModel]);

  const refresh = useCallback(
    async (search = query) => {
      const result = await listMiniApps(search);
      if (!result.ok) {
        setError(result.message);
        return;
      }
      setItems(result.data.items || []);
    },
    [query],
  );

  useEffect(() => {
    if (props.open) {
      void refresh();
    }
  }, [props.open, refresh]);

  const openDraft = useCallback(async (id: string) => {
    setBusy(true);
    setError("");
    const [result, sourceResult] = await Promise.all([
      getMiniAppDraft(id),
      getMiniAppSource(id),
    ]);
    setBusy(false);
    if (!result.ok) {
      setError(result.message);
      return;
    }
    const normalized = normalizeMiniAppDocument(result.data);
    setSelectedId(id);
    setDraft(normalized);
    setSource(sourceResult.ok ? sourceResult.data : null);
    setRaw(JSON.stringify(normalized, null, 2));
    setRunInputs({});
    setConfirmations({});
    setRunResult(null);
    setMode("form");
    const firstStep = normalized.workflow[0];
    setSelectedStepId(firstStep?.id || "");
    setStepJSON(firstStep ? JSON.stringify(firstStep, null, 2) : "");
    setAuthoringHistory([]);
    setAuthoringOperations([]);
  }, []);

  const save = useCallback(async () => {
    if (!draft) {
      return;
    }
    setBusy(true);
    setError("");
    let next = draft;
    if (mode === "json") {
      try {
        next = JSON.parse(raw) as MiniAppDocument;
      } catch (parseError) {
        setBusy(false);
        setError(
          t("miniapps.invalidJson", {
            message:
              parseError instanceof Error
                ? parseError.message
                : String(parseError),
          }),
        );
        return;
      }
    }
    const result = await putMiniAppDraft(next);
    setBusy(false);
    if (!result.ok) {
      setError(result.message);
      return;
    }
    const normalized = normalizeMiniAppDocument(result.data);
    setDraft(normalized);
    setRaw(JSON.stringify(normalized, null, 2));
    setNotice(t("miniapps.saved"));
    void refresh();
  }, [draft, mode, raw, refresh, t]);

  const distill = useCallback(async () => {
    const sessionId = props.currentSessionId.trim();
    if (!sessionId) {
      setError(t("miniapps.distillNeedsSession"));
      return;
    }
    setBusy(true);
    setError("");
    setNotice(t("miniapps.distilling"));
    const started = await distillSession(sessionId);
    if (!started.ok) {
      setBusy(false);
      setError(started.message);
      return;
    }
    let job = started.data;
    for (let attempt = 0; attempt < 240; attempt += 1) {
      if (job.status === "completed" && job.app_id) {
        setBusy(false);
        setNotice(t("miniapps.distilled"));
        await refresh();
        await openDraft(job.app_id);
        return;
      }
      if (job.status === "failed" || job.status === "cancelled") {
        setBusy(false);
        setError(job.error || t("miniapps.distillFailed"));
        return;
      }
      await new Promise((resolve) => window.setTimeout(resolve, 500));
      const update = await getDistillation(job.id);
      if (!update.ok) {
        setBusy(false);
        setError(update.message);
        return;
      }
      job = update.data;
      setNotice(
        t("miniapps.distillProgress", {
          phase: job.phase,
          progress: job.progress,
        }),
      );
    }
    setBusy(false);
    setError(t("miniapps.distillTimeout"));
  }, [openDraft, props.currentSessionId, refresh, t]);

  useEffect(() => {
    const requestEpoch = props.distillRequestEpoch || 0;
    if (
      !props.open ||
      requestEpoch <= 0 ||
      requestEpoch === lastDistillRequestRef.current
    ) {
      return;
    }
    lastDistillRequestRef.current = requestEpoch;
    void distill();
  }, [distill, props.distillRequestEpoch, props.open]);

  const create = useCallback(async () => {
    const app = blankMiniApp();
    setBusy(true);
    const result = await createMiniApp(app);
    setBusy(false);
    if (!result.ok) {
      setError(result.message);
      return;
    }
    await refresh();
    await openDraft(app.id);
  }, [openDraft, refresh]);

  const importJSON = useCallback(
    async (file: File | undefined) => {
      if (!file) {
        return;
      }
      setBusy(true);
      setError("");
      try {
        const app = JSON.parse(await file.text()) as MiniAppDocument;
        const result = await createMiniApp(app);
        if (!result.ok) {
          setError(result.message);
          return;
        }
        setNotice(t("miniapps.imported"));
        await refresh();
        await openDraft(result.data.id);
      } catch (importError) {
        setError(
          t("miniapps.invalidJson", {
            message:
              importError instanceof Error
                ? importError.message
                : String(importError),
          }),
        );
      } finally {
        setBusy(false);
      }
    },
    [openDraft, refresh, t],
  );

  const run = useCallback(
    async (test: boolean) => {
      if (!draft) {
        return;
      }
      setBusy(true);
      setError("");
      setRunResult(null);
      const result = test
        ? await testMiniApp(draft.id, runInputs, confirmations)
        : await runMiniApp(
            draft.id,
            selectedEntry?.version || releaseVersion,
            runInputs,
            confirmations,
          );
      setBusy(false);
      if (!result.ok) {
        setError(result.message);
        return;
      }
      setRunResult(result.data);
      setNotice(
        result.data.status === "succeeded"
          ? t("miniapps.runSucceeded")
          : t("miniapps.runFailed"),
      );
    },
    [
      confirmations,
      draft,
      releaseVersion,
      runInputs,
      selectedEntry?.version,
      t,
    ],
  );

  const release = useCallback(async () => {
    if (!draft) {
      return;
    }
    setBusy(true);
    const result = await releaseMiniApp(draft.id, releaseVersion);
    setBusy(false);
    if (!result.ok) {
      setError(result.message);
      return;
    }
    setNotice(t("miniapps.released", { version: releaseVersion }));
    await refresh();
  }, [draft, refresh, releaseVersion, t]);

  const updateMetadata = (
    key: "name" | "description" | "goal" | "author",
    value: string,
  ) => {
    setDraft((current) =>
      current
        ? {
            ...current,
            metadata: { ...current.metadata, [key]: value },
          }
        : current,
    );
  };

  const updateInput = (index: number, patch: Partial<MiniAppInput>) => {
    setDraft((current) => {
      if (!current) {
        return current;
      }
      const inputs = current.inputs.map((input, i) =>
        i === index ? { ...input, ...patch } : input,
      );
      return { ...current, inputs };
    });
  };

  const addInput = () => {
    setDraft((current) =>
      current
        ? {
            ...current,
            inputs: [
              ...current.inputs,
              newMiniAppInput(current.inputs, t("miniapps.newInputTitle")),
            ],
          }
        : current,
    );
  };

  const removeInputAt = (index: number) => {
    setDraft((current) =>
      current
        ? {
            ...current,
            inputs: current.inputs.filter(
              (_, currentIndex) => currentIndex !== index,
            ),
          }
        : current,
    );
  };

  const selectStep = (step: MiniAppStep) => {
    setSelectedStepId(step.id);
    setStepJSON(JSON.stringify(step, null, 2));
  };

  const addStep = () => {
    if (!draft) {
      return;
    }
    const step = newMiniAppStep(
      draft.workflow,
      "program",
      t("miniapps.newStepTitle"),
    );
    setDraft({ ...draft, workflow: [...draft.workflow, step] });
    selectStep(step);
  };

  const removeStepAt = (index: number) => {
    if (!draft || draft.workflow.length <= 1) {
      return;
    }
    const workflow = draft.workflow.filter(
      (_, currentIndex) => currentIndex !== index,
    );
    const fallback = workflow[Math.min(index, workflow.length - 1)];
    setDraft({ ...draft, workflow });
    if (draft.workflow[index]?.id === selectedStepId && fallback) {
      selectStep(fallback);
    }
  };

  const updateSelectedStep = (patch: Partial<MiniAppStep>) => {
    if (!draft || selectedStepIndex < 0) {
      return;
    }
    const current = draft.workflow[selectedStepIndex];
    const next = { ...current, ...patch };
    const workflow = draft.workflow.map((step, index) =>
      index === selectedStepIndex ? next : step,
    );
    setDraft({ ...draft, workflow });
    if (patch.id && patch.id !== selectedStepId) {
      setSelectedStepId(patch.id);
    }
    setStepJSON(JSON.stringify(next, null, 2));
  };

  const changeSelectedStepKind = (kind: string) => {
    if (!draft || !selectedStep) {
      return;
    }
    const template = newMiniAppStep(
      draft.workflow.filter((step) => step.id !== selectedStep.id),
      kind,
    );
    updateSelectedStep({
      ...template,
      id: selectedStep.id,
      title: selectedStep.title,
    });
  };

  const applyStepJSON = () => {
    if (!draft || selectedStepIndex < 0) {
      return;
    }
    try {
      const next = JSON.parse(stepJSON) as MiniAppStep;
      if (!next.id || !next.kind || !next.title) {
        throw new Error(t("miniapps.stepRequiredFields"));
      }
      const workflow = draft.workflow.map((step, index) =>
        index === selectedStepIndex ? next : step,
      );
      setDraft({ ...draft, workflow });
      setSelectedStepId(next.id);
      setStepJSON(JSON.stringify(next, null, 2));
      setError("");
    } catch (parseError) {
      setError(
        t("miniapps.invalidJson", {
          message:
            parseError instanceof Error
              ? parseError.message
              : String(parseError),
        }),
      );
    }
  };

  const updateSuccess = (
    key: "expectations" | "expected_result" | "acceptance_criterion",
    value: string,
  ) => {
    setDraft((current) => {
      if (!current) {
        return current;
      }
      const checks =
        key === "acceptance_criterion"
          ? current.success.checks.map((check) =>
              check.kind === "prompt" ? { ...check, prompt: value } : check,
            )
          : current.success.checks;
      return {
        ...current,
        success: { ...current.success, [key]: value, checks },
      };
    });
  };

  const generateExpectation = useCallback(async () => {
    if (!draft) {
      return;
    }
    const expectations = (draft.success.expectations || "").trim();
    if (!expectations) {
      setError(t("miniapps.expectationsRequired"));
      return;
    }
    setBusy(true);
    setGeneratingExpectation(true);
    setError("");
    setNotice(t("miniapps.generatingExpectedResult"));
    const result = await generateExpectedResult(draft, expectations);
    setBusy(false);
    setGeneratingExpectation(false);
    if (!result.ok) {
      setNotice("");
      setError(result.message);
      return;
    }
    const normalized = normalizeMiniAppDocument(result.data.app);
    setDraft(normalized);
    setRaw(JSON.stringify(normalized, null, 2));
    setNotice(t("miniapps.expectedResultGenerated"));
    void refresh();
  }, [draft, refresh, t]);

  const selectLogicalModel = useCallback(
    async (modelRef: string) => {
      if (!draft || !modelRef) {
        return;
      }
      setBusy(true);
      setError("");
      const result = await setMiniAppModelBinding(draft, modelRef);
      setBusy(false);
      if (!result.ok) {
        setError(result.message);
        return;
      }
      const normalized = normalizeMiniAppDocument(result.data);
      setDraft(normalized);
      setRaw(JSON.stringify(normalized, null, 2));
      const step = normalized.workflow.find(
        (candidate) => candidate.id === selectedStepId,
      );
      if (step) {
        setStepJSON(JSON.stringify(step, null, 2));
      }
      setNotice(t("miniapps.modelSaved", { model: modelRef }));
    },
    [draft, selectedStepId, t],
  );

  const sendAuthoringMessage = useCallback(async () => {
    const message = authoringMessage.trim();
    if (!draft || !message) {
      return;
    }
    setAuthoringBusy(true);
    setError("");
    const previousHistory = authoringHistory;
    setAuthoringHistory((current) => [
      ...current,
      { role: "user", content: message },
    ]);
    setAuthoringMessage("");
    const result = await editMiniAppWithAssistant(
      draft,
      message,
      previousHistory,
    );
    setAuthoringBusy(false);
    if (!result.ok) {
      setError(result.message);
      return;
    }
    const normalized = normalizeMiniAppDocument(result.data.app);
    setDraft(normalized);
    setRaw(JSON.stringify(normalized, null, 2));
    setAuthoringHistory((current) => [
      ...current,
      { role: "assistant", content: result.data.message },
    ]);
    setAuthoringOperations(result.data.operations || []);
    const step =
      normalized.workflow.find(
        (candidate) => candidate.id === selectedStepId,
      ) || normalized.workflow[0];
    if (step) {
      setSelectedStepId(step.id);
      setStepJSON(JSON.stringify(step, null, 2));
    }
    setNotice(t("miniapps.assistantUpdated"));
    void refresh();
  }, [authoringHistory, authoringMessage, draft, refresh, selectedStepId, t]);

  const filteredItems = useMemo(() => items, [items]);

  if (!props.open) {
    return null;
  }

  return (
    <aside
      className="miniapps-workspace drawer"
      aria-label={t("miniapps.title")}
      data-testid="miniapps-workspace"
    >
      <header className="miniapps-head">
        <div>
          <strong>{t("miniapps.title")}</strong>
          <span>{t("miniapps.subtitle")}</span>
        </div>
        <button
          type="button"
          className="sessions-close"
          aria-label={t("miniapps.close")}
          onClick={props.onClose}
        >
          ×
        </button>
      </header>

      <div className="miniapps-toolbar">
        <button type="button" className="scheduler-btn" onClick={create}>
          {t("miniapps.new")}
        </button>
        <label className="scheduler-btn miniapps-import">
          {t("miniapps.import")}
          <input
            type="file"
            accept="application/json,.json"
            disabled={busy}
            onChange={(event) => {
              void importJSON(event.target.files?.[0]);
              event.target.value = "";
            }}
          />
        </label>
        <button
          type="button"
          className="scheduler-btn scheduler-btn-primary"
          onClick={() => void distill()}
          disabled={busy || !props.currentSessionId.trim()}
        >
          {t("miniapps.distillSession")}
        </button>
        <input
          type="search"
          value={query}
          placeholder={t("miniapps.search")}
          onChange={(event) => {
            const value = event.target.value;
            setQuery(value);
            void refresh(value);
          }}
        />
      </div>

      {error ? <div className="miniapps-banner is-error">{error}</div> : null}
      {notice ? <div className="miniapps-banner">{notice}</div> : null}

      <div className="miniapps-grid">
        <nav className="miniapps-catalog" aria-label={t("miniapps.catalog")}>
          {filteredItems.length === 0 ? (
            <p className="sessions-empty">{t("miniapps.empty")}</p>
          ) : null}
          {filteredItems.map((item) => (
            <button
              type="button"
              key={item.id}
              className={`miniapps-card${selectedId === item.id ? " is-active" : ""}`}
              onClick={() => void openDraft(item.id)}
            >
              <strong>{item.name}</strong>
              <span>{item.description}</span>
              <small>
                {item.state}
                {item.version ? ` · v${item.version}` : ""}
              </small>
            </button>
          ))}
        </nav>

        <section className="miniapps-editor">
          {!draft ? (
            <div className="miniapps-placeholder">
              <h2>{t("miniapps.selectTitle")}</h2>
              <p>{t("miniapps.selectDescription")}</p>
            </div>
          ) : (
            <>
              <div className="miniapps-editor-tabs" role="tablist">
                {(["form", "json", "run"] as EditorMode[]).map((tab) => (
                  <button
                    key={tab}
                    type="button"
                    role="tab"
                    aria-selected={mode === tab}
                    className={mode === tab ? "is-active" : ""}
                    onClick={() => {
                      if (tab === "json" && draft) {
                        setRaw(JSON.stringify(draft, null, 2));
                      }
                      setMode(tab);
                    }}
                  >
                    {t(`miniapps.tab.${tab}`)}
                  </button>
                ))}
              </div>
              <div className="miniapps-model-bar">
                <label>
                  {t("miniapps.logicalModel")}
                  <select
                    aria-label={t("miniapps.logicalModel")}
                    value={selectedLogicalModel}
                    disabled={busy || modelOptions.length === 0}
                    onChange={(event) =>
                      void selectLogicalModel(event.target.value)
                    }
                  >
                    {modelOptions.length === 0 ? (
                      <option value="">{t("miniapps.noModels")}</option>
                    ) : selectedLogicalModel ? null : (
                      <option value="">{t("miniapps.chooseModel")}</option>
                    )}
                    {modelOptions.map((model) => (
                      <option key={model} value={model}>
                        {model}
                      </option>
                    ))}
                  </select>
                </label>
                <span>{t("miniapps.logicalModelDescription")}</span>
              </div>

              {mode === "form" ? (
                <div className="miniapps-authoring">
                  <aside className="miniapps-step-nav">
                    <div className="miniapps-section-head">
                      <h3>
                        {t("miniapps.steps", { count: draft.workflow.length })}
                      </h3>
                      <button
                        type="button"
                        className="miniapps-icon-action"
                        onClick={addStep}
                      >
                        {t("miniapps.addStep")}
                      </button>
                    </div>
                    {draft.workflow.map((step, index) => (
                      <div
                        className={`miniapps-step-row${
                          selectedStepId === step.id ? " is-active" : ""
                        }`}
                        key={step.id}
                      >
                        <button
                          type="button"
                          className="miniapps-step-select"
                          onClick={() => selectStep(step)}
                        >
                          <span>{index + 1}</span>
                          <b>{step.kind}</b>
                          <small>{step.title}</small>
                        </button>
                        <button
                          type="button"
                          className="miniapps-remove-action"
                          aria-label={t("miniapps.removeStep", {
                            title: step.title,
                          })}
                          disabled={draft.workflow.length <= 1}
                          onClick={() => removeStepAt(index)}
                        >
                          ×
                        </button>
                      </div>
                    ))}
                    {source ? (
                      <section className="miniapps-source">
                        <h3>{t("miniapps.source")}</h3>
                        {source.sanitized_user ? (
                          <p>{source.sanitized_user}</p>
                        ) : null}
                        {source.accepted_result ? (
                          <pre>{source.accepted_result}</pre>
                        ) : null}
                      </section>
                    ) : null}
                  </aside>
                  <div className="miniapps-form">
                    <div className="miniapps-metadata-row">
                      <label>
                        {t("miniapps.name")}
                        <input
                          value={draft.metadata.name}
                          onChange={(event) =>
                            updateMetadata("name", event.target.value)
                          }
                        />
                      </label>
                      <label>
                        {t("miniapps.author")}
                        <input
                          value={draft.metadata.author || ""}
                          onChange={(event) =>
                            updateMetadata("author", event.target.value)
                          }
                        />
                      </label>
                    </div>
                    <label>
                      {t("miniapps.description")}
                      <input
                        value={draft.metadata.description}
                        onChange={(event) =>
                          updateMetadata("description", event.target.value)
                        }
                      />
                    </label>
                    <label>
                      {t("miniapps.goal")}
                      <textarea
                        value={draft.metadata.goal}
                        onChange={(event) =>
                          updateMetadata("goal", event.target.value)
                        }
                      />
                    </label>
                    {selectedStep ? (
                      <section className="miniapps-step-editor">
                        <div className="miniapps-section-head">
                          <h3>{t("miniapps.editStep")}</h3>
                          <span>{selectedStep.id}</span>
                        </div>
                        <div className="miniapps-step-fields">
                          <label>
                            {t("miniapps.stepId")}
                            <input
                              value={selectedStep.id}
                              onChange={(event) =>
                                updateSelectedStep({ id: event.target.value })
                              }
                            />
                          </label>
                          <label>
                            {t("miniapps.stepTitle")}
                            <input
                              value={selectedStep.title}
                              onChange={(event) =>
                                updateSelectedStep({
                                  title: event.target.value,
                                })
                              }
                            />
                          </label>
                          <label>
                            {t("miniapps.stepKind")}
                            <select
                              value={selectedStep.kind}
                              onChange={(event) =>
                                changeSelectedStepKind(event.target.value)
                              }
                            >
                              {[
                                "input",
                                "program",
                                "script",
                                "command",
                                "agent",
                                "api",
                                "mcp",
                                "skill",
                                "file",
                                "confirm",
                                "branch",
                                "miniapp",
                              ].map((kind) => (
                                <option key={kind} value={kind}>
                                  {kind}
                                </option>
                              ))}
                            </select>
                          </label>
                        </div>
                        <label>
                          {t("miniapps.stepJson")}
                          <textarea
                            className="miniapps-step-json"
                            value={stepJSON}
                            spellCheck={false}
                            onChange={(event) =>
                              setStepJSON(event.target.value)
                            }
                          />
                        </label>
                        <button
                          type="button"
                          className="scheduler-btn"
                          onClick={applyStepJSON}
                        >
                          {t("miniapps.applyStepJson")}
                        </button>
                      </section>
                    ) : null}
                    <section className="miniapps-input-editor">
                      <div className="miniapps-section-head">
                        <h3>
                          {t("miniapps.inputs", { count: draft.inputs.length })}
                        </h3>
                        <button
                          type="button"
                          className="miniapps-icon-action"
                          onClick={addInput}
                        >
                          {t("miniapps.addInput")}
                        </button>
                      </div>
                      {draft.inputs.map((input, index) => (
                        <div
                          className="miniapps-input-row"
                          key={`${input.id}-${index}`}
                        >
                          <input
                            aria-label={t("miniapps.inputId")}
                            value={input.id}
                            onChange={(event) =>
                              updateInput(index, { id: event.target.value })
                            }
                          />
                          <input
                            aria-label={t("miniapps.inputTitle")}
                            value={input.title}
                            onChange={(event) =>
                              updateInput(index, { title: event.target.value })
                            }
                          />
                          <select
                            aria-label={t("miniapps.inputType")}
                            value={input.type}
                            onChange={(event) =>
                              updateInput(index, { type: event.target.value })
                            }
                          >
                            {[
                              "string",
                              "text",
                              "integer",
                              "number",
                              "boolean",
                              "date",
                              "datetime",
                              "enum",
                              "file",
                              "files",
                              "directory",
                              "secret",
                            ].map((type) => (
                              <option key={type} value={type}>
                                {type}
                              </option>
                            ))}
                          </select>
                          <label className="miniapps-required">
                            <input
                              type="checkbox"
                              checked={!!input.required}
                              onChange={(event) =>
                                updateInput(index, {
                                  required: event.target.checked,
                                })
                              }
                            />
                            {t("miniapps.required")}
                          </label>
                          <button
                            type="button"
                            className="miniapps-remove-action"
                            aria-label={t("miniapps.removeInput", {
                              title: input.title,
                            })}
                            onClick={() => removeInputAt(index)}
                          >
                            ×
                          </button>
                        </div>
                      ))}
                    </section>
                    <section className="miniapps-acceptance">
                      <div className="miniapps-acceptance-head">
                        <div>
                          <strong>{t("miniapps.acceptance")}</strong>
                          <span>{t("miniapps.acceptanceDescription")}</span>
                        </div>
                        <button
                          type="button"
                          className="scheduler-btn scheduler-btn-primary"
                          disabled={
                            busy || !(draft.success.expectations || "").trim()
                          }
                          onClick={() => void generateExpectation()}
                        >
                          {generatingExpectation
                            ? t("miniapps.generatingExpectedResult")
                            : t("miniapps.generateExpectedResult")}
                        </button>
                      </div>
                      <label>
                        {t("miniapps.authorExpectations")}
                        <textarea
                          value={draft.success.expectations || ""}
                          placeholder={t(
                            "miniapps.authorExpectationsPlaceholder",
                          )}
                          onChange={(event) =>
                            updateSuccess("expectations", event.target.value)
                          }
                        />
                      </label>
                      <label>
                        {t("miniapps.expectedResult")}
                        <textarea
                          value={draft.success.expected_result || ""}
                          onChange={(event) =>
                            updateSuccess("expected_result", event.target.value)
                          }
                        />
                      </label>
                      <label>
                        {t("miniapps.acceptanceCriterion")}
                        <textarea
                          value={draft.success.acceptance_criterion || ""}
                          onChange={(event) =>
                            updateSuccess(
                              "acceptance_criterion",
                              event.target.value,
                            )
                          }
                        />
                      </label>
                    </section>
                  </div>
                  <aside
                    className="miniapps-assistant"
                    aria-label={t("miniapps.assistant")}
                  >
                    <header>
                      <strong>{t("miniapps.assistant")}</strong>
                      <span>{t("miniapps.assistantDescription")}</span>
                    </header>
                    <div
                      className="miniapps-assistant-history"
                      aria-live="polite"
                    >
                      {authoringHistory.length === 0 ? (
                        <p>{t("miniapps.assistantEmpty")}</p>
                      ) : null}
                      {authoringHistory.map((turn, index) => (
                        <div
                          className={`miniapps-assistant-turn is-${turn.role}`}
                          key={`${turn.role}-${index}`}
                        >
                          <small>
                            {turn.role === "user"
                              ? t("miniapps.operator")
                              : t("miniapps.assistant")}
                          </small>
                          <p>{turn.content}</p>
                        </div>
                      ))}
                      {authoringOperations.length > 0 ? (
                        <details>
                          <summary>
                            {t("miniapps.operations", {
                              count: authoringOperations.length,
                            })}
                          </summary>
                          <ul>
                            {authoringOperations.map((operation) => (
                              <li key={operation}>{operation}</li>
                            ))}
                          </ul>
                        </details>
                      ) : null}
                    </div>
                    <form
                      onSubmit={(event) => {
                        event.preventDefault();
                        void sendAuthoringMessage();
                      }}
                    >
                      <label>
                        {t("miniapps.assistantMessage")}
                        <textarea
                          aria-label={t("miniapps.assistantMessage")}
                          value={authoringMessage}
                          placeholder={t("miniapps.assistantPlaceholder")}
                          onChange={(event) =>
                            setAuthoringMessage(event.target.value)
                          }
                        />
                      </label>
                      <button
                        type="submit"
                        className="scheduler-btn scheduler-btn-primary"
                        disabled={authoringBusy || !authoringMessage.trim()}
                      >
                        {authoringBusy
                          ? t("miniapps.assistantWorking")
                          : t("miniapps.send")}
                      </button>
                    </form>
                  </aside>
                </div>
              ) : null}

              {mode === "json" ? (
                <textarea
                  className="miniapps-json-editor"
                  value={raw}
                  spellCheck={false}
                  aria-label={t("miniapps.jsonAria")}
                  onChange={(event) => setRaw(event.target.value)}
                />
              ) : null}

              {mode === "run" ? (
                <div className="miniapps-runner">
                  <form
                    onSubmit={(event) => {
                      event.preventDefault();
                      void run(!selectedEntry?.version);
                    }}
                  >
                    {draft.inputs
                      .filter((input) =>
                        evaluateInputCondition(
                          input.visible_when,
                          runInputs,
                          true,
                        ),
                      )
                      .map((input) => (
                        <label key={input.id}>
                          {input.title}
                          {input.type === "boolean" ? (
                            <input
                              type="checkbox"
                              checked={Boolean(inputValue(input, runInputs))}
                              onChange={(event) =>
                                setRunInputs((current) => ({
                                  ...current,
                                  [input.id]: event.target.checked,
                                }))
                              }
                            />
                          ) : input.validation?.enum ? (
                            <select
                              value={String(inputValue(input, runInputs))}
                              onChange={(event) =>
                                setRunInputs((current) => ({
                                  ...current,
                                  [input.id]: parseInputValue(
                                    input,
                                    event.target.value,
                                  ),
                                }))
                              }
                            >
                              {input.validation.enum.map((value) => (
                                <option
                                  key={String(value)}
                                  value={String(value)}
                                >
                                  {String(value)}
                                </option>
                              ))}
                            </select>
                          ) : input.ui.control === "textarea" ||
                            input.type === "text" ? (
                            <textarea
                              required={
                                !!input.required ||
                                evaluateInputCondition(
                                  input.required_when,
                                  runInputs,
                                  false,
                                )
                              }
                              disabled={
                                !evaluateInputCondition(
                                  input.enabled_when,
                                  runInputs,
                                  true,
                                )
                              }
                              value={String(inputValue(input, runInputs))}
                              onChange={(event) =>
                                setRunInputs((current) => ({
                                  ...current,
                                  [input.id]: event.target.value,
                                }))
                              }
                            />
                          ) : (
                            <input
                              required={
                                !!input.required ||
                                evaluateInputCondition(
                                  input.required_when,
                                  runInputs,
                                  false,
                                )
                              }
                              disabled={
                                !evaluateInputCondition(
                                  input.enabled_when,
                                  runInputs,
                                  true,
                                )
                              }
                              type={
                                input.type === "secret"
                                  ? "password"
                                  : input.type === "date"
                                    ? "date"
                                    : input.type === "datetime"
                                      ? "datetime-local"
                                      : input.type === "file" ||
                                          input.type === "files" ||
                                          input.type === "directory"
                                        ? "text"
                                        : input.type === "number" ||
                                            input.type === "integer"
                                          ? "number"
                                          : "text"
                              }
                              value={String(inputValue(input, runInputs))}
                              onChange={(event) =>
                                setRunInputs((current) => ({
                                  ...current,
                                  [input.id]: parseInputValue(
                                    input,
                                    event.target.value,
                                  ),
                                }))
                              }
                            />
                          )}
                          {input.description ? (
                            <small>{input.description}</small>
                          ) : null}
                        </label>
                      ))}
                    {draft.workflow
                      .filter((step) => step.kind === "confirm")
                      .map((step) => (
                        <label className="miniapps-confirmation" key={step.id}>
                          <input
                            type="checkbox"
                            checked={!!confirmations[step.id]}
                            onChange={(event) =>
                              setConfirmations((current) => ({
                                ...current,
                                [step.id]: event.target.checked,
                              }))
                            }
                          />
                          <span>
                            {typeof step.message === "string"
                              ? step.message
                              : step.title}
                            <small>{t("miniapps.confirmation")}</small>
                          </span>
                        </label>
                      ))}
                    <button
                      type="submit"
                      className="scheduler-btn scheduler-btn-primary"
                      disabled={busy}
                    >
                      {selectedEntry?.version
                        ? t("miniapps.run")
                        : t("miniapps.testRun")}
                    </button>
                  </form>
                  <div className="miniapps-result">
                    <h3>{t("miniapps.result")}</h3>
                    <pre>
                      {runResult
                        ? JSON.stringify(
                            runResult.outputs ?? {
                              status: runResult.status,
                              error: runResult.error,
                            },
                            null,
                            2,
                          )
                        : t("miniapps.noResult")}
                    </pre>
                  </div>
                </div>
              ) : null}

              <footer className="miniapps-editor-footer">
                <span>
                  {draft.state}
                  {draft.revision ? ` · ${draft.revision.slice(0, 8)}` : ""}
                </span>
                <div>
                  <button
                    type="button"
                    className="scheduler-btn"
                    disabled={busy || mode === "run"}
                    onClick={() => void save()}
                  >
                    {t("miniapps.saveDraft")}
                  </button>
                  <a
                    className="scheduler-btn miniapps-export"
                    href={`/foxxycode/miniapps/${encodeURIComponent(draft.id)}/export`}
                    download={`${draft.id}-miniapp.json`}
                  >
                    {t("miniapps.export")}
                  </a>
                  <button
                    type="button"
                    className="scheduler-btn"
                    disabled={busy}
                    onClick={() => {
                      setMode("run");
                      void run(true);
                    }}
                  >
                    {t("miniapps.testRun")}
                  </button>
                  <input
                    className="miniapps-version"
                    aria-label={t("miniapps.version")}
                    value={releaseVersion}
                    onChange={(event) => setReleaseVersion(event.target.value)}
                  />
                  <button
                    type="button"
                    className="scheduler-btn scheduler-btn-primary"
                    disabled={busy}
                    onClick={() => void release()}
                  >
                    {t("miniapps.release")}
                  </button>
                </div>
              </footer>
            </>
          )}
        </section>
      </div>
    </aside>
  );
}
