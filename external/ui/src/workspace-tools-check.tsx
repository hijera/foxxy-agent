// Live production components used for card screenshots and responsive checks.
import { createRoot } from "react-dom/client";
import { ToolCallMessage } from "./ui/messages/ToolCallMessage";
import { setLocale } from "./ui/i18n/i18n";
import "./styles.css";

const query = new URLSearchParams(location.search);
document.documentElement.dataset.theme = query.get("theme") || "dark";
setLocale(query.get("lang") === "ru" ? "ru" : "en");
const groups: Record<string, Array<[string, object, string]>> = {
  docs: [
    [
      "docs_write",
      {
        path: "docs/features/deployment.md",
        overwrite: true,
        content:
          "# Deployment\n\nBuild the application, then publish the release.\n\n- Run the checks\n- Review the package\n- Deploy the verified build",
      },
      "wrote 138 bytes to /project/docs/features/deployment.md",
    ],
    [
      "docs_edit",
      {
        path: "docs/features/deployment.md",
        oldString: "Run the build manually.",
        newString: "Run `make build` and verify the release.",
      },
      "edited /project/docs/features/deployment.md (152 bytes written)",
    ],
    [
      "ssh_run_command",
      {
        host: "deploy@staging.example.net",
        port: 2222,
        command: "systemctl --user status foxxycode",
        timeout_seconds: 45,
      },
      "● foxxycode.service — FoxxyCode\n   Active: active (running)",
    ],
  ],
  todo: [
    [
      "foxxycode_todo_plan_read",
      {},
      "- [x] Inspect the workspace\n- [ ] Build the release `in_progress`\n- [ ] Publish the package",
    ],
    [
      "foxxycode_todo_item_add",
      {
        content: "Verify the release on Windows",
        status: "pending",
        after_index: 1,
      },
      "added plan item (4 rows total)",
    ],
    [
      "foxxycode_todo_item_move",
      { from_index: 3, to_index: 1 },
      "moved item from index 3 to index 1",
    ],
    [
      "foxxycode_todo_item_remove",
      { index: 2 },
      "removed item at index 2 (3 items remaining)",
    ],
    [
      "foxxycode_todo_plan_archive",
      {},
      "archived plan (3 items) to todos/archive/plan_1792440000.md; active plan cleared",
    ],
  ],
  files: [
    [
      "grep",
      { path: "internal", pattern: "NewRegistry", glob: "*.go" },
      "internal/tools/export.go:26:func NewRegistry() *Registry {\ninternal/tools/export.go:27:    return NewRegistryFor(nil)",
    ],
    [
      "glob",
      { path: "internal", pattern: "**/config*.go" },
      "internal/config/config.go\ninternal/tools/config_get.go\ninternal/tools/config_set.go",
    ],
    [
      "print_tree",
      { path: "src", depth: 2 },
      "src\n├── app.ts\n└── ui/\n    ├── Card.tsx\n    └── Card.test.tsx",
    ],
    [
      "read",
      { path: "internal/tools/export.go", offset: 26, limit: 3 },
      "func NewRegistry() *Registry {\n    return NewRegistryFor(nil)\n}",
    ],
    ["read", { path: "docs/" }, "features/\nreference/\nREADME.md"],
  ],
  mutations: [
    [
      "mkdir",
      { path: "output/release", parents: true },
      "created directory /project/output/release",
    ],
    [
      "touch",
      { path: "output/release/.ready" },
      "created file /project/output/release/.ready",
    ],
    [
      "mv",
      { src: "output/draft.zip", dst: "output/release/package.zip" },
      "moved /project/output/draft.zip -> /project/output/release/package.zip",
    ],
    [
      "rm",
      { path: "output/cache", recursive: true },
      "removed tree /project/output/cache",
    ],
    ["rmdir", { path: "output/old" }, "removed directory /project/output/old"],
  ],
};
const calls = groups[query.get("group") || "docs"] || groups.docs!;
createRoot(document.getElementById("root")!).render(
  <main
    style={{ maxWidth: 900, margin: "24px auto", padding: 16, minWidth: 0 }}
  >
    <div className="messages-inner" style={{ width: "100%", padding: 0 }}>
      {calls.map(([title, args, resultText], i) => (
        <ToolCallMessage
          key={i}
          toolCallId={`workspace-${i}`}
          title={title}
          argsText={JSON.stringify(
            query.has("long") && title === "docs_write"
              ? {
                  ...args,
                  content:
                    "# Long document\n\n" +
                    "A paragraph that remains readable when the preview expands.\n\n".repeat(
                      60,
                    ),
                }
              : query.has("long") && title === "docs_edit"
                ? {
                    ...args,
                    oldString: "Old line\n".repeat(60),
                    newString: "New line\n".repeat(60),
                  }
                : args,
          )}
          status="completed"
          resultText={resultText}
          durationMs={45}
        />
      ))}
    </div>
  </main>,
);
