import type { TodoPlanEntry } from "./todoToolPreview";

export const workspaceToolNames = new Set([
  "read",
  "grep",
  "glob",
  "print_tree",
  "mkdir",
  "touch",
  "rm",
  "rmdir",
  "mv",
  "foxxycode_todo_plan_read",
  "foxxycode_todo_plan_archive",
  "foxxycode_todo_item_add",
  "foxxycode_todo_item_remove",
  "foxxycode_todo_item_move",
]);

export function grepRows(text: string) {
  return text.split(/\r?\n/).map((line) => {
    // Non-greedy path handles drive letters without consuming colons in the match.
    const match = /^(.*?):([1-9]\d*):(.*)$/.exec(line);
    return match
      ? { location: `${match[1]}:${match[2]}`, text: match[3]! }
      : { location: "", text: line };
  });
}

/** Read the exact checklist syntax written by todo.FormatPlanMarkdown. */
export function readTodoChecklist(text: string): TodoPlanEntry[] | null {
  if (!text.trim()) return [];
  const entries: TodoPlanEntry[] = [];
  for (const line of text.split(/\r?\n/)) {
    if (!line.trim()) continue;
    const match = /^- \[([ xX])\] (.*)$/.exec(line);
    if (!match) return null;
    let content = match[2]!;
    let status = match[1]!.toLowerCase() === "x" ? "completed" : "pending";
    // The wire format is ambiguous by design: a non-pending, non-completed
    // status is only a trailing " `status`" on the content, so a pending item
    // whose text happens to end that way parses with the suffix's status and
    // shortened content - the same reading the model itself gets from
    // plan_read. The next plan snapshot replaces the parsed view.
    const suffix = / `((?:in_progress|failed|cancelled))`$/.exec(content);
    if (suffix && status === "pending") {
      status = suffix[1]!;
      content = content.slice(0, -suffix[0].length);
    }
    entries.push({ content, status });
  }
  return entries;
}

export type WorkspaceOutcome = {
  action: string;
  path?: string;
  destination?: string;
  count?: string;
  position?: string;
};

/** Recognise only the success sentences emitted by these tools; preserve everything else. */
export function workspaceOutcome(
  name: string,
  text: string,
): WorkspaceOutcome | null {
  const patterns: Record<string, Array<[RegExp, string]>> = {
    mkdir: [[/^created directory (.+)$/, "createdDirectory"]],
    touch: [
      [/^created file (.+)$/, "createdFile"],
      [/^updated timestamps for (.+)$/, "timestamps"],
    ],
    rm: [
      [/^removed tree (.+)$/, "removedTree"],
      [/^removed (.+)$/, "removed"],
    ],
    rmdir: [[/^removed directory (.+)$/, "removedDirectory"]],
    mv: [[/^moved (.+) -> (.+)$/, "moved"]],
  };
  for (const [pattern, action] of patterns[name] || []) {
    const match = pattern.exec(text);
    if (match)
      return {
        action,
        path: match[1]!,
        ...(match[2] ? { destination: match[2] } : {}),
      };
  }
  if (name === "foxxycode_todo_plan_archive") {
    const archived =
      /^archived plan \((\d+) items\) to (.+); active plan cleared$/.exec(text);
    if (archived)
      return { action: "archived", count: archived[1]!, path: archived[2]! };
    const memory =
      /^archived plan state \((\d+) items finalized\); active plan cleared \(no session dir for file archive\)$/.exec(
        text,
      );
    if (memory) return { action: "finalized", count: memory[1]! };
    if (text === "plan archive: no active items (nothing to write)")
      return { action: "noActiveItems" };
  }
  if (name === "foxxycode_todo_item_add") {
    const match = /^added plan item \((\d+) rows total\)$/.exec(text);
    if (match) return { action: "added", count: match[1]! };
  }
  if (name === "foxxycode_todo_item_remove") {
    const match =
      /^removed item at index (\d+) \((\d+) items remaining\)$/.exec(text);
    if (match)
      return {
        action: "removed",
        position: String(Number(match[1]) + 1),
        count: match[2]!,
      };
  }
  if (name === "foxxycode_todo_item_move") {
    const moved = /^moved item from index (\d+) to index (\d+)$/.exec(text);
    if (moved)
      return {
        action: "moved",
        position: `${Number(moved[1]) + 1} → ${Number(moved[2]) + 1}`,
      };
    const unchanged =
      /^item (\d+) unchanged \(from_index equals to_index\)$/.exec(text);
    if (unchanged)
      return {
        action: "unchanged",
        position: String(Number(unchanged[1]) + 1),
      };
  }
  return null;
}
