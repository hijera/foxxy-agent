import { expect, test } from "vitest";
import {
  grepRows,
  readTodoChecklist,
  workspaceOutcome,
} from "./workspaceToolDisplay";

test("grep keeps Windows paths and every colon and space in matching text", () => {
  expect(
    grepRows("C:\\repo\\a.go:12:  url: https://example.test:80\n..."),
  ).toEqual([
    { location: "C:\\repo\\a.go:12", text: "  url: https://example.test:80" },
    { location: "", text: "..." },
  ]);
});

test("unknown checklist syntax cannot silently drop returned lines", () => {
  expect(readTodoChecklist("- [x] done\nunknown note")).toBeNull();
  expect(
    readTodoChecklist("- [ ] Build `in_progress`\n- [ ] Test `cancelled`"),
  ).toEqual([
    { content: "Build", status: "in_progress" },
    { content: "Test", status: "cancelled" },
  ]);
});

test("an archive without persistence does not claim a file was written", () => {
  expect(
    workspaceOutcome(
      "foxxycode_todo_plan_archive",
      "archived plan state (2 items finalized); active plan cleared (no session dir for file archive)",
    ),
  ).toEqual({ action: "finalized", count: "2" });
});

test("unknown output and an unrelated success sentence are not reinterpreted", () => {
  expect(workspaceOutcome("rm", "created directory /tmp/foo")).toBeNull();
  expect(workspaceOutcome("mv", "moved unexpectedly\nwarning")).toBeNull();
  expect(
    workspaceOutcome(
      "foxxycode_todo_item_move",
      "item 0 unchanged (from_index equals to_index)",
    ),
  ).toEqual({ action: "unchanged", position: "1" });
});
