import { expect, test } from "vitest";
import { initLocale } from "../i18n/i18n";
import { deriveSettingsSections } from "./settingsSections";
import type { JsonSchema } from "./SchemaForm";

// Mirrors the top-level shape + order produced by Go UISchemaMap().
const rootSchema: JsonSchema = {
  type: "object",
  "x-foxxycode-property-order": [
    "providers",
    "models",
    "agent",
    "tools",
    "subagents",
    "mcp_servers",
    "skills",
    "memory",
    "scheduler",
    "prompts",
    "instructions",
    "logger",
    "sessions",
    "gateways",
    "ui",
  ],
  properties: {
    providers: { type: "array", title: "LLM providers", items: { type: "object" } },
    models: { type: "array", title: "Logical models", items: { type: "object" } },
    agent: { type: "object", title: "ReAct agent", properties: {} },
    tools: { type: "object", title: "Tools and permissions", properties: {} },
    subagents: { type: "object", title: "Subagents", properties: {} },
    mcp_servers: { type: "array", title: "MCP servers", items: { type: "object" } },
    skills: { type: "object", title: "Skills", properties: {} },
    memory: { type: "object", title: "Long-term memory", properties: {} },
    scheduler: { type: "object", title: "Scheduler", properties: {} },
    prompts: { type: "object", title: "Prompts", properties: {} },
    instructions: { type: "object", title: "Instructions", properties: {} },
    logger: { type: "object", title: "Logger", properties: {} },
    sessions: { type: "object", title: "Sessions", properties: {} },
    gateways: { type: "object", title: "Messenger gateways", properties: {} },
    ui: { type: "object", title: "UI", properties: {} },
  },
} as unknown as JsonSchema;

test("derives tabs in schema order with General and Appearance first and System group", () => {
  const sections = deriveSettingsSections(rootSchema);
  const ids = sections.map((s) => s.id);
  expect(ids).toEqual([
    "general",
    "appearance",
    "sessions_manager",
    "providers",
    "models",
    "agent",
    "tools",
    "subagents",
    "mcp_servers",
    "skills",
    "memory",
    "system",
  ]);
});

test("the ui schema key is hidden — its locale is edited by the General picker", () => {
  const ids = deriveSettingsSections(rootSchema).map((s) => s.id);
  expect(ids).not.toContain("ui");
});

test("array sections carry their label field", () => {
  const byId = Object.fromEntries(deriveSettingsSections(rootSchema).map((s) => [s.id, s]));
  expect(byId.providers!.kind).toBe("array");
  expect(byId.providers!.labelField).toBe("name");
  expect(byId.models!.kind).toBe("array");
  expect(byId.models!.labelField).toBe("model");
});

test("mcp_servers is its own managed tab", () => {
  const byId = Object.fromEntries(
    deriveSettingsSections(rootSchema).map((s) => [s.id, s]),
  );
  expect(byId.mcp_servers!.kind).toBe("mcp");
});

// Hybrid tab: the generated form still edits the config section, so the tab
// keeps its schema key, while the kind routes it to the panel that also lists
// the definitions and records approvals.
test("subagents is a hybrid tab that keeps its schema key", () => {
  const byId = Object.fromEntries(
    deriveSettingsSections(rootSchema).map((s) => [s.id, s]),
  );
  expect(byId.subagents!.kind).toBe("subagents");
  expect(byId.subagents!.schemaKey).toBe("subagents");
});

test("System group folds the rarely edited tail keys", () => {
  const system = deriveSettingsSections(rootSchema).find((s) => s.id === "system");
  expect(system?.kind).toBe("group");
  expect(system?.childKeys).toEqual([
    "scheduler",
    "prompts",
    "instructions",
    "logger",
    "sessions",
    "gateways",
  ]);
});

test("skills is its own combined tab; labels come from schema titles", () => {
  const byId = Object.fromEntries(deriveSettingsSections(rootSchema).map((s) => [s.id, s]));
  expect(byId.skills!.kind).toBe("skills");
  expect(byId.agent!.kind).toBe("object");
  expect(byId.agent!.label).toBe("ReAct agent");
});

test("General, Appearance and Sessions are present even without a schema", () => {
  // All three are client-side tabs: the language and theme pickers edit no
  // schema-driven form and the session table talks to /foxxycode/sessions, so
  // none of them waits for the schema.
  const sections = deriveSettingsSections(null);
  expect(sections.map((s) => s.id)).toEqual([
    "general",
    "appearance",
    "sessions_manager",
  ]);
});

test("the session management tab is synthetic and edits no config key", () => {
  const byId = Object.fromEntries(
    deriveSettingsSections(rootSchema).map((s) => [s.id, s]),
  );
  // `sessions` in the schema is the storage directory and stays in System; the
  // management tab must not be confused with it.
  expect(byId.sessions).toBeUndefined();
  expect(byId.sessions_manager?.kind).toBe("sessions");
  expect(byId.sessions_manager?.schemaKey).toBeUndefined();
  expect(byId.sessions_manager?.label).toBe("Sessions");
  expect(byId.sessions_manager?.description).toBe("Stored chats & cleanup");
});

test("the session management tab follows the active locale", () => {
  initLocale("ru");
  try {
    const byId = Object.fromEntries(
      deriveSettingsSections(rootSchema).map((s) => [s.id, s]),
    );
    expect(byId.sessions_manager?.label).toBe("Сессии");
    expect(byId.sessions_manager?.description).toBe(
      "Сохранённые чаты и очистка",
    );
  } finally {
    initLocale("en");
  }
});
