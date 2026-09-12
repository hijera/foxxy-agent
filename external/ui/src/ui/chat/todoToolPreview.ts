export type TodoPlanEntry = {
  content: string;
  status: string;
};

// Counts only: the caller localizes the header and meta line (the fork routes
// every visible string through t()/tp(), so this module stays pure).
export type TodoToolPreview =
  | {
      variant: "item";
      /** 1-based position of the updated row. */
      position: number;
      /** Plan length; 0 when the row was rebuilt from the call arguments alone. */
      total: number;
      /** A row with empty content was rebuilt from the arguments: the caller labels it. */
      entries: TodoPlanEntry[];
    }
  | {
      variant: "plan";
      completed: number;
      total: number;
      entries: TodoPlanEntry[];
    };

type TodoToolPreviewInput = {
  toolName: string;
  argsText?: string | undefined;
  planSnapshot?: readonly TodoPlanEntry[] | undefined;
};

const todoStatuses = new Set([
  "pending",
  "in_progress",
  "completed",
  "failed",
  "cancelled",
]);

function objectArgs(text: string | undefined): Record<string, unknown> | null {
  if (!text?.trim()) return null;
  try {
    const value = JSON.parse(text) as unknown;
    return value && typeof value === "object" && !Array.isArray(value)
      ? (value as Record<string, unknown>)
      : null;
  } catch {
    return null;
  }
}

function itemIndex(args: Record<string, unknown> | null): number | null {
  const index = args?.index;
  return typeof index === "number" && Number.isInteger(index) && index >= 0
    ? index
    : null;
}

/** Validates untyped HTTP/SSE data before it enters the transcript state. */
export function normalizeTodoPlanSnapshot(
  value: unknown,
): TodoPlanEntry[] | undefined {
  if (!Array.isArray(value)) return undefined;
  const entries: TodoPlanEntry[] = [];
  for (const item of value) {
    if (!item || typeof item !== "object" || Array.isArray(item)) {
      return undefined;
    }
    const row = item as Record<string, unknown>;
    const content = typeof row.content === "string" ? row.content.trim() : "";
    const status = typeof row.status === "string" ? row.status.trim() : "";
    if (!content || !todoStatuses.has(status)) return undefined;
    entries.push({ content, status });
  }
  return entries;
}

/**
 * Parses the markdown checklist foxxycode_todo_plan_replace takes as its argument,
 * mirroring internal/tools/todo/markdown.go: a `- ` / `* ` list line is an entry,
 * `[x]` marks it completed, anything else on the line is pending; other lines
 * are ignored.
 */
export function parsePlanMarkdown(markdown: string): TodoPlanEntry[] {
  const entries: TodoPlanEntry[] = [];
  const normalized = markdown.replace(/\\n/g, "\n");
  for (const raw of normalized.split("\n")) {
    const line = raw.trim();
    if (!line) continue;
    const prefix = ["- ", "* "].find((p) => line.startsWith(p));
    if (prefix === undefined) continue;
    let rest = line.slice(prefix.length).trim();
    let status = "pending";
    if (/^\[[xX]\]/.test(rest)) {
      status = "completed";
      rest = rest.slice(3).trim();
    } else if (rest.startsWith("[ ]")) {
      rest = rest.slice(3).trim();
    }
    if (!rest) continue;
    entries.push({ content: rest, status });
  }
  return entries;
}

export function isTodoPreviewTool(toolName: string): boolean {
  const name = toolName.trim().toLowerCase();
  return (
    name === "foxxycode_todo_item_update" ||
    name === "foxxycode_todo_plan_replace"
  );
}

/**
 * Builds a stable timeline preview for a todo tool call. The plan snapshot saved
 * with the call is the source of truth; when a reloaded transcript carries none
 * (older session, branch, snapshot lost) the call arguments still describe the
 * change, so the card keeps its todo shape instead of falling back to raw JSON.
 */
export function buildTodoToolPreview(
  input: TodoToolPreviewInput,
): TodoToolPreview | null {
  const toolName = input.toolName.trim().toLowerCase();
  const entries = normalizeTodoPlanSnapshot(input.planSnapshot);
  const args = objectArgs(input.argsText);

  if (toolName === "foxxycode_todo_item_update") {
    const index = itemIndex(args);
    if (index === null) return null;
    if (entries && entries.length > 0) {
      if (index >= entries.length) return null;
      return {
        variant: "item",
        position: index + 1,
        total: entries.length,
        entries: [entries[index]!],
      };
    }
    const content =
      typeof args?.content === "string" ? args.content.trim() : "";
    const status =
      typeof args?.status === "string" && todoStatuses.has(args.status.trim())
        ? args.status.trim()
        : "pending";
    return {
      variant: "item",
      position: index + 1,
      total: 0,
      entries: [{ content, status }],
    };
  }

  if (toolName === "foxxycode_todo_plan_replace") {
    const rows =
      entries && entries.length > 0
        ? entries
        : typeof args?.markdown === "string"
          ? parsePlanMarkdown(args.markdown)
          : [];
    if (rows.length === 0) return null;
    const completed = rows.filter(
      (entry) => entry.status === "completed",
    ).length;
    return {
      variant: "plan",
      completed,
      total: rows.length,
      entries: rows,
    };
  }

  return null;
}
