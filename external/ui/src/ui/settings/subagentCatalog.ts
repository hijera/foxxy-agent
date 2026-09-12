/**
 * Pure shapes and decisions for the subagent catalog, kept out of the React
 * component the way `mcpServerJson.ts` is kept out of `MCPSection.tsx`: the
 * rules about what may be approved and what an approval covers are the part
 * worth testing on their own.
 *
 * Helpers here return i18n *keys*, never text — resolving them needs the
 * provider, which only the component has.
 */

/** Trust policy for definitions that travel with a checkout (`subagents.project_trust`). */
export type SubagentProjectTrust = "ask" | "allow" | "deny";

export type SubagentScope = "builtin" | "user" | "project";

/** One row of GET /foxxycode/subagents. Mirrors `subagents.CatalogEntry`. */
export type SubagentCatalogEntry = {
  name: string;
  description: string;
  scope: SubagentScope;
  path?: string;
  digest?: string;
  model?: string;
  mode?: string;
  builtin: boolean;
  hidden: boolean;
  trust: "trusted" | "needs_approval";
  trusted: boolean;
  needs_approval: boolean;
  /** Declared bounds. An absent field means "inherits", not "empty". */
  tools?: string[];
  disallowed_tools?: string[];
  permission_mode?: string;
  timeout_seconds?: number;
  max_turns?: number;
  background?: boolean;
  role_bytes?: number;
};

export type SubagentCatalog = {
  items: SubagentCatalogEntry[];
  /** Canonical workspace the receipts are keyed by, as the server resolved it. */
  workspace: string;
  policy: SubagentProjectTrust;
};

/**
 * The shield is only meaningful for a project-scope file under `ask`: built-ins
 * and user-scope files are the operator's own and are always trusted, `deny`
 * never reads project directories at all, and `allow` leaves no decision to
 * make. Same reasoning as `showsTrustControl` on the MCP side.
 */
export function showsSubagentTrustControl(
  entry: SubagentCatalogEntry,
  policy: SubagentProjectTrust,
): boolean {
  return entry.scope === "project" && !entry.builtin && policy === "ask";
}

/** One row of the "what am I approving" list: a label key with a value or a value key. */
export type SubagentFact = {
  labelKey: string;
  value?: string;
  /** Used instead of `value` when the definition declares nothing for this bound. */
  valueKey?: string;
};

const INHERITS = "settings.subagents.fact.inherits";

/**
 * The declaration a receipt would cover. Every bound the definition leaves out
 * is shown as inherited rather than omitted: "this file restricts nothing" is
 * the fact that matters most when deciding whether to approve it.
 */
export function subagentApprovalFacts(entry: SubagentCatalogEntry): SubagentFact[] {
  const facts: SubagentFact[] = [];
  const set = (labelKey: string, value: string | undefined, emptyKey: string) =>
    facts.push(
      value !== undefined && value !== ""
        ? { labelKey, value }
        : { labelKey, valueKey: emptyKey },
    );

  set("settings.subagents.fact.model", entry.model, "settings.subagents.fact.modelInherits");
  set("settings.subagents.fact.mode", entry.mode, "settings.subagents.fact.modeInherits");
  set(
    "settings.subagents.fact.permissions",
    entry.permission_mode,
    "settings.subagents.fact.permissionsInherits",
  );
  set(
    "settings.subagents.fact.tools",
    entry.tools && entry.tools.length > 0 ? entry.tools.join(", ") : undefined,
    "settings.subagents.fact.toolsAll",
  );
  if (entry.disallowed_tools && entry.disallowed_tools.length > 0) {
    facts.push({
      labelKey: "settings.subagents.fact.denies",
      value: entry.disallowed_tools.join(", "),
    });
  }
  set(
    "settings.subagents.fact.timeout",
    entry.timeout_seconds ? formatSeconds(entry.timeout_seconds) : undefined,
    "settings.subagents.fact.timeoutDefault",
  );
  set(
    "settings.subagents.fact.maxTurns",
    entry.max_turns ? String(entry.max_turns) : undefined,
    INHERITS,
  );
  if (entry.background) {
    facts.push({
      labelKey: "settings.subagents.fact.background",
      valueKey: "settings.subagents.fact.backgroundAlways",
    });
  }
  if (entry.role_bytes) {
    facts.push({
      labelKey: "settings.subagents.fact.role",
      value: formatBytes(entry.role_bytes),
    });
  }
  return facts;
}

/** Short duration for a timeout: seconds under a minute, else minutes. */
export function formatSeconds(seconds: number): string {
  if (seconds < 60) {
    return `${seconds}s`;
  }
  const minutes = Math.round(seconds / 60);
  return `${minutes}m`;
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) {
    return `${bytes} B`;
  }
  return `${Math.round(bytes / 1024)} KiB`;
}

/** i18n key for the scope badge. */
export function scopeBadgeKey(scope: SubagentScope): string {
  return `settings.subagents.scope.${scope}`;
}

/** Shortened digest for display; the full value goes in a title attribute. */
export function shortDigest(digest: string | undefined): string {
  const d = (digest ?? "").trim();
  return d.length > 12 ? d.slice(0, 12) : d;
}

/** Definitions still awaiting a receipt, for the pending-approval hint. */
export function pendingApprovalCount(items: SubagentCatalogEntry[]): number {
  return items.filter((e) => e.needs_approval).length;
}
