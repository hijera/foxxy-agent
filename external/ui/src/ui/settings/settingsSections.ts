import { t } from "../i18n/i18n";
import { tSchemaText } from "../i18n/schemaStrings";
import type { JsonSchema } from "./SchemaForm";

export type SectionKind =
  | "array"
  | "object"
  | "group"
  | "skills"
  | "mcp"
  | "subagents"
  | "appearance"
  | "general"
  | "sessions";

export type SectionDescriptor = {
  /** Unique id: a config key, or a synthetic id ("system", "appearance", "general", "sessions_manager"). */
  id: string;
  /** Tab label. */
  label: string;
  /** Short (3–5 word) blurb shown under the label on the mobile tile grid. */
  description?: string | undefined;
  kind: SectionKind;
  /** Config key for array/object sections. */
  schemaKey?: string | undefined;
  /** For array sections: which item field labels each row in the list. */
  labelField?: string | undefined;
  /** For group sections: config keys grouped under this tab. */
  childKeys?: string[] | undefined;
};

/**
 * Config keys never shown as their own schema tab. `ui` (ui.locale) is edited
 * by the curated language picker in the synthetic General tab; a raw schema
 * form for it would be a duplicate control. The key still round-trips through
 * the footer Save because the whole config doc is PUT back unchanged.
 */
const HIDDEN_KEYS = ["ui"];

/** Config keys folded into the single "System" tab (rarely edited). */
export const SYSTEM_KEYS = [
  "scheduler",
  "prompts",
  "instructions",
  "logger",
  "sessions",
  "gateways",
];

/** Array sections shown as master–detail lists, with the field used as the row label. */
export const ARRAY_LABEL_FIELDS: Record<string, string> = {
  providers: "name",
  models: "model",
};

/**
 * Section ids that have a curated i18n blurb (`settings.section.<id>.desc`) for
 * the mobile tile grid. Schema `description` strings are full sentences (or
 * missing), so these short 3–5 word summaries keep the tiles readable; unmapped
 * keys fall back to the schema description.
 */
const SECTION_DESC_IDS = new Set([
  "general",
  "appearance",
  "sessions_manager",
  "providers",
  "models",
  "agent",
  "compaction",
  "autocomplete",
  "tools",
  "subagents",
  "hooks",
  "mcp_servers",
  "skills",
  "memory",
  "title",
  "browser",
  "vcs",
  "debug",
  "system",
]);

/**
 * i18n keys for known section labels (`settings.section.<id>.label`). The
 * schema `title` is written as a form heading and reads as a sentence, which
 * does not fit the tab rail or the 2-wide mobile tile grid; these curated
 * short forms do. Unknown schema sections keep their server-provided title.
 */
const SECTION_LABEL_KEYS: Record<string, string> = {
  general: "settings.section.general.label",
  appearance: "settings.section.appearance.label",
  sessions_manager: "settings.section.sessions_manager.label",
  providers: "settings.section.providers.label",
  models: "settings.section.models.label",
  agent: "settings.section.agent.label",
  compaction: "settings.section.compaction.label",
  autocomplete: "settings.section.autocomplete.label",
  tools: "settings.section.tools.label",
  subagents: "settings.section.subagents.label",
  hooks: "settings.section.hooks.label",
  mcp_servers: "settings.section.mcp_servers.label",
  skills: "settings.section.skills.label",
  memory: "settings.section.memory.label",
  title: "settings.section.title.label",
  browser: "settings.section.browser.label",
  vcs: "settings.section.vcs.label",
  debug: "settings.section.debug.label",
  system: "settings.section.system.label",
};

/** Resolve a tile blurb: curated i18n summary first, then the schema description. */
function descFor(id: string, sub?: JsonSchema): string | undefined {
  if (SECTION_DESC_IDS.has(id)) {
    return t(`settings.section.${id}.desc`);
  }
  return tSchemaText(sub?.description) || undefined;
}

/** Resolve a section label: curated short i18n form first, then the schema title. */
function labelFor(id: string, sub?: JsonSchema): string {
  const key = SECTION_LABEL_KEYS[id];
  return key ? t(key) : tSchemaText(sub?.title) || id;
}

/**
 * deriveSettingsSections turns the root config JSON Schema into ordered tab
 * descriptors. Top-level schema properties map 1:1 to tabs (using the schema's
 * `x-foxxycode-property-order` and each property's `title`), except that the rarely
 * edited tail keys are folded into a single "System" tab and three synthetic
 * tabs lead the list: "General" (the UI language picker, the default tab),
 * "Appearance" (the client-side theme picker) and "Sessions" (the stored
 * history as a table). All three are present even when no schema is available.
 */
export function deriveSettingsSections(
  schema: JsonSchema | null | undefined,
): SectionDescriptor[] {
  const general: SectionDescriptor = {
    id: "general",
    label: labelFor("general"),
    description: descFor("general"),
    kind: "general",
  };
  const appearance: SectionDescriptor = {
    id: "appearance",
    label: labelFor("appearance"),
    description: descFor("appearance"),
    kind: "appearance",
  };

  // The stored history is managed, not configured: this tab reads and prunes
  // session bundles over /foxxycode/sessions and edits no config key. Its id is
  // sessions_manager because `sessions` is already a config key (the storage
  // directory), folded into the System tab.
  const sessionsManager: SectionDescriptor = {
    id: "sessions_manager",
    label: labelFor("sessions_manager"),
    description: descFor("sessions_manager"),
    kind: "sessions",
  };

  if (!schema || schema.type !== "object" || !schema.properties) {
    return [general, appearance, sessionsManager];
  }

  const props = schema.properties;
  const order =
    schema["x-foxxycode-property-order"] && schema["x-foxxycode-property-order"].length
      ? schema["x-foxxycode-property-order"]
      : Object.keys(props).sort();

  const out: SectionDescriptor[] = [];
  const seen = new Set<string>();
  let systemEmitted = false;

  const emit = (key: string) => {
    const sub = props[key];
    if (!sub || seen.has(key) || HIDDEN_KEYS.includes(key)) {
      return;
    }
    seen.add(key);
    if (SYSTEM_KEYS.includes(key)) {
      if (!systemEmitted) {
        out.push({
          id: "system",
          label: labelFor("system"),
          description: descFor("system"),
          kind: "group",
          childKeys: SYSTEM_KEYS.filter((k) => props[k] !== undefined),
        });
        systemEmitted = true;
      }
      return;
    }
    if (key === "skills") {
      out.push({
        id: key,
        label: labelFor(key, sub),
        description: descFor(key, sub),
        kind: "skills",
        schemaKey: key,
      });
      return;
    }
    if (key === "mcp_servers") {
      out.push({
        id: key,
        label: labelFor(key, sub),
        description: descFor(key, sub),
        kind: "mcp",
        schemaKey: key,
      });
      return;
    }
    // Subagents is a hybrid tab: the generated form for the config section,
    // plus the definition catalog with the per-workspace approvals, which are
    // receipts rather than configuration.
    if (key === "subagents") {
      out.push({
        id: key,
        label: labelFor(key, sub),
        description: descFor(key, sub),
        kind: "subagents",
        schemaKey: key,
      });
      return;
    }
    if (key in ARRAY_LABEL_FIELDS) {
      out.push({
        id: key,
        label: labelFor(key, sub),
        description: descFor(key, sub),
        kind: "array",
        schemaKey: key,
        labelField: ARRAY_LABEL_FIELDS[key],
      });
      return;
    }
    out.push({
      id: key,
      label: labelFor(key, sub),
      description: descFor(key, sub),
      kind: "object",
      schemaKey: key,
    });
  };

  for (const key of order) {
    emit(key);
  }
  // Cover any properties not named in the order array.
  for (const key of Object.keys(props).sort()) {
    emit(key);
  }

  return [general, appearance, sessionsManager, ...out];
}
