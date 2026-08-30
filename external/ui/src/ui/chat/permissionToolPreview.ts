import { t, tp } from "../i18n/i18n";
import {
  flattenDiffLines,
  parseDiffPatch,
  type ParsedDiffLine,
} from "../messages/parseDiff";
import { permissionPromptDetail } from "./permissionPromptDisplay";
import type { FoxxyCodePermissionPayload } from "./permissionTypes";
import { permissionBodyText } from "./permissionTypes";

export type PermissionToolCallContext = {
  title?: string | undefined;
  kind?: string | undefined;
  argsText?: string | undefined;
};

type PermissionPreviewBase = {
  toolName: string;
  title: string;
  header: string;
  meta: string[];
  copyText: string;
};

export type PermissionToolPreview =
  | (PermissionPreviewBase & { kind: "code"; text: string })
  | (PermissionPreviewBase & { kind: "path" })
  | (PermissionPreviewBase & {
      kind: "move";
      sourcePath: string;
      destinationPath: string;
    })
  | (PermissionPreviewBase & {
      kind: "diff";
      lines: ParsedDiffLine[];
      hunkHeaders: Array<{ at: number; text: string }>;
    });

function normalizedToolName(value: string | undefined): string {
  return (value || "").replace(/^run:\s*/i, "").trim();
}

/** Concrete FoxxyCode tool id, preferring the matching transcript call over its generic ACP kind. */
export function permissionPromptToolName(
  payload: FoxxyCodePermissionPayload,
  context?: PermissionToolCallContext | undefined,
): string {
  return (
    normalizedToolName(context?.title) ||
    normalizedToolName(payload.toolCall.title) ||
    normalizedToolName(context?.kind) ||
    normalizedToolName(payload.toolCall.kind) ||
    "tool"
  );
}

function parseArgsText(text: string): Record<string, unknown> | null {
  const raw = text.trim();
  if (!raw) return null;
  const match = /^Arguments:\s*(\{[\s\S]*\})\s*$/i.exec(raw);
  const candidate = match?.[1] || raw;
  if (!candidate.startsWith("{")) return null;
  try {
    const parsed = JSON.parse(candidate) as unknown;
    return parsed && typeof parsed === "object" && !Array.isArray(parsed)
      ? (parsed as Record<string, unknown>)
      : null;
  } catch {
    return null;
  }
}

function stringArg(args: Record<string, unknown>, ...names: string[]): string {
  for (const name of names) {
    const value = args[name];
    if (typeof value === "string") return value;
  }
  return "";
}

function boolArg(
  args: Record<string, unknown>,
  name: string,
  fallback: boolean,
): boolean {
  return typeof args[name] === "boolean" ? args[name] : fallback;
}

function numberArg(
  args: Record<string, unknown>,
  name: string,
  fallback: number,
): number {
  const value = args[name];
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

/**
 * Short "what is this tool acting on" string: file path, command, or search pattern.
 * Never localized — the caller renders it verbatim next to a localized verb.
 *
 * Deliberately not derived from buildToolCallPreview: that one localizes `header` for
 * shell/move tools, puts the whole file body in `text` for write, and re-parses diffs on
 * every call. The live status label recomputes on every transcript change, so it needs a
 * cheap accessor.
 *
 * Returns "" when the arguments have not arrived yet (a `pending` tool call legitimately
 * has none until its `tool_call_update` frame).
 */
export function toolCallTargetText(context: PermissionToolCallContext): string {
  const toolName = (
    normalizedToolName(context.title) ||
    normalizedToolName(context.kind) ||
    ""
  ).toLowerCase();
  const args = parseArgsText(context.argsText || "");
  if (!args) {
    return "";
  }
  switch (toolName) {
    case "run_command":
    case "ssh_run_command":
      return stringArg(args, "command");
    case "grep":
    case "glob":
      return stringArg(args, "pattern");
    case "websearch":
      return stringArg(args, "query");
    case "mv":
      return stringArg(args, "src");
    case "question":
      return "";
    default:
      // read / write / edit / apply_patch / mkdir / touch / rm / rmdir / list_dir /
      // print_tree / docs_* / plan_* take a path; browser and webfetch take a url.
      return stringArg(args, "path", "filePath", "file_path", "url", "name");
  }
}

function questionForTool(
  toolName: string,
  args: Record<string, unknown>,
): string {
  switch (toolName.toLowerCase()) {
    case "run_command":
    case "ssh_run_command":
      return t("prompts.permissionQuestion.runCommand");
    case "write":
    case "write_file":
      return t("prompts.permissionQuestion.write");
    case "edit":
      return t("prompts.permissionQuestion.edit");
    case "apply_patch":
      return t("prompts.permissionQuestion.applyPatch");
    case "mkdir":
      return t("prompts.permissionQuestion.mkdir");
    case "touch":
      return t("prompts.permissionQuestion.touch");
    case "mv":
      return t("prompts.permissionQuestion.mv");
    case "rm":
      return boolArg(args, "recursive", false)
        ? t("prompts.permissionQuestion.rmRecursive")
        : t("prompts.permissionQuestion.rm");
    case "rmdir":
      return t("prompts.permissionQuestion.rmdir");
    default:
      return t("prompts.permissionQuestion.fallback");
  }
}

function editDiffLines(oldString: string, newString: string): ParsedDiffLine[] {
  const oldLines = oldString === "" ? [] : oldString.split(/\r?\n/);
  const newLines = newString === "" ? [] : newString.split(/\r?\n/);
  let prefix = 0;
  while (
    prefix < oldLines.length &&
    prefix < newLines.length &&
    oldLines[prefix] === newLines[prefix]
  ) {
    prefix++;
  }
  let suffix = 0;
  while (
    suffix < oldLines.length - prefix &&
    suffix < newLines.length - prefix &&
    oldLines[oldLines.length - 1 - suffix] ===
      newLines[newLines.length - 1 - suffix]
  ) {
    suffix++;
  }

  const contextBefore = Math.min(prefix, 2);
  const contextAfter = Math.min(suffix, 2);
  const rows: ParsedDiffLine[] = [];
  for (let i = prefix - contextBefore; i < prefix; i++) {
    rows.push({
      kind: "ctx",
      oldNo: i + 1,
      newNo: i + 1,
      content: oldLines[i] || "",
    });
  }
  for (let i = prefix; i < oldLines.length - suffix; i++) {
    rows.push({
      kind: "del",
      oldNo: i + 1,
      newNo: null,
      content: oldLines[i] || "",
    });
  }
  for (let i = prefix; i < newLines.length - suffix; i++) {
    rows.push({
      kind: "add",
      oldNo: null,
      newNo: i + 1,
      content: newLines[i] || "",
    });
  }
  for (let offset = contextAfter; offset > 0; offset--) {
    const oldIndex = oldLines.length - offset;
    const newIndex = newLines.length - offset;
    rows.push({
      kind: "ctx",
      oldNo: oldIndex + 1,
      newNo: newIndex + 1,
      content: oldLines[oldIndex] || "",
    });
  }
  return rows;
}

function diffMeta(lines: ParsedDiffLine[]): string[] {
  const additions = lines.filter((line) => line.kind === "add").length;
  const deletions = lines.filter((line) => line.kind === "del").length;
  return ["+" + additions, "−" + deletions];
}

/** Tool-specific, render-ready preview shared by permission gates and transcript foldouts. */
export function buildToolCallPreview(
  context: PermissionToolCallContext,
  fallback = "",
): PermissionToolPreview {
  const toolName =
    normalizedToolName(context.title) ||
    normalizedToolName(context.kind) ||
    "tool";
  const normalized = toolName.toLowerCase();
  const args = parseArgsText(context.argsText || "") || {};
  const title = questionForTool(normalized, args);

  if (normalized === "run_command" || normalized === "ssh_run_command") {
    const command = stringArg(args, "command") || fallback;
    const timeout = numberArg(args, "timeout_seconds", 30);
    return {
      toolName,
      title,
      header:
        normalized === "ssh_run_command"
          ? t("prompts.permissionHeader.sshShell")
          : t("prompts.permissionHeader.shell"),
      meta: [t("prompts.permissionMeta.timeout", { seconds: timeout })],
      copyText: command,
      kind: "code",
      text: command,
    };
  }

  if (normalized === "apply_patch") {
    const path = stringArg(args, "path", "filePath");
    const patch = stringArg(args, "patch", "diff");
    const parsed = parseDiffPatch(patch, path);
    const lines = flattenDiffLines(parsed);
    let at = 0;
    const hunkHeaders = parsed.hunks.map((hunk) => {
      const row = { at, text: hunk.header };
      at += hunk.lines.length;
      return row;
    });
    return {
      toolName,
      title,
      header: parsed.filePath || path,
      meta: diffMeta(lines),
      copyText: patch,
      kind: "diff",
      lines,
      hunkHeaders,
    };
  }

  if (normalized === "edit") {
    const path = stringArg(args, "path");
    const oldString = stringArg(args, "oldString");
    const newString = stringArg(args, "newString");
    const lines = editDiffLines(oldString, newString);
    const meta = diffMeta(lines);
    if (boolArg(args, "replaceAll", false)) {
      meta.push(t("prompts.permissionMeta.replaceAll"));
    }
    return {
      toolName,
      title,
      header: path,
      meta,
      copyText: newString,
      kind: "diff",
      lines,
      hunkHeaders: [],
    };
  }

  if (normalized === "write" || normalized === "write_file") {
    const path = stringArg(args, "path", "filePath");
    const content = stringArg(args, "content");
    return {
      toolName,
      title,
      header: path,
      meta: [tp("prompts.permissionMeta.chars", content.length)],
      copyText: content,
      kind: "code",
      text: content,
    };
  }

  if (normalized === "mv") {
    const sourcePath = stringArg(args, "src");
    const destinationPath = stringArg(args, "dst");
    return {
      toolName,
      title,
      header: t("prompts.permissionHeader.move"),
      meta: [],
      copyText: (sourcePath + "\n" + destinationPath).trim(),
      kind: "move",
      sourcePath,
      destinationPath,
    };
  }

  const path = stringArg(args, "path");
  if (normalized === "mkdir") {
    return {
      toolName,
      title,
      header: path,
      meta: [
        boolArg(args, "parents", true)
          ? t("prompts.permissionMeta.createParents")
          : t("prompts.permissionMeta.directParentOnly"),
      ],
      copyText: path,
      kind: "path",
    };
  }
  if (normalized === "touch") {
    return {
      toolName,
      title,
      header: path,
      meta: [
        boolArg(args, "create_parents", true)
          ? t("prompts.permissionMeta.createParents")
          : t("prompts.permissionMeta.existingParentsOnly"),
      ],
      copyText: path,
      kind: "path",
    };
  }
  if (normalized === "rm") {
    const recursive = boolArg(args, "recursive", false);
    return {
      toolName,
      title,
      header: path,
      meta: recursive ? [t("prompts.permissionMeta.recursive")] : [],
      copyText: path,
      kind: "path",
    };
  }
  if (normalized === "rmdir") {
    return {
      toolName,
      title,
      header: path,
      meta: [t("prompts.permissionMeta.emptyDirectoryOnly")],
      copyText: path,
      kind: "path",
    };
  }

  if (normalized === "read" || normalized === "list_dir") {
    const meta: string[] = [];
    const offset = numberArg(args, "offset", 0);
    const limit = numberArg(args, "limit", 0);
    if (offset > 0) meta.push(t("prompts.permissionMeta.fromLine", { line: offset }));
    if (limit > 0) meta.push(tp("prompts.permissionMeta.lines", limit));
    if (boolArg(args, "recursive", false)) {
      meta.push(t("prompts.permissionMeta.recursive"));
    }
    if (boolArg(args, "show_hidden", false)) {
      meta.push(t("prompts.permissionMeta.hiddenFiles"));
    }
    return {
      toolName,
      title,
      header: stringArg(args, "path") || t("prompts.permissionHeader.workspace"),
      meta,
      copyText: stringArg(args, "path"),
      kind: "path",
    };
  }

  if (normalized === "grep" || normalized === "glob") {
    const pattern = stringArg(args, "pattern");
    const meta: string[] = [];
    const glob = stringArg(args, "glob");
    if (glob) meta.push(glob);
    if (boolArg(args, "case_sensitive", false)) {
      meta.push(t("prompts.permissionMeta.caseSensitive"));
    }
    const maxResults = numberArg(args, "max_results", 0);
    if (maxResults > 0) {
      meta.push(t("prompts.permissionMeta.maxResults", { count: maxResults }));
    }
    return {
      toolName,
      title,
      header: stringArg(args, "path") || t("prompts.permissionHeader.workspace"),
      meta,
      copyText: pattern,
      kind: "code",
      text: pattern,
    };
  }

  if (normalized === "print_tree") {
    const depth = numberArg(args, "depth", 0);
    return {
      toolName,
      title,
      header: stringArg(args, "path") || t("prompts.permissionHeader.workspace"),
      meta: depth > 0 ? [t("prompts.permissionMeta.depth", { depth })] : [],
      copyText: stringArg(args, "path"),
      kind: "path",
    };
  }

  const text =
    fallback ||
    (Object.keys(args).length > 0 ? JSON.stringify(args, null, 2) : "");
  return {
    toolName,
    title,
    header: "",
    meta: [],
    copyText: text,
    kind: "code",
    text,
  };
}

/** Permission wrapper that can fall back to the ACP content when transcript args are absent. */
export function buildPermissionToolPreview(
  payload: FoxxyCodePermissionPayload,
  context?: PermissionToolCallContext | undefined,
): PermissionToolPreview {
  const toolName = permissionPromptToolName(payload, context);
  const argsText = context?.argsText || permissionBodyText(payload);
  return buildToolCallPreview(
    { title: toolName, kind: context?.kind, argsText },
    permissionPromptDetail(payload),
  );
}
