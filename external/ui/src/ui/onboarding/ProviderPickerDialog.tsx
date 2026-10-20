import { useCallback, useEffect, useMemo, useState } from "react";
import { useT } from "../i18n/I18nProvider";
import { Combobox } from "../settings/Combobox";
import { CodexAuthField } from "../settings/CodexAuthField";
import {
  NEURALDEEP_API_BASE_OPTIONS,
  NEURALDEEP_DEFAULT_API_BASE,
  neuralDeepHubFor,
} from "../settings/neuralDeepEndpoints";
import { NeuralDeepAuthField } from "../settings/NeuralDeepAuthField";
import { ProxyUrlField } from "../settings/ProxyUrlField";
import type { FetchedModel } from "../settings/useProviderModels";
import { useProbeModels } from "./useProbeModels";

export type ProviderPresetId =
  | "openai"
  | "anthropic"
  | "ollama"
  | "compatible"
  | "neuraldeep"
  | "codex";

type ProviderPreset = {
  id: ProviderPresetId;
  labelKey?: string;
  descriptionKey?: string;
  label: string;
  description: string;
  providerName: string;
  providerType: "openai" | "anthropic" | "neuraldeep" | "codex";
  apiBase?: string;
  /** Whether the provider's models accept image/file inputs (models[].multimodal). */
  multimodal: boolean;
  defaultModel: string;
  /**
   * Context window written as models[].max_context_tokens when the preset
   * knows one. Unset leaves the field empty, so the provider's own model
   * listing fills it. NeuralDeep's free tier caps a request at 46112 input
   * tokens regardless of the model's native window, so its preset pins that
   * number: the context ring and the compaction trigger then measure against
   * the tier's ceiling, not the catalog's.
   */
  maxContextTokens?: number;
  envKey?: string;
  website?: string;
};

// A non-empty tuple, not a plain array: every lookup here falls back to PRESETS[0],
// and under noUncheckedIndexedAccess a plain array would make that fallback
// `ProviderPreset | undefined` and force a null check on a list that is a source
// constant and can never be empty.
const PRESETS: [ProviderPreset, ...ProviderPreset[]] = [
  {
    id: "openai",
    label: "OpenAI",
    descriptionKey: "onboarding.provider.openai.description",
    description: "GPT-4o and compatible models",
    providerName: "openai",
    providerType: "openai",
    multimodal: true,
    defaultModel: "openai/gpt-4o",
    envKey: "${OPENAI_API_KEY}",
  },
  {
    id: "anthropic",
    label: "Anthropic",
    descriptionKey: "onboarding.provider.anthropic.description",
    description: "Claude models",
    providerName: "anthropic",
    providerType: "anthropic",
    multimodal: false,
    defaultModel: "anthropic/claude-sonnet-4-20250514",
    envKey: "${ANTHROPIC_API_KEY}",
  },
  {
    id: "ollama",
    label: "Ollama",
    descriptionKey: "onboarding.provider.ollama.description",
    description: "Local models via OpenAI-compatible API",
    providerName: "local",
    providerType: "openai",
    multimodal: true,
    apiBase: "http://127.0.0.1:11434/v1",
    defaultModel: "local/llama3.2",
  },
  {
    id: "compatible",
    labelKey: "onboarding.provider.compatible.label",
    descriptionKey: "onboarding.provider.compatible.description",
    label: "OpenAI-compatible",
    description: "DeepSeek, Groq, Together, custom api_base",
    providerName: "custom",
    providerType: "openai",
    multimodal: true,
    defaultModel: "custom/gpt-4o",
  },
  {
    id: "neuraldeep",
    labelKey: "onboarding.provider.neuraldeep.label",
    descriptionKey: "onboarding.provider.neuraldeep.description",
    label: "NeuralDeep",
    description: "AI hub — neuraldeep.ru or neuraldeep.tech",
    providerName: "neuraldeep",
    providerType: "neuraldeep",
    multimodal: true,
    apiBase: NEURALDEEP_DEFAULT_API_BASE,
    // The Qwen 3.6 35B A3B id every tier serves (the free tier's only chat
    // model), so the saved ref resolves for a fresh sign-in and a paid key
    // alike. The name must stay lowercase: it is the provider row's own
    // conventional name, and the model prefix has to match it exactly or the
    // saved models[].model names no provider.
    defaultModel: "neuraldeep/qwen3.6-35b-a3b",
    maxContextTokens: 46112,
    envKey: "${NEURALDEEP_API_KEY}",
    website: "https://hub.neuraldeep.ru",
  },
  {
    id: "codex",
    label: "Codex",
    descriptionKey: "onboarding.provider.codex.description",
    description: "Use your ChatGPT subscription through OAuth",
    providerName: "codex",
    providerType: "codex",
    multimodal: false,
    defaultModel: "codex/gpt-5.6-sol",
  },
];

/**
 * catalogEntry finds the probed model behind a model id. The combobox writes a
 * bare id, the fixed-endpoint presets pre-fill a prefixed one, so both forms
 * have to match.
 */
function catalogEntry(
  preset: ProviderPreset,
  modelId: string,
  catalog: FetchedModel[],
): FetchedModel | undefined {
  const raw = modelId.trim();
  if (!raw) {
    return undefined;
  }
  return catalog.find(
    (m) => m.id === raw || `${preset.providerName}/${m.id}` === raw,
  );
}

function buildConfigBody(
  preset: ProviderPreset,
  apiKey: string,
  apiBase: string,
  proxy: string,
  modelId: string,
  baseDoc: Record<string, unknown>,
  catalog: FetchedModel[],
  hubLogin = false,
): Record<string, unknown> {
  const provider: Record<string, unknown> = {
    name: preset.providerName,
    type: preset.providerType,
  };
  // A hub login is the credential: writing the env reference (let alone a
  // typed key) would shadow the stored login, so the row stays keyless and
  // requests resolve the key from the login file. Codex never carries a key.
  const keyless = preset.providerType === "codex" || hubLogin;
  if (!keyless) {
    provider.api_key = apiKey.trim() || preset.envKey || "";
  }
  // NeuralDeep keeps the chosen official deployment; Codex uses its fixed backend.
  const base = apiBase.trim() || preset.apiBase || "";
  if (preset.providerType !== "codex" && base) {
    provider.api_base = base;
  }
  // The proxy applies independently of the selected provider endpoint.
  const proxyURL = proxy.trim();
  if (proxyURL) {
    provider.proxy = proxyURL;
  }
  const rawModel = modelId.trim();
  let model: string;
  if (!rawModel) {
    model = preset.defaultModel;
  } else if (rawModel.includes("/")) {
    model = rawModel;
  } else {
    // Bare id picked from the fetched-models select: prefix the provider name
    // to satisfy the required provider/model_id config format.
    model = `${preset.providerName}/${rawModel}`;
  }
  // A hub serves vision and text-only models side by side, so the per-provider
  // preset constant is only a fallback: whenever the picked id is in the probed
  // catalog, that entry's advisory vision flag decides. It stays advisory - the
  // value is editable afterwards in Settings.
  const listed = catalogEntry(preset, modelId, catalog);
  return {
    ...baseDoc,
    providers: [provider],
    models: [
      {
        model,
        max_tokens: 8192,
        temperature: 0.2,
        ...(preset.maxContextTokens
          ? { max_context_tokens: preset.maxContextTokens }
          : {}),
        multimodal: listed ? listed.vision === true : preset.multimodal,
      },
    ],
    agent: {
      ...(typeof baseDoc.agent === "object" && baseDoc.agent
        ? (baseDoc.agent as Record<string, unknown>)
        : {}),
      model,
      max_turns: 35,
    },
  };
}

export function ProviderPickerDialog(props: {
  open: boolean;
  onSaved: () => void;
  onSkip: () => void;
}) {
  const { t } = useT();
  const [selected, setSelected] = useState<ProviderPresetId>("openai");
  const [apiKey, setApiKey] = useState("");
  const [apiBase, setApiBase] = useState("");
  const [neuralDeepApiBase, setNeuralDeepApiBase] = useState<string>(
    NEURALDEEP_DEFAULT_API_BASE,
  );
  const [proxy, setProxy] = useState("");
  const [modelId, setModelId] = useState("");
  const [showKey, setShowKey] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [testOk, setTestOk] = useState(false);
  const [baseDoc, setBaseDoc] = useState<Record<string, unknown>>({});
  // True while the NeuralDeep hub login is the credential the saved row will
  // authenticate with: the device-flow widget set it, and no key was typed.
  const [hubLogin, setHubLogin] = useState(false);
  const {
    loading: modelsLoading,
    models: fetchedModels,
    error: modelsError,
    fetched: modelsFetched,
    probe: probeModels,
    reset: resetModels,
  } = useProbeModels();

  const preset = useMemo(
    () => PRESETS.find((p) => p.id === selected) ?? PRESETS[0],
    [selected],
  );

  const effectiveApiBase =
    selected === "neuraldeep"
      ? neuralDeepApiBase
      : apiBase.trim() || preset.apiBase || "";
  const probeApiBase = preset.providerType === "codex" ? "" : effectiveApiBase;
  const hubUrl =
    selected === "neuraldeep"
      ? neuralDeepHubFor(neuralDeepApiBase)
      : preset.website;

  /** A neuraldeep probe without a typed key reads the provider's stored hub
   * login server-side, so the catalog is listed for a device-flow sign-in the
   * same way Codex lists its models through the OAuth file. */
  const probeUsesStoredLogin =
    preset.providerType === "neuraldeep" && !apiKey.trim() && hubLogin;
  const probeInput = useMemo(
    () => ({
      type: preset.providerType,
      ...(preset.providerType === "codex" || probeUsesStoredLogin
        ? { provider_name: preset.providerName }
        : {}),
      api_base: probeApiBase,
      api_key:
        preset.providerType === "codex" || probeUsesStoredLogin
          ? ""
          : apiKey.trim(),
      proxy: proxy.trim(),
    }),
    [
      apiKey,
      preset.providerName,
      preset.providerType,
      probeApiBase,
      probeUsesStoredLogin,
      proxy,
    ],
  );

  const presetLabel = useCallback(
    (p: ProviderPreset) => (p.labelKey ? t(p.labelKey) : p.label),
    [t],
  );

  const presetDescription = useCallback(
    (p: ProviderPreset) =>
      p.descriptionKey ? t(p.descriptionKey) : p.description,
    [t],
  );

  useEffect(() => {
    if (!props.open) return;
    setError(null);
    setTestOk(false);
    void fetch("/foxxycode/config")
      .then((r) => (r.ok ? r.json() : {}))
      .then((doc) => setBaseDoc((doc as Record<string, unknown>) || {}))
      .catch(() => setBaseDoc({}));
  }, [props.open]);

  useEffect(() => {
    setTestOk(false);
    setError(null);
    setHubLogin(false);
    // Pre-select fixed recommendations so they show in the combobox instead of
    // only as placeholder text; generic API providers start empty.
    const next = PRESETS.find((p) => p.id === selected) ?? PRESETS[0];
    setModelId(
      selected === "neuraldeep" || selected === "codex"
        ? next.defaultModel
        : "",
    );
    resetModels();
    if (selected === "ollama") {
      setApiBase((b) => b || "http://127.0.0.1:11434/v1");
      setApiKey((k) => k || "~");
    }
  }, [selected, resetModels]);

  // Auto-fetch the provider's model list once credentials are in place, so the
  // model can be picked from a select instead of typed by hand. Debounced to
  // avoid probing on every keystroke; the manual refresh button re-probes.
  useEffect(() => {
    if (!props.open) {
      return;
    }
    if (preset.providerType === "codex") {
      return;
    }
    if (!probeInput.api_key && !probeUsesStoredLogin) {
      return;
    }
    const handle = window.setTimeout(() => {
      void probeModels(probeInput);
    }, 600);
    return () => window.clearTimeout(handle);
  }, [
    props.open,
    probeInput,
    probeUsesStoredLogin,
    preset.providerType,
    probeModels,
  ]);

  // The hub login is the credential only while no key is typed: a typed key
  // shadows the stored login (the settings widget says so too), so the saved
  // row must then carry the key, not stay keyless.
  const hubCredential = hubLogin && !apiKey.trim();

  const configBody = useMemo(
    () =>
      buildConfigBody(
        preset,
        apiKey,
        effectiveApiBase,
        proxy,
        modelId,
        baseDoc,
        fetchedModels,
        hubCredential,
      ),
    [
      preset,
      apiKey,
      effectiveApiBase,
      proxy,
      modelId,
      baseDoc,
      fetchedModels,
      hubCredential,
    ],
  );

  /** Catalog entry behind the chosen model, when the probe listed it. */
  const pickedModel = useMemo(
    () => catalogEntry(preset, modelId, fetchedModels),
    [preset, modelId, fetchedModels],
  );

  const testConnection = useCallback(async () => {
    setBusy(true);
    setError(null);
    setTestOk(false);
    try {
      const body = JSON.stringify(configBody);
      const v = await fetch("/foxxycode/config/validate", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body,
      });
      const vj = (await v.json()) as { ok?: boolean; error?: string };
      if (!vj.ok) {
        setError(vj.error || t("onboarding.validationFailed"));
        return;
      }
      const p = await fetch("/foxxycode/config", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body,
      });
      const pj = (await p.json()) as { ok?: boolean; error?: string };
      if (!p.ok || !pj.ok) {
        setError(pj.error || t("onboarding.saveFailed", { status: p.status }));
        return;
      }
      const models = await fetch("/v1/models");
      if (!models.ok) {
        setError(t("onboarding.modelsProbeFailed", { status: models.status }));
        return;
      }
      const mj = (await models.json()) as {
        data?: { id: string; owned_by?: string }[];
      };
      // GET /v1/models lists the synthetic agent/plan/docs/ask pseudo-models
      // (owned_by "foxxycode") before the real configured provider models, so
      // never auto-select one of those — pick the first real provider model.
      if (!modelId.trim() && mj.data && mj.data.length > 0) {
        const real = mj.data.find((m) => m.owned_by !== "foxxycode");
        if (real) {
          setModelId(real.id);
        }
      }
      setTestOk(true);
    } catch (e) {
      setError(
        e instanceof Error ? e.message : t("onboarding.connectionFailed"),
      );
    } finally {
      setBusy(false);
    }
  }, [configBody, modelId, t]);

  const save = useCallback(async () => {
    setBusy(true);
    setError(null);
    try {
      const body = JSON.stringify(configBody);
      const v = await fetch("/foxxycode/config/validate", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body,
      });
      const vj = (await v.json()) as { ok?: boolean; error?: string };
      if (!vj.ok) {
        setError(vj.error || t("onboarding.validationFailed"));
        return;
      }
      const p = await fetch("/foxxycode/config", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body,
      });
      const pj = (await p.json()) as { ok?: boolean; error?: string };
      if (!p.ok || !pj.ok) {
        setError(pj.error || t("onboarding.saveFailed", { status: p.status }));
        return;
      }
      props.onSaved();
    } catch (e) {
      setError(
        e instanceof Error ? e.message : t("onboarding.saveFailedGeneric"),
      );
    } finally {
      setBusy(false);
    }
  }, [configBody, props, t]);

  if (!props.open) {
    return null;
  }

  const showApiBase = selected === "compatible" || selected === "ollama";

  return (
    <div className="provider-picker-host" data-testid="provider-picker-dialog">
      <button
        type="button"
        className="backdrop is-open"
        aria-label={t("onboarding.close")}
        onClick={props.onSkip}
      />
      <div
        className="provider-picker-panel"
        role="dialog"
        aria-modal="true"
        aria-labelledby="provider-picker-title"
      >
        <div className="provider-picker-head">
          <h2 id="provider-picker-title">{t("onboarding.title")}</h2>
          <p className="provider-picker-lead">{t("onboarding.lead")}</p>
        </div>

        <div className="provider-picker-grid">
          {PRESETS.map((p) => (
            <button
              key={p.id}
              type="button"
              className={[
                "provider-picker-card",
                selected === p.id ? "provider-picker-card--active" : "",
              ]
                .filter(Boolean)
                .join(" ")}
              data-testid={`provider-card-${p.id}`}
              onClick={() => setSelected(p.id)}
            >
              <span className="provider-picker-card-title">
                {presetLabel(p)}
              </span>
              <span className="provider-picker-card-desc">
                {presetDescription(p)}
              </span>
            </button>
          ))}
        </div>

        <div className="provider-picker-form">
          {selected === "neuraldeep" ? (
            <label className="provider-picker-field">
              <span>{t("onboarding.neuralDeepDomain")}</span>
              <select
                className="provider-picker-input"
                value={neuralDeepApiBase}
                onChange={(ev) => {
                  setNeuralDeepApiBase(ev.target.value);
                  setHubLogin(false);
                  setTestOk(false);
                  setError(null);
                  resetModels();
                }}
                aria-label={t("onboarding.neuralDeepDomain")}
                data-testid="provider-neuraldeep-domain"
              >
                {NEURALDEEP_API_BASE_OPTIONS.map((option) => (
                  <option key={option.value} value={option.value}>
                    {t(option.labelKey)}
                  </option>
                ))}
              </select>
            </label>
          ) : null}
          {selected === "codex" ? (
            <CodexAuthField providerName={preset.providerName} />
          ) : (
            <label className="provider-picker-field">
              <span>{t("onboarding.apiKey")}</span>
              <div className="provider-picker-key-row">
                <input
                  className="provider-picker-input"
                  type={showKey ? "text" : "password"}
                  value={apiKey}
                  onChange={(ev) => setApiKey(ev.target.value)}
                  placeholder={preset.envKey || "sk-..."}
                  autoComplete="off"
                  data-testid="provider-api-key"
                />
                <button
                  type="button"
                  className="provider-picker-ghost-btn"
                  onClick={() => setShowKey((v) => !v)}
                >
                  {showKey ? t("onboarding.hideKey") : t("onboarding.showKey")}
                </button>
              </div>
            </label>
          )}

          {selected === "neuraldeep" ? (
            <NeuralDeepAuthField
              providerName={preset.providerName}
              hasExplicitKey={apiKey.trim() !== ""}
              apiBase={neuralDeepApiBase}
              descriptionKey="onboarding.neuralDeepAuth.description"
              fieldLabelKey="onboarding.neuralDeepAuth.fieldLabel"
              onConnectedChange={setHubLogin}
            />
          ) : null}

          {showApiBase ? (
            <label className="provider-picker-field">
              <span>{t("onboarding.apiBase")}</span>
              <input
                className="provider-picker-input"
                value={apiBase}
                onChange={(ev) => setApiBase(ev.target.value)}
                placeholder="https://api.example.com/v1"
                data-testid="provider-api-base"
              />
            </label>
          ) : null}

          <label className="provider-picker-field">
            <span>{t("onboarding.proxy")}</span>
            <ProxyUrlField
              value={proxy}
              onChange={setProxy}
              ariaLabel={t("onboarding.proxy")}
              placeholder="socks5h://127.0.0.1:1080"
              inputClassName="provider-picker-input"
              rowClassName="provider-picker-key-row"
              buttonClassName="provider-picker-ghost-btn"
              dataTestId="provider-proxy"
            />
            <span className="provider-picker-hint">
              {t("onboarding.proxyHint")}
            </span>
          </label>

          {hubUrl ? (
            <a
              className="provider-picker-hub-link"
              href={hubUrl}
              target="_blank"
              rel="noopener noreferrer"
              data-testid="provider-hub-link"
            >
              {t("onboarding.openHub", { domain: new URL(hubUrl).host })} ↗
            </a>
          ) : null}

          <label className="provider-picker-field">
            <span>{t("onboarding.defaultModel")}</span>
            <div className="provider-picker-key-row">
              <Combobox
                value={modelId}
                onChange={setModelId}
                options={fetchedModels.map((m) => {
                  const label = m.name || m.id;
                  return {
                    value: m.id,
                    label: m.vision
                      ? t("settings.modelVisionOption", { model: label })
                      : label,
                  };
                })}
                placeholder={preset.defaultModel}
                ariaLabel={t("onboarding.defaultModel")}
                testid="provider-model-id"
                openUp
              />
              <button
                type="button"
                className="provider-picker-ghost-btn"
                onClick={() => void probeModels(probeInput)}
                disabled={
                  modelsLoading ||
                  (preset.providerType !== "codex" &&
                    !apiKey.trim() &&
                    !probeUsesStoredLogin)
                }
                data-testid="provider-fetch-models"
              >
                {modelsLoading
                  ? t("onboarding.fetchingModels")
                  : fetchedModels.length > 0
                    ? t("onboarding.refreshModels")
                    : t("onboarding.fetchModels")}
              </button>
            </div>
            {modelsFetched && modelsError ? (
              <span
                className="provider-picker-hint"
                data-testid="provider-models-error"
              >
                {t("onboarding.modelsFetchFailed")}
              </span>
            ) : null}
            {pickedModel ? (
              <span
                className="provider-picker-hint"
                data-testid="provider-multimodal-note"
              >
                {pickedModel.vision
                  ? t("onboarding.modelMultimodalOn")
                  : t("onboarding.modelMultimodalOff")}
              </span>
            ) : null}
          </label>

          {error ? (
            <div className="provider-picker-error" data-testid="provider-error">
              {error}
            </div>
          ) : null}
          {testOk ? (
            <div className="provider-picker-ok" data-testid="provider-test-ok">
              {t("onboarding.testOk")}
            </div>
          ) : null}
        </div>

        <div className="provider-picker-actions">
          <button
            type="button"
            className="provider-picker-secondary"
            onClick={props.onSkip}
            disabled={busy}
            data-testid="provider-skip"
          >
            {t("onboarding.skip")}
          </button>
          <button
            type="button"
            className="provider-picker-secondary"
            onClick={() => void testConnection()}
            disabled={busy}
            data-testid="provider-test"
          >
            {t("onboarding.test")}
          </button>
          <button
            type="button"
            className="provider-picker-primary"
            onClick={() => void save()}
            disabled={busy}
            data-testid="provider-save"
          >
            {t("onboarding.save")}
          </button>
        </div>
      </div>
    </div>
  );
}
