export const svnOperations = [
  "info",
  "status",
  "diff",
  "log",
  "list",
  "add",
  "revert",
  "resolve",
  "update",
  "commit",
  "switch",
  "merge",
  "checkout",
] as const;

export function svnOperation(name: string): string | null {
  const operation = name.trim().toLowerCase().replace(/^svn_/, "");
  return name.trim().toLowerCase().startsWith("svn_") &&
    svnOperations.some((value) => value === operation)
    ? operation
    : null;
}

export function svnArgs(text?: string): Record<string, unknown> | null {
  if (!text?.trim()) return {};
  try {
    const value: unknown = JSON.parse(text);
    return value && typeof value === "object" && !Array.isArray(value)
      ? (value as Record<string, unknown>)
      : null;
  } catch {
    return null;
  }
}

export function svnFailed(status: string, result: string): boolean {
  return status === "failed" || /^error:/i.test(result.trimStart());
}

// SVN status has seven fixed columns. Preserve them, including property and tree conflicts.
export function svnStatusLine(line: string) {
  if (!/^[ MADRC?!~XI][ MC][ L][ +][ S][ KOTB][ C] .+$/.test(line)) return null;
  const columns = line.slice(0, 7);
  const code = columns.includes("C")
    ? "C"
    : columns[0]!.trim() || columns[1]!.trim() || columns.trim();
  return {
    columns,
    code,
    path: line.slice(8),
    conflict: columns.includes("C"),
  };
}

export function svnDiffTone(line: string): string {
  if (/^(Index:|={3,}|@@|--- |\+\+\+ |Property changes on:)/.test(line))
    return "header";
  if (line.startsWith("+")) return "add";
  if (line.startsWith("-")) return "del";
  return "context";
}
