import { t } from "../i18n/i18n";
import { tSchemaText } from "../i18n/schemaStrings";
import type { JsonSchema } from "./SchemaForm";

export type SectionKind =
  | "array"
  | "object"
  | "group"
  | "skills"
  | "appearance"
  | "general";

export type SectionDescriptor = {
  /** Unique id: a config key, or a synthetic id ("system", "appearance", "general"). */
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
  mcp_servers: "name",
};

/**
 * Section ids that have a curated i18n blurb (`settings.sectionDesc.<id>`) for
 * the mobile tile grid. Schema `description` strings are full sentences (or
 * missing), so these short 3–5 word summaries keep the tiles readable; unmapped
 * keys fall back to the schema description.
 */
const SECTION_DESC_IDS = new Set([
  "general",
  "appearance",
  "providers",
  "models",
  "agent",
  "tools",
  "mcp_servers",
  "skills",
  "memory",
  "system",
]);

/** Resolve a tile blurb: curated i18n summary first, then the schema description. */
function descFor(id: string, sub?: JsonSchema): string | undefined {
  if (SECTION_DESC_IDS.has(id)) {
    return t(`settings.sectionDesc.${id}`);
  }
  return tSchemaText(sub?.description) || undefined;
}

/**
 * deriveSettingsSections turns the root config JSON Schema into ordered tab
 * descriptors. Top-level schema properties map 1:1 to tabs (using the schema's
 * `x-foxxycode-property-order` and each property's `title`), except that the rarely
 * edited tail keys are folded into a single "System" tab and two synthetic tabs
 * lead the list: "General" (the UI language picker, the default tab) and
 * "Appearance" (the client-side theme picker). Both are present even when no
 * schema is available.
 */
export function deriveSettingsSections(
  schema: JsonSchema | null | undefined,
): SectionDescriptor[] {
  const general: SectionDescriptor = {
    id: "general",
    label: t("settings.section.general"),
    description: descFor("general"),
    kind: "general",
  };
  const appearance: SectionDescriptor = {
    id: "appearance",
    label: t("settings.section.appearance"),
    description: descFor("appearance"),
    kind: "appearance",
  };

  if (!schema || schema.type !== "object" || !schema.properties) {
    return [general, appearance];
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
          label: t("settings.section.system"),
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
        label: tSchemaText(sub.title) || key,
        description: descFor(key, sub),
        kind: "skills",
        schemaKey: key,
      });
      return;
    }
    if (key in ARRAY_LABEL_FIELDS) {
      out.push({
        id: key,
        label: tSchemaText(sub.title) || key,
        description: descFor(key, sub),
        kind: "array",
        schemaKey: key,
        labelField: ARRAY_LABEL_FIELDS[key],
      });
      return;
    }
    out.push({
      id: key,
      label: tSchemaText(sub.title) || key,
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

  return [general, appearance, ...out];
}
