import { AppearanceThemePicker } from "../theme/AppearanceModal";
import { GeneralLocalePicker, GeneralSendModePicker } from "./GeneralSection";
import { useT } from "../i18n/I18nProvider";
import { tSchemaText } from "../i18n/schemaStrings";
import { ModelField } from "./ModelField";
import { ModelPicker } from "./ModelPicker";
import { SchemaForm, type FieldOverride, type JsonSchema } from "./SchemaForm";
import { SettingsArraySection } from "./SettingsArraySection";
import { SkillsSection } from "./SkillsSection";
import { ProviderExportButtons } from "./ProviderExportButtons";
import { ProviderImportMenu } from "./ProviderImportMenu";
import { uniqueProviderName } from "./providerTransfer";
import type { SectionDescriptor } from "./settingsSections";

const NEURALDEEP_API_BASE = "https://api.neuraldeep.ru/v1";

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
  const { schema } = props.ctx;
  const label = tSchemaText(schema.title) || "API base URL";
  const desc = tSchemaText(schema.description);

  // NeuralDeep speaks an OpenAI-compatible API at a fixed endpoint; the base URL
  // is not user-configurable. Show it read-only but do NOT persist it into the
  // config: leaving the stored api_base untouched preserves any value entered for
  // another provider type, so switching back to openai/anthropic restores it. The
  // backend pins the endpoint regardless (llm.providerBaseURL).
  return (
    <div className="settings-row">
      <span className="settings-label">{label}</span>
      {desc ? <p className="settings-field-desc">{desc}</p> : null}
      <input
        className="settings-input"
        type="text"
        value={NEURALDEEP_API_BASE}
        aria-label={label}
        title={desc || undefined}
        readOnly
      />
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
  return <NeuralDeepAPIBaseField ctx={ctx} />;
}

/**
 * SettingsSection renders the active settings tab. Object sections render their
 * sub-schema fields directly (the tab already names the section); array sections
 * become master–detail lists; the System group stacks its child object sections;
 * Skills, General (language) and Appearance are special tabs. Model fields
 * receive custom editors via the SchemaForm fieldOverride hook.
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
  const modelIds = stringList(doc.models, "model");

  const setKey = (key: string, value: unknown) =>
    setDoc({ ...doc, [key]: value });

  if (section.kind === "general") {
    return (
      <>
        <GeneralLocalePicker doc={doc} setDoc={setDoc} />
        <GeneralSendModePicker doc={doc} setDoc={setDoc} />
      </>
    );
  }

  if (section.kind === "appearance") {
    return (
      <>
        <AppearanceThemePicker />
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

  const key = section.schemaKey ?? section.id;

  if (section.kind === "array") {
    const sub = props_[key];
    if (!sub) {
      return <p className="settings-muted">{t("settings.sectionSchemaUnavailable")}</p>;
    }
    const override: FieldOverride | undefined =
      key === "models"
        ? (ctx) =>
            ctx.path === "model" ? (
              <ModelField
                value={ctx.value === undefined || ctx.value === null ? "" : String(ctx.value)}
                onChange={(v) => ctx.onChange(v)}
                providers={providerNames}
                label={tSchemaText(ctx.schema.title) || t("settings.modelIdLabel")}
              />
            ) : null
        : key === "providers"
          ? neuralDeepAPIBaseOverride
          : undefined;
    const isProviders = key === "providers";
    return (
      <SettingsArraySection
        schema={sub}
        value={asArray(doc[key])}
        onChange={(v) => setKey(key, v)}
        labelField={section.labelField}
        fieldOverride={override}
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
    key === "agent" || key === "memory"
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
