import { useState } from "react";
import { t } from "../i18n/i18n";
import { Combobox } from "./Combobox";
import { useProviderModels, type FetchedModel } from "./useProviderModels";

function providerOf(value: string): string {
  const i = value.indexOf("/");
  return i > 0 ? value.slice(0, i) : "";
}

/**
 * ModelField edits a logical model id (provider/api-model-id). The provider is an
 * editable combobox over the configured providers; "Fetch models" pulls the
 * provider's advertised models (Kilo-style) into the model combobox, which is also
 * editable so the id can be typed manually when no list is available.
 *
 * Models the catalog says accept images are badged in the dropdown, and onChange
 * hands the catalog entry back so the caller can seed sibling fields
 * (models[].multimodal) from it. A hand-typed id is not in the catalog, so no
 * entry is reported and nothing is inferred.
 */
export function ModelField(props: {
  value: string;
  onChange: (v: string, picked?: FetchedModel) => void;
  providers: string[];
  label?: string | undefined;
  /**
   * Set by callers that apply the catalog's vision flag to a sibling multimodal
   * field. It only controls the explanatory note: the flag is a default, not a
   * gate, and a silent toggle further down the form reads as a bug.
   */
  syncsMultimodal?: boolean | undefined;
}) {
  const { value, onChange, providers } = props;
  const label = props.label ?? t("settings.modelIdLabel");

  const inferred = providerOf(value);
  const [provider, setProvider] = useState<string>(
    inferred && providers.includes(inferred) ? inferred : providers[0] ?? "",
  );
  const { loading, models, error, fetched, fetchModels, reset } = useProviderModels();

  const byLogicalID = new Map(models.map((m) => [`${provider}/${m.id}`, m]));
  const modelOptions = models.map((m) => {
    const id = `${provider}/${m.id}`;
    const label = m.name ? `${m.name} — ${id}` : id;
    return {
      value: id,
      label: m.vision ? t("settings.modelVisionOption", { model: label }) : label,
    };
  });
  // The note describes the current value only while it is a catalog entry, so it
  // disappears the moment the id is typed by hand.
  const current = byLogicalID.get(value);

  return (
    <div className="settings-row" data-testid="model-field">
      <span className="settings-label">{label}</span>

      <div className="model-field-controls">
        <Combobox
          value={provider}
          onChange={(v) => {
            setProvider(v);
            reset();
          }}
          options={providers.map((p) => ({ value: p }))}
          ariaLabel={t("settings.providerLabel")}
          testid="model-field-provider"
          placeholder="provider"
        />
        <button
          type="button"
          className="settings-btn"
          data-testid="model-field-fetch"
          disabled={!provider || loading}
          onClick={() => void fetchModels(provider)}
        >
          {loading ? t("onboarding.fetchingModels") : t("onboarding.fetchModels")}
        </button>
      </div>

      {fetched && error ? (
        <p className="settings-field-desc">
          {t("settings.modelsFetchError", { error })}
        </p>
      ) : null}
      {fetched && !error && models.length === 0 ? (
        <p className="settings-field-desc">{t("settings.modelsFetchEmpty")}</p>
      ) : null}

      <Combobox
        value={value}
        onChange={(v) => onChange(v, byLogicalID.get(v))}
        options={modelOptions}
        ariaLabel={label}
        testid="model-field-model"
        placeholder="provider/model-id"
      />

      {props.syncsMultimodal && current ? (
        <p
          className="settings-field-desc"
          data-testid="model-field-multimodal-note"
        >
          {current.vision
            ? t("settings.modelMultimodalOn")
            : t("settings.modelMultimodalOff")}
        </p>
      ) : null}
    </div>
  );
}
