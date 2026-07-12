import { useCallback, useEffect, useMemo, useState } from "react";
import { useT } from "../i18n/I18nProvider";
import { Combobox } from "../settings/Combobox";
import { useProbeModels } from "./useProbeModels";

export type ProviderPresetId =
  | "openai"
  | "anthropic"
  | "ollama"
  | "compatible"
  | "neuraldeep";

type ProviderPreset = {
  id: ProviderPresetId;
  labelKey?: string;
  descriptionKey?: string;
  label: string;
  description: string;
  providerName: string;
  providerType: "openai" | "anthropic" | "neuraldeep";
  apiBase?: string;
  /**
   * The provider type pins its own endpoint, so apiBase is display-only: shown
   * read-only (without /v1) and never probed or written to the config.
   */
  apiBaseFixed?: boolean;
  /** Whether the provider's models accept image/file inputs (models[].multimodal). */
  multimodal: boolean;
  defaultModel: string;
  envKey?: string;
  website?: string;
};

/** Strip a trailing /v1 for display; the saved api_base keeps it (the OpenAI SDK
 * does not append /v1 itself). */
function stripV1(base: string): string {
  return base.replace(/\/v1\/?$/, "");
}

const PRESETS: ProviderPreset[] = [
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
    description: "Russian AI hub — models via api.neuraldeep.ru",
    providerName: "neuraldeep",
    providerType: "neuraldeep",
    multimodal: true,
    apiBase: "https://api.neuraldeep.ru/v1",
    apiBaseFixed: true,
    defaultModel: "neuraldeep/default",
    envKey: "${NEURALDEEP_API_KEY}",
    website: "https://hub.neuraldeep.ru",
  },
];

function buildConfigBody(
  preset: ProviderPreset,
  apiKey: string,
  apiBase: string,
  proxy: string,
  modelId: string,
  baseDoc: Record<string, unknown>,
): Record<string, unknown> {
  const provider: Record<string, unknown> = {
    name: preset.providerName,
    type: preset.providerType,
    api_key: apiKey.trim() || preset.envKey || "",
  };
  // Presets whose provider type pins its own endpoint never write api_base: the
  // backend would ignore it, and an editable-looking value in the saved YAML only
  // invites confusion.
  const base = apiBase.trim() || preset.apiBase || "";
  if (!preset.apiBaseFixed && base) {
    provider.api_base = base;
  }
  // The proxy applies to every provider type (including neuraldeep, whose base
  // URL is fixed but still routed through the proxy), so it is orthogonal to
  // apiBaseFixed. Only write it when set — an empty proxy means direct.
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
  return {
    ...baseDoc,
    providers: [provider],
    models: [
      {
        model,
        max_tokens: 8192,
        temperature: 0.2,
        multimodal: preset.multimodal,
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
  const [proxy, setProxy] = useState("");
  const [modelId, setModelId] = useState("");
  const [showKey, setShowKey] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [testOk, setTestOk] = useState(false);
  const [baseDoc, setBaseDoc] = useState<Record<string, unknown>>({});
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

  /** Base URL sent when probing the provider's model list. Presets with a fixed
   * endpoint send nothing: the backend pins the URL from the provider type. */
  const probeApiBase = preset.apiBaseFixed
    ? ""
    : apiBase.trim() || preset.apiBase || "";

  const presetLabel = useCallback(
    (p: ProviderPreset) => (p.labelKey ? t(p.labelKey) : p.label),
    [t],
  );

  const presetDescription = useCallback(
    (p: ProviderPreset) => (p.descriptionKey ? t(p.descriptionKey) : p.description),
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
    setModelId("");
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
    const key = apiKey.trim();
    if (!key) {
      return;
    }
    const type = preset.providerType;
    const base = probeApiBase;
    const proxyURL = proxy.trim();
    const handle = window.setTimeout(() => {
      void probeModels({ type, api_base: base, api_key: key, proxy: proxyURL });
    }, 600);
    return () => window.clearTimeout(handle);
  }, [props.open, apiKey, probeApiBase, proxy, preset.providerType, probeModels]);

  const configBody = useMemo(
    () =>
      buildConfigBody(
        preset,
        apiKey,
        apiBase,
        proxy,
        modelId,
        baseDoc,
      ),
    [preset, apiKey, apiBase, proxy, modelId, baseDoc],
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
      // GET /v1/models lists the synthetic agent/plan/docs pseudo-models
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
      setError(e instanceof Error ? e.message : t("onboarding.connectionFailed"));
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
      setError(e instanceof Error ? e.message : t("onboarding.saveFailedGeneric"));
    } finally {
      setBusy(false);
    }
  }, [configBody, props, t]);

  if (!props.open) {
    return null;
  }

  const showApiBase =
    selected === "compatible" || selected === "ollama" || selected === "neuraldeep";

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
              <span className="provider-picker-card-title">{presetLabel(p)}</span>
              <span className="provider-picker-card-desc">
                {presetDescription(p)}
              </span>
            </button>
          ))}
        </div>

        <div className="provider-picker-form">
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

          {showApiBase ? (
            <label className="provider-picker-field">
              <span>{t("onboarding.apiBase")}</span>
              {preset.apiBaseFixed ? (
                <input
                  className="provider-picker-input provider-picker-input--readonly"
                  value={stripV1(preset.apiBase || "")}
                  readOnly
                  data-testid="provider-api-base"
                />
              ) : (
                <input
                  className="provider-picker-input"
                  value={apiBase}
                  onChange={(ev) => setApiBase(ev.target.value)}
                  placeholder="https://api.example.com/v1"
                  data-testid="provider-api-base"
                />
              )}
            </label>
          ) : null}

          <label className="provider-picker-field">
            <span>{t("onboarding.proxy")}</span>
            <input
              className="provider-picker-input"
              value={proxy}
              onChange={(ev) => setProxy(ev.target.value)}
              placeholder="socks5h://127.0.0.1:1080"
              autoComplete="off"
              data-testid="provider-proxy"
            />
            <span className="provider-picker-hint">{t("onboarding.proxyHint")}</span>
          </label>

          {preset.website ? (
            <a
              className="provider-picker-hub-link"
              href={preset.website}
              target="_blank"
              rel="noopener noreferrer"
              data-testid="provider-hub-link"
            >
              {t("onboarding.openHub")} ↗
            </a>
          ) : null}

          <label className="provider-picker-field">
            <span>{t("onboarding.defaultModel")}</span>
            <div className="provider-picker-key-row">
              <Combobox
                value={modelId}
                onChange={setModelId}
                options={fetchedModels.map((m) => ({
                  value: m.id,
                  label: m.name || m.id,
                }))}
                placeholder={preset.defaultModel}
                ariaLabel={t("onboarding.defaultModel")}
                testid="provider-model-id"
                openUp
              />
              <button
                type="button"
                className="provider-picker-ghost-btn"
                onClick={() =>
                  void probeModels({
                    type: preset.providerType,
                    api_base: probeApiBase,
                    api_key: apiKey.trim(),
                    proxy: proxy.trim(),
                  })
                }
                disabled={modelsLoading || !apiKey.trim()}
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
              <span className="provider-picker-hint" data-testid="provider-models-error">
                {t("onboarding.modelsFetchFailed")}
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
