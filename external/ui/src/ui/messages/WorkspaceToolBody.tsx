import type { ReactNode } from "react";
import { useT } from "../i18n/I18nProvider";
import { buildToolCallPreview } from "../chat/permissionToolPreview";
import { TodoPreview } from "../chat/PermissionPromptPreview";
import type { TodoPlanEntry } from "../chat/todoToolPreview";
import type { ToolArgs } from "../chat/structuredToolDisplay";
import {
  grepRows,
  readTodoChecklist,
  workspaceOutcome,
} from "../chat/workspaceToolDisplay";
import { readsADirectory } from "./toolDisplayName";
import { CodeBlockCopyButton } from "./CodeBlockCopyButton";

function textArg(args: ToolArgs, key: string): string {
  return typeof args[key] === "string" ? (args[key] as string) : "";
}

export function workspaceToolHeading(
  name: string,
  args: ToolArgs,
  fallback: string,
): string {
  if (name === "mv") return `${textArg(args, "src")} → ${textArg(args, "dst")}`;
  return textArg(args, "path") || fallback;
}

function Raw({ text }: { text: string }) {
  return text ? <pre className="structured-tool-output">{text}</pre> : null;
}

function FileRows({ text }: { text: string }) {
  const { t } = useT();
  return (
    <ul
      className="scheduler-tool-rows workspace-file-list"
      aria-label={t("workspaceTool.files")}
    >
      {text.split(/\r?\n/).map((path, i) => (
        <li key={i}>
          <span aria-hidden="true" className="workspace-file-kind">
            <svg
              width="14"
              height="14"
              viewBox="0 0 16 16"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.2"
            >
              <path
                d={
                  /[\\/]$/.test(path)
                    ? "M2 4h4l2 2h6v7H2V4Z"
                    : "M4 2h5l3 3v9H4V2Zm5 0v4h3"
                }
              />
            </svg>
          </span>
          <code className="structured-tool-mono">{path}</code>
        </li>
      ))}
    </ul>
  );
}

function Source({ text, offset }: { text: string; offset: number }) {
  const lines = text.split("\n");
  return (
    <div className="workspace-file-code">
      <CodeBlockCopyButton textToCopy={text} />
      <pre className="workspace-file-source">
        {lines.map((line, index) => (
          <span
            className="workspace-file-line"
            data-line={offset + index}
            key={index}
          >
            {line}
            {index < lines.length - 1 ? "\n" : ""}
          </span>
        ))}
      </pre>
    </div>
  );
}

function Checklist({ entries }: { entries: TodoPlanEntry[] }) {
  const { t } = useT();
  return (
    <TodoPreview
      preview={{
        kind: "todo",
        variant: "plan",
        entries,
        header: t("todoPreview.header.plan"),
        title: "",
        toolName: "",
        meta: [],
        copyText: "",
      }}
    />
  );
}

export function WorkspaceToolBody(props: {
  name: string;
  args: ToolArgs;
  argsText?: string | undefined;
  result: string;
  status: string;
  todoPlan?: TodoPlanEntry[] | undefined;
}) {
  const { t } = useT();
  const { name, args, result, status } = props;
  const completed = status === "completed";
  const todo = name.startsWith("foxxycode_todo_");
  const outcome = completed ? workspaceOutcome(name, result) : null;
  const preview = buildToolCallPreview({
    title: name,
    argsText: props.argsText,
  });
  const info: Array<[string, ReactNode]> = [];
  const field = (key: string, value: ReactNode) =>
    info.push([t(`workspaceTool.${key}`), value]);
  let answer: ReactNode = null;

  if (status === "pending" || status === "in_progress")
    field("outcome", t("toolAction.running"));

  if (todo) {
    if (name === "foxxycode_todo_item_add") {
      const content = textArg(args, "content");
      if (content)
        answer = (
          <Checklist
            entries={[
              { content, status: textArg(args, "status") || "pending" },
            ]}
          />
        );
      if (typeof args.after_index === "number")
        field("position", args.after_index + 2);
      else field("position", t("workspaceTool.atEnd"));
    }
    if (name === "foxxycode_todo_item_remove" && typeof args.index === "number")
      field("position", args.index + 1);
    if (
      name === "foxxycode_todo_item_move" &&
      typeof args.from_index === "number" &&
      typeof args.to_index === "number"
    ) {
      field("position", `${args.from_index + 1} → ${args.to_index + 1}`);
      // The snapshot is the state after the move, never before a deletion.
      const moved = completed ? props.todoPlan?.[args.to_index] : undefined;
      if (moved) answer = <Checklist entries={[moved]} />;
    }
    if (name === "foxxycode_todo_plan_read" && (result || completed)) {
      const entries = readTodoChecklist(result);
      answer =
        entries === null ? (
          <Raw text={result} />
        ) : entries.length ? (
          <Checklist entries={entries} />
        ) : (
          <span className="scheduler-tool-muted">
            {t("workspaceTool.emptyPlan")}
          </span>
        );
    }
  } else {
    if (name === "grep" || name === "glob")
      field(
        "pattern",
        <code className="structured-tool-mono">
          {textArg(args, "pattern")}
        </code>,
      );
    if (name === "print_tree") {
      const depth =
        typeof args.depth === "number" && args.depth > 0 ? args.depth : 3;
      field("depth", Math.min(depth, 8));
    } else if (preview.meta.length) field("options", preview.meta.join(" · "));
  }

  if (outcome) {
    field("outcome", t(`workspaceTool.${outcome.action}`));
    if (outcome.count !== undefined)
      field(
        name === "foxxycode_todo_item_remove" ? "remaining" : "items",
        outcome.count,
      );
    if (outcome.path)
      field(
        "path",
        <code className="structured-tool-mono">{outcome.path}</code>,
      );
    if (outcome.destination)
      field(
        "destination",
        <code className="structured-tool-mono">{outcome.destination}</code>,
      );
  } else if (todo && name !== "foxxycode_todo_plan_read" && result) {
    answer = (
      <>
        {answer}
        <Raw text={result} />
      </>
    );
  }

  if (!todo && !outcome) {
    if (name === "read") {
      const directory = readsADirectory(props.argsText);
      const offset =
        typeof args.offset === "number" &&
        Number.isInteger(args.offset) &&
        args.offset > 0
          ? args.offset
          : 1;
      answer = result ? (
        directory ? (
          <FileRows text={result} />
        ) : (
          <Source text={result} offset={offset} />
        )
      ) : completed ? (
        <span className="scheduler-tool-muted">
          {t("workspaceTool.emptyResult")}
        </span>
      ) : null;
    } else if (name === "grep") {
      answer =
        result === "no matches found" ? (
          <span className="scheduler-tool-muted">
            {t("workspaceTool.noMatches")}
          </span>
        ) : result ? (
          <ul
            className="scheduler-tool-rows workspace-search-results"
            aria-label={t("workspaceTool.matches")}
          >
            {grepRows(result).map((row, i) => (
              <li key={i}>
                {row.location ? (
                  <code className="structured-tool-mono">{row.location}</code>
                ) : null}
                <Raw text={row.text} />
              </li>
            ))}
          </ul>
        ) : null;
    } else if (name === "glob") {
      answer =
        result === "(no files matched)" ? (
          <span className="scheduler-tool-muted">
            {t("workspaceTool.noMatches")}
          </span>
        ) : result ? (
          <FileRows text={result} />
        ) : null;
    } else if (name === "print_tree") {
      // Keep the tree's connectors, root and truncation note exactly as returned.
      answer = result ? (
        <div className="workspace-file-tree">
          <CodeBlockCopyButton textToCopy={result} />
          <Raw text={result} />
        </div>
      ) : null;
    } else answer = <Raw text={result} />;
  }

  return (
    <>
      {info.length ? (
        <dl className="scheduler-tool-fields">
          {info.map(([label, value], i) => (
            <div className="scheduler-tool-field" key={i}>
              <dt>{label}</dt>
              <dd>{value}</dd>
            </div>
          ))}
        </dl>
      ) : null}
      {answer ? <div className="structured-tool-answer">{answer}</div> : null}
    </>
  );
}
