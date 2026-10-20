import { useState } from "react";
import { afterEach, expect, test, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
  waitFor,
} from "@testing-library/react";
import { ToolCallMessage } from "./ToolCallMessage";
import { setLocale } from "../i18n/i18n";

afterEach(() => {
  cleanup();
  setLocale("en");
});

function show(
  name: string,
  args: object,
  result: string,
  status = "completed",
) {
  const view = render(
    <ToolCallMessage
      toolCallId="workspace"
      title={name}
      argsText={JSON.stringify(args)}
      resultText={result}
      status={status}
    />,
  );
  fireEvent.click(screen.getByLabelText("Tool summary"));
  return view.container;
}

test("workspace cards show documentation, checklist operations, file results and SSH destinations", () => {
  let root = show(
    "docs_write",
    {
      path: "docs/guide.md",
      content: "# Guide\n\nRead this.",
      overwrite: true,
    },
    "wrote 24 bytes to /repo/docs/guide.md",
  );
  expect(
    within(root).getByRole("heading", { name: "Guide" }),
  ).toBeInTheDocument();
  expect(root.textContent).toContain("Overwrite");
  cleanup();
  root = show(
    "docs_edit",
    { path: "docs/guide.md", oldString: "Before", newString: "After" },
    "edited /repo/docs/guide.md (5 bytes written)",
  );
  expect(within(root).getByLabelText("Edit preview")).toBeInTheDocument();
  expect(root.textContent).not.toContain('"oldString"');
  cleanup();
  for (const [name, args, result, expected] of [
    [
      "foxxycode_todo_plan_read",
      {},
      "- [x] Inspect\n- [ ] Build `in_progress`",
      "Build",
    ],
    [
      "foxxycode_todo_plan_archive",
      {},
      "archived plan (2 items) to todos/archive/plan_1.md; active plan cleared",
      "todos/archive/plan_1.md",
    ],
    [
      "foxxycode_todo_item_add",
      { content: "Ship", status: "pending" },
      "added plan item (3 rows total)",
      "Ship",
    ],
    [
      "foxxycode_todo_item_remove",
      { index: 1 },
      "removed item at index 1 (2 items remaining)",
      "Removed",
    ],
    [
      "foxxycode_todo_item_move",
      { from_index: 0, to_index: 2 },
      "moved item from index 0 to index 2",
      "1 → 3",
    ],
    [
      "grep",
      { pattern: "hello" },
      "C:\\repo\\a.go:12:hello: world",
      "C:\\repo\\a.go:12",
    ],
    ["glob", { pattern: "**/*.go" }, "/repo/a.go\n/repo/b.go", "/repo/b.go"],
    [
      "print_tree",
      { path: "/repo", depth: 2 },
      "/repo\n├── a.go\n└── src/\n    └── b.go",
      "b.go",
    ],
    ["read", { path: "a.go", offset: 12 }, "hello\nworld", "hello"],
    [
      "mkdir",
      { path: "src" },
      "created directory /repo/src",
      "Created directory",
    ],
    [
      "touch",
      { path: "a.go" },
      "updated timestamps for /repo/a.go",
      "Updated timestamps",
    ],
    [
      "rm",
      { path: "out", recursive: true },
      "removed tree /repo/out",
      "Removed tree",
    ],
    [
      "rmdir",
      { path: "empty" },
      "removed directory /repo/empty",
      "Removed directory",
    ],
    ["mv", { src: "old", dst: "new" }, "moved /repo/old -> /repo/new", "Moved"],
  ] as Array<[string, object, string, string]>) {
    root = show(name, args, result);
    const card = within(root).getByTestId("structured-tool-card");
    expect(card.textContent, name).toContain(expected);
    expect(root.querySelector(".tool-call-result-card"), name).toBeNull();
    cleanup();
  }
  root = show(
    "ssh_run_command",
    { host: "ops@server", port: 2222, command: "pwd" },
    "/srv",
  );
  expect(root.textContent).toContain("ops@server");
  expect(root.textContent).toContain("2222");
});

test("read preserves source text, line offsets and trailing whitespace", () => {
  const root = show(
    "read",
    { path: "notes.md", offset: 8 },
    "# Source  \n\tvalue\n",
  );
  expect(root.querySelector(".workspace-file-source")?.textContent).toBe(
    "# Source  \n\tvalue\n",
  );
  expect(within(root).queryByRole("heading", { name: "Source" })).toBeNull();
  expect(root.querySelector('[data-line="8"]')).toBeInTheDocument();
});

test("todo read preserves the active and failed statuses written by Go", () => {
  show(
    "foxxycode_todo_plan_read",
    {},
    "- [ ] Build `in_progress`\n- [ ] Test `failed`",
  );
  expect(screen.getByLabelText("In progress: Build")).toBeInTheDocument();
  expect(screen.getByLabelText("Failed: Test")).toBeInTheDocument();
});

test.each(["failed", "cancelled"])(
  "%s calls preserve the actual error",
  (status) => {
    const root = show("rm", { path: "src" }, "permission denied", status);
    expect(root.textContent).toContain("permission denied");
    expect(
      root.querySelector('[data-testid="structured-tool-card"]'),
    ).toBeNull();
  },
);

test("unknown output and truncated search lines remain visible", () => {
  const root = show(
    "grep",
    { pattern: "hello" },
    "a.go:2:hello\npartial unknown line\n...",
  );
  expect(root.textContent).toContain("partial unknown line");
  expect(root.textContent).toContain("...");
});

test("pending mutations do not claim success and empty reads stay empty", () => {
  let root = show("rm", { path: "src" }, "", "in_progress");
  expect(root.textContent).not.toContain("Removed");
  cleanup();
  root = show("read", { path: "empty.txt" }, "");
  expect(root.textContent).toContain("Empty result");
});

test("directory listings use rows only when the arguments identify a directory", () => {
  const root = show("read", { path: "src/" }, "a.go\nsub/");
  expect(within(root).getByRole("list", { name: "Files" })).toBeInTheDocument();
});

test.each(["docs_write", "docs_edit", "foxxycode_todo_item_add"])(
  "%s recovers full arguments after reopening history",
  async (name) => {
    const fetched = vi.fn();
    function History() {
      const [args, setArgs] = useState('{"content":"truncated');
      return (
        <ToolCallMessage
          toolCallId="history"
          title={name}
          status="completed"
          argsText={args}
          resultText="done"
          onFetchToolCallFull={async () => {
            fetched();
            setArgs(
              JSON.stringify({
                path: "docs/a.md",
                content: "# Restored",
                oldString: "Before",
                newString: "After",
              }),
            );
          }}
        />
      );
    }
    render(<History />);
    fireEvent.click(screen.getByLabelText("Tool summary"));
    await waitFor(() => expect(fetched).toHaveBeenCalledTimes(1));
    expect(document.body.textContent).not.toContain('"truncated');
  },
);

test("expanded read results retain the file viewer and reset the viewport on collapse", async () => {
  function History() {
    const [full, setFull] = useState("");
    return (
      <ToolCallMessage
        toolCallId="read"
        title="read"
        status="completed"
        argsText='{"path":"a.go","offset":100}'
        resultText="preview\n..."
        resultWasTruncated
        fullResultText={full}
        onFetchToolCallFull={async () => setFull("full\n".repeat(40))}
      />
    );
  }
  render(<History />);
  fireEvent.click(screen.getByLabelText("Tool summary"));
  fireEvent.click(screen.getByTestId("tool-result-more"));
  await waitFor(() =>
    expect(screen.getByTestId("tool-result-less")).toBeInTheDocument(),
  );
  const viewport = screen.getByTestId("tool-result-viewport");
  expect(viewport).toHaveClass("tool-result-viewport--scroll");
  expect(viewport.querySelector('[data-line="100"]')).toBeInTheDocument();
  viewport.scrollTop = 120;
  fireEvent.click(screen.getByTestId("tool-result-less"));
  expect(viewport.scrollTop).toBe(0);
  expect(viewport.textContent).toContain("preview");
});

test("Russian cards translate outcomes and empty states", () => {
  setLocale("ru");
  render(
    <ToolCallMessage
      toolCallId="ru"
      title="mkdir"
      status="completed"
      argsText='{"path":"src"}'
      resultText="created directory /repo/src"
    />,
  );
  fireEvent.click(screen.getByLabelText("Сводка инструмента"));
  expect(screen.getByText("Каталог создан")).toBeInTheDocument();
});
