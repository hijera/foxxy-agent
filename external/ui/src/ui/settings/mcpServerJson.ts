// Pure logic for the MCP settings section: Cursor-style mcp.json entry
// parsing/validation and row -> editor-text conversion. Kept out of
// MCPSection.tsx so it stays unit-testable.

export type MCPEntryJson = {
  type?: string;
  command?: string;
  args?: string[];
  env?: Record<string, string>;
  url?: string;
  headers?: Record<string, string>;
  disabled?: boolean;
  disabledTools?: string[];
};

export type MCPToolRow = {
  name: string;
  description?: string;
  enabled: boolean;
};

export type MCPScope = "global" | "local";

export type MCPValidationKey =
  | "settings.mcp.validation.nameRequired"
  | "settings.mcp.validation.nameNamespace"
  | "settings.mcp.validation.namePath"
  | "settings.mcp.validation.invalidJson"
  | "settings.mcp.validation.objectRequired"
  | "settings.mcp.validation.typeString"
  | "settings.mcp.validation.commandString"
  | "settings.mcp.validation.urlString"
  | "settings.mcp.validation.commandOrUrl"
  | "settings.mcp.validation.argsStringArray"
  | "settings.mcp.validation.envStringMap"
  | "settings.mcp.validation.headersStringMap"
  | "settings.mcp.validation.disabledBoolean"
  | "settings.mcp.validation.disabledToolsStringArray";

export type MCPServerRow = {
  name: string;
  /** Scope: global (config.yaml or ~/.foxxycode/mcp.json) or local (./.foxxycode/mcp.json). */
  source: MCPScope;
  /** Owning file: config (config.yaml), home (~/.foxxycode/mcp.json), project (./.foxxycode/mcp.json). */
  origin: "config" | "home" | "project";
  /** True for config.yaml entries: toggle-only here, edited in the config sections. */
  readonly?: boolean;
  transport: string;
  command?: string;
  args?: string[];
  url?: string;
  env?: Record<string, string>;
  headers?: Record<string, string>;
  enabled: boolean;
  status: "connected" | "error" | "disabled" | "unsupported";
  error?: string;
  tools: MCPToolRow[];
  disabled_tools?: string[];
};

/** Human name of the file that owns a row's definition (badge tooltips). */
export function originLabel(origin: MCPServerRow["origin"]): string {
  switch (origin) {
    case "config":
      return "config.yaml";
    case "home":
      return "~/.foxxycode/mcp.json";
    default:
      return "./.foxxycode/mcp.json";
  }
}

// Prefill for the "Add server" editor, mirroring Cursor's snippet.
export const MCP_SERVER_TEMPLATE = `{
  "command": "npx",
  "args": ["-y", "@modelcontextprotocol/server-example"],
  "env": {}
}`;

/** validateMCPServerName mirrors the backend rules ("__" namespaces tools). */
export function validateMCPServerName(name: string): MCPValidationKey | null {
  if (!name.trim()) return "settings.mcp.validation.nameRequired";
  if (name.includes("__")) return "settings.mcp.validation.nameNamespace";
  if (/[\s/\\]/.test(name)) {
    return "settings.mcp.validation.namePath";
  }
  return null;
}

function isStringArray(v: unknown): v is string[] {
  return Array.isArray(v) && v.every((x) => typeof x === "string");
}

function isStringMap(v: unknown): v is Record<string, string> {
  return (
    typeof v === "object" &&
    v !== null &&
    !Array.isArray(v) &&
    Object.values(v).every((x) => typeof x === "string")
  );
}

/**
 * parseServerEntryJson validates the JSON editor text as one mcp.json entry
 * (Cursor format: env/headers objects, per-tool switches in disabledTools).
 */
export function parseServerEntryJson(text: string): {
  entry?: MCPEntryJson;
  error?: MCPValidationKey;
} {
  let raw: unknown;
  try {
    raw = JSON.parse(text);
  } catch {
    return { error: "settings.mcp.validation.invalidJson" };
  }
  if (typeof raw !== "object" || raw === null || Array.isArray(raw)) {
    return { error: "settings.mcp.validation.objectRequired" };
  }
  const obj = raw as Record<string, unknown>;
  const entry: MCPEntryJson = {};
  if (obj.type !== undefined) {
    if (typeof obj.type !== "string") {
      return { error: "settings.mcp.validation.typeString" };
    }
    entry.type = obj.type;
  }
  if (obj.command !== undefined) {
    if (typeof obj.command !== "string") {
      return { error: "settings.mcp.validation.commandString" };
    }
    entry.command = obj.command;
  }
  if (obj.url !== undefined) {
    if (typeof obj.url !== "string") {
      return { error: "settings.mcp.validation.urlString" };
    }
    entry.url = obj.url;
  }
  if (!entry.command?.trim() && !entry.url?.trim()) {
    return { error: "settings.mcp.validation.commandOrUrl" };
  }
  if (obj.args !== undefined) {
    if (!isStringArray(obj.args)) {
      return { error: "settings.mcp.validation.argsStringArray" };
    }
    entry.args = obj.args;
  }
  if (obj.env !== undefined) {
    if (!isStringMap(obj.env)) {
      return { error: "settings.mcp.validation.envStringMap" };
    }
    entry.env = obj.env;
  }
  if (obj.headers !== undefined) {
    if (!isStringMap(obj.headers)) {
      return { error: "settings.mcp.validation.headersStringMap" };
    }
    entry.headers = obj.headers;
  }
  if (obj.disabled !== undefined) {
    if (typeof obj.disabled !== "boolean") {
      return { error: "settings.mcp.validation.disabledBoolean" };
    }
    entry.disabled = obj.disabled;
  }
  if (obj.disabledTools !== undefined) {
    if (!isStringArray(obj.disabledTools)) {
      return { error: "settings.mcp.validation.disabledToolsStringArray" };
    }
    entry.disabledTools = obj.disabledTools;
  }
  return { entry };
}

/** serverRowToEntryJson prefills the editor from a listed server row. */
export function serverRowToEntryJson(row: MCPServerRow): string {
  const entry: MCPEntryJson = {};
  if (row.transport && row.transport !== "stdio") entry.type = row.transport;
  if (row.command) entry.command = row.command;
  if (row.args && row.args.length > 0) entry.args = row.args;
  if (row.env && Object.keys(row.env).length > 0) entry.env = row.env;
  if (row.url) entry.url = row.url;
  if (row.headers && Object.keys(row.headers).length > 0) {
    entry.headers = row.headers;
  }
  if (!row.enabled) entry.disabled = true;
  if (row.disabled_tools && row.disabled_tools.length > 0) {
    entry.disabledTools = row.disabled_tools;
  }
  return JSON.stringify(entry, null, 2);
}
