import { AppearanceThemePicker } from "../theme/AppearanceModal";
import {
  GeneralSendModePicker,
  GeneralStatusLinePicker,
} from "./GeneralSection";
import { useT } from "../i18n/I18nProvider";
import { tSchemaText } from "../i18n/schemaStrings";
import { applyModelsChange } from "./applyModelsChange";
import { CodexAuthField } from "./CodexAuthField";
import { ModelField } from "./ModelField";
import { ModelPicker } from "./ModelPicker";
import { ReasoningLevelsField } from "./ReasoningLevelsField";
import {
  defaultForSchema,
  SchemaForm,
  type FieldOverride,
  type JsonSchema,
} from "./SchemaForm";
import { NeuralDeepAuthField } from "./NeuralDeepAuthField";
import { MCPSection } from "./MCPSection";
import { SettingsArraySection } from "./SettingsArraySection";
import { SkillsSection } from "./SkillsSection";
import { ProviderExportButtons } from "./ProviderExportButtons";
import { ProviderImportMenu } from "./ProviderImportMenu";
import { uniqueProviderName } from "./providerTransfer";
import type { SectionDescriptor } from "./settingsSections";

// The deployments a neuraldeep provider may point at, mirroring
// neuralDeepEndpoints in internal/llm/neuraldeep_auth.go. The backend ignores
// anything else in api_base and falls back to the first entry.
const NEURALDEEP_API_BASE_OPTIONS = [
  {
    value: "https://api.neuraldeep.ru/v1",
    labelKey: "settings.neuralDeepApiBase.optionRu",
  },
  {
    value: "https://api.neuraldeep.tech/v1",
    labelKey: "settings.neuralDeepApiBase.optionTech",
  },
] as const;
const NEURALDEEP_DEFAULT_API_BASE = NEURALDEEP_API_BASE_OPTIONS[0].value;

/** Canonical spelling of a stored api_base, or "" when it names no NeuralDeep endpoint. */
function matchNeuralDeepAPIBase(value: unknown): string {
  let want = String(value ?? "").trim();
  while (want.endsWith("/")) {
    want = want.slice(0, -1);
  }
  want = want.toLowerCase();
  return (
    NEURALDEEP_API_BASE_OPTIONS.find((o) => o.value.toLowerCase() === want)
      ?.value ?? ""
  );
}

type FieldOverrideContext = Parameters<FieldOverride>[0];

function asObject(v: unknown): Record<string, unknown> {
  return v && typeof v === "object" && !Array.isArray(v)
    ? (v as Record<string, unknown>)
    : {};
}

function asArray(v: unknown): unknown[] {
  return Array.isArray(v) ? v : [];
}

function stringList(v: unknown, key: string): string[] {
  return asArray(v)
    .map((row) => {
      if (row && typeof row === "object" && !Array.isArray(row)) {
        const cell = (row as Record<string, unknown>)[key];
        return cell === undefined || cell === null ? "" : String(cell);
      }
      return "";
    })
    .filter((s) => s.trim() !== "");
}

function NeuralDeepAPIBaseField(props: { ctx: FieldOverrideContext }) {
  const { schema, value, onChange } = props.ctx;
  const { t } = useT();
  const label = tSchemaText(schema.title) || "API base URL";
  const stored = String(value ?? "").trim();
  const matched = matchNeuralDeepAPIBase(value);

  // NeuralDeep speaks an OpenAI-compatible API at two official deployments:
  // api.neuraldeep.ru for Russia, api.neuraldeep.tech for everywhere else. Only
  // those are offered, and the choice also decides which hub mints the key for
  // the sign-in block below. Nothing is written until the user picks one, so a
  // base entered for another provider type survives switching to neuraldeep and
  // back; meanwhile the select shows the endpoint requests really use and the
  // note below explains why the stored value is not it.
  return (
    <div className="settings-row">
      <span className="settings-label">{label}</span>
      <p className="settings-field-desc">
        {t("settings.neuralDeepApiBase.description")}
      </p>
      <select
        className="settings-input"
        value={matched || NEURALDEEP_DEFAULT_API_BASE}
        aria-label={label}
        data-testid="neuraldeep-api-base"
        onChange={(e) => onChange(e.target.value)}
      >
        {NEURALDEEP_API_BASE_OPTIONS.map((opt) => (
          <option key={opt.value} value={opt.value}>
            {t(opt.labelKey)}
          </option>
        ))}
      </select>
      {stored !== "" && matched === "" ? (
        <p className="settings-field-desc">
          {t("settings.neuralDeepApiBase.unknown", {
            value: stored,
            fallback: NEURALDEEP_DEFAULT_API_BASE,
          })}
        </p>
      ) : null}
    </div>
  );
}

function neuralDeepAPIBaseOverride(ctx: FieldOverrideContext) {
  const providerType =
    ctx.parentObj?.type === undefined || ctx.parentObj.type === null
      ? ""
      : String(ctx.parentObj.type);
  if (ctx.path !== "api_base" || providerType !== "neuraldeep") {
    return null;
  }
  // The overrides stack: the endpoint picker keeps the api_base slot, and the
  // hub sign-in block renders below it. The manual api_key field above stays
  // fully functional - an explicit key wins over the stored login, which the
  // sign-in block reports instead of hiding.
  const providerName =
    ctx.parentObj?.name === undefined || ctx.parentObj.name === null
      ? ""
      : String(ctx.parentObj.name);
  const hasExplicitKey =
    String(ctx.parentObj?.api_key ?? "").trim() !== "" ||
    String(ctx.parentObj?.api_key_command ?? "").trim() !== "";
  // The endpoint requests really use for this row: the picked one, or the
  // default when the stored value names no NeuralDeep deployment.
  const apiBase =
    matchNeuralDeepAPIBase(ctx.value) || NEURALDEEP_DEFAULT_API_BASE;
  return (
    <>
      <NeuralDeepAPIBaseField ctx={ctx} />
      <NeuralDeepAuthField
        providerName={providerName}
        hasExplicitKey={hasExplicitKey}
        apiBase={apiBase}
      />
    </>
  );
}

function providerFieldOverride(ctx: FieldOverrideContext) {
  const providerType =
    ctx.parentObj?.type === undefined || ctx.parentObj.type === null
      ? ""
      : String(ctx.parentObj.type);
  if (providerType === "codex") {
    if (ctx.path === "api_key" || ctx.path === "api_key_command") {
      return false;
    }
    if (ctx.path === "api_base") {
      const providerName =
        ctx.parentObj?.name === undefined || ctx.parentObj.name === null
          ? ""
          : String(ctx.parentObj.name);
      return <CodexAuthField providerName={providerName} />;
    }
  }
  return neuralDeepAPIBaseOverride(ctx);
}

/**
 * SettingsSection renders the active settings tab. Object sections render their
 * sub-schema fields directly (the tab already names the section); array sections
 * become master–detail lists; the System group stacks its child object sections;
 * Skills, General and Appearance (theme + language) are special tabs. Model
 * fields receive custom editors via the SchemaForm fieldOverride hook.
 */
export function SettingsSection(props: {
  section: SectionDescriptor;
  schema: JsonSchema;
  doc: Record<string, unknown>;
  setDoc: (next: Record<string, unknown>) => void;
  /** Desktop shows the edited item's name on the array-section back button. */
  isMobileShell?: boolean;
  /** Reopen the onboarding form + guided tour (rendered in the Appearance tab). */
  onRestartOnboarding?: (() => void) | undefined;
}) {
  const { t } = useT();
  const { section, schema, doc, setDoc } = props;
  const props_ = schema.properties ?? {};

  const providerNames = stringList(doc.providers, "name");
  // The provider row the model id points at, as it stands in the (unsaved)
  // form: its type decides the Codex reasoning remap server-side, so it must
  // come from the document being edited rather than from the config on disk.
  const providerTypeFor = (modelId: string): string | undefined => {
    const slash = modelId.indexOf("/");
    if (slash <= 0) {
      return undefined;
    }
    const name = modelId.slice(0, slash);
    const row = asArray(doc["providers"]).find(
      (p) => asObject(p)["name"] === name,
    );
    const type = row === undefined ? "" : String(asObject(row)["type"] ?? "");
    return type.trim() || undefined;
  };
  const modelIds = stringList(doc.models, "model");

  const setKey = (key: string, value: unknown) =>
    setDoc({ ...doc, [key]: value });

  if (section.kind === "general") {
    return (
      <>
        <GeneralSendModePicker doc={doc} setDoc={setDoc} />
        <GeneralStatusLinePicker doc={doc} setDoc={setDoc} />
      </>
    );
  }

  if (section.kind === "appearance") {
    return (
      <>
        <AppearanceThemePicker doc={doc} setDoc={setDoc} />
        {props.onRestartOnboarding ? (
          <div className="appearance-onboarding-restart">
            <button
              type="button"
              className="settings-btn"
              data-testid="settings-restart-onboarding"
              onClick={props.onRestartOnboarding}
            >
              {t("settings.restartOnboarding")}
            </button>
            <p className="appearance-onboarding-restart-hint">
              {t("settings.restartOnboardingDesc")}
            </p>
          </div>
        ) : null}
      </>
    );
  }

  if (section.kind === "skills") {
    const sub = props_.skills;
    if (!sub) {
      return <p className="settings-muted">{t("settings.skillsSchemaUnavailable")}</p>;
    }
    return (
      <SkillsSection
        schema={sub}
        value={asObject(doc.skills)}
        onChange={(v) => setKey("skills", v)}
      />
    );
  }

  // The MCP tab is API-driven (/foxxycode/mcp*): toggles and project entries
  // persist into config.yaml / .foxxycode/mcp.json immediately, so it does not
  // edit the settings document at all.
  if (section.kind === "mcp") {
    return <MCPSection />;
  }

  const key = section.schemaKey ?? section.id;

  if (section.kind === "array") {
    const sub = props_[key];
    if (!sub) {
      return <p className="settings-muted">{t("settings.sectionSchemaUnavailable")}</p>;
    }
    const override: FieldOverride | undefined =
      key === "models"
        ? (ctx) => {
            if (ctx.path === "model") {
              return (
                <ModelField
                  value={ctx.value === undefined || ctx.value === null ? "" : String(ctx.value)}
                  // Picking a listed model also seeds the sibling `multimodal`
                  // switch from the catalog's image-input flag, in the same update
                  // as the id. Without it the id is the only thing Settings can
                  // write, which is how a vision model ends up saved as
                  // multimodal:false. A hand-typed id reports no catalog entry, so
                  // the switch keeps whatever the operator set.
                  onChange={(v, picked) => {
                    if (picked && ctx.patchParent) {
                      ctx.patchParent({ model: v, multimodal: picked.vision === true });
                      return;
                    }
                    ctx.onChange(v);
                  }}
                  providers={providerNames}
                  syncsMultimodal
                  label={tSchemaText(ctx.schema.title) || t("settings.modelIdLabel")}
                />
              );
            }
            // The generic array editor cannot express "key absent" (auto-detect)
            // and cannot tell it apart from an explicit [] that hides the
            // reasoning selector, so this field owns all three states.
            if (ctx.path === "reasoning_levels") {
              const modelId =
                ctx.parentObj?.["model"] === undefined ||
                ctx.parentObj?.["model"] === null
                  ? ""
                  : String(ctx.parentObj["model"]);
              return (
                <ReasoningLevelsField
                  value={ctx.value}
                  onChange={(v) => ctx.onChange(v)}
                  model={modelId}
                  providerType={providerTypeFor(modelId)}
                  label={
                    tSchemaText(ctx.schema.title) ||
                    t("settings.reasoning.levelsFallback")
                  }
                  description={tSchemaText(ctx.schema.description)}
                />
              );
            }
            return null;
          }
        : key === "providers"
          ? providerFieldOverride
          : undefined;
    const isProviders = key === "providers";
    // Renaming a logical model id must follow through to the default-model
    // references (agent.model / memory.model), or the saved config becomes
    // invalid ("not found in models list"). applyModelsChange reconciles them.
    const onArrayChange =
      key === "models"
        ? (v: unknown[]) => setDoc(applyModelsChange(doc, v))
        : (v: unknown[]) => setKey(key, v);
    const newItem =
      key === "models"
        ? () => {
            const seed = defaultForSchema(sub.items ?? {});
            if (
              seed === null ||
              typeof seed !== "object" ||
              Array.isArray(seed)
            ) {
              return seed;
            }
            // Empty reasoning_levels explicitly disables server-side detection.
            // A freshly added logical model has no such user choice yet, so omit
            // the optional override and let the backend resolve the model family.
            const { reasoning_levels: _reasoningLevels, ...model } =
              seed as Record<string, unknown>;
            return model;
          }
        : undefined;
    return (
      <SettingsArraySection
        schema={sub}
        value={asArray(doc[key])}
        onChange={onArrayChange}
        labelField={section.labelField}
        fieldOverride={override}
        newItem={newItem}
        backLabelUsesItemName={!props.isMobileShell}
        renderListExtraActions={
          isProviders
            ? ({ appendItems }) => (
                <ProviderImportMenu
                  onImport={(items) => {
                    // Reconcile provider name collisions before appending so the
                    // saved config stays valid (unique provider names).
                    const taken = [...providerNames];
                    const reconciled = items.map((it) => {
                      const nm = uniqueProviderName(
                        String(it.name ?? ""),
                        taken,
                      );
                      if (nm) {
                        taken.push(nm);
                      }
                      return { ...it, name: nm };
                    });
                    appendItems(reconciled, "api_key");
                  }}
                />
              )
            : undefined
        }
        renderItemFooter={
          isProviders
            ? ({ item }) => <ProviderExportButtons provider={item} />
            : undefined
        }
      />
    );
  }

  if (section.kind === "group") {
    const children = section.childKeys ?? [];
    return (
      <div className="settings-group">
        {children.map((ck) => {
          const sub = props_[ck];
          if (!sub) {
            return null;
          }
          return (
            <div key={ck} className="settings-group-block">
              <p className="appearance-section-label">{tSchemaText(sub.title) || ck}</p>
              <SchemaForm
                schema={sub}
                value={asObject(doc[ck])}
                onChange={(v) => setKey(ck, v)}
              />
            </div>
          );
        })}
      </div>
    );
  }

  // object section (agent, tools, memory, …)
  const sub = props_[key];
  if (!sub) {
    return <p className="settings-muted">{t("settings.sectionSchemaUnavailable")}</p>;
  }
  const override: FieldOverride | undefined =
    key === "agent" || key === "memory" || key === "autocomplete"
      ? (ctx) =>
          ctx.path === "model" ? (
            <ModelPicker
              value={ctx.value === undefined || ctx.value === null ? "" : String(ctx.value)}
              onChange={(v) => ctx.onChange(v)}
              models={modelIds}
              label={tSchemaText(ctx.schema.title) || t("settings.defaultModelLabel")}
              description={tSchemaText(ctx.schema.description) || undefined}
            />
          ) : null
      : undefined;
  return (
    <SchemaForm
      schema={sub}
      value={asObject(doc[key])}
      onChange={(v) => setKey(key, v)}
      fieldOverride={override}
    />
  );
}
