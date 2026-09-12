import { expect, test } from "vitest";

import { buildTodoToolPreview, parsePlanMarkdown } from "./todoToolPreview";

const plan = [
  { content: "Inspect the existing tool cards", status: "completed" },
  { content: "Render the structured preview", status: "in_progress" },
  { content: "Add interaction tests", status: "pending" },
];

test("item update reads its final row from the persisted plan snapshot", () => {
  expect(
    buildTodoToolPreview({
      toolName: "foxxycode_todo_item_update",
      argsText: JSON.stringify({ index: 1, status: "in_progress" }),
      planSnapshot: plan,
    }),
  ).toEqual({
    variant: "item",
    position: 2,
    total: 3,
    entries: [plan[1]],
  });
});

test("plan replacement preserves item order and reports completed count", () => {
  expect(
    buildTodoToolPreview({
      toolName: "foxxycode_todo_plan_replace",
      argsText: JSON.stringify({ markdown: "- [ ] ignored here" }),
      planSnapshot: plan,
    }),
  ).toEqual({
    variant: "plan",
    completed: 1,
    total: 3,
    entries: plan,
  });
});

test("snapshot index out of range leaves the generic tool preview in place", () => {
  expect(
    buildTodoToolPreview({
      toolName: "foxxycode_todo_item_update",
      argsText: JSON.stringify({ index: 4, status: "completed" }),
      planSnapshot: plan,
    }),
  ).toBeNull();
});

// A reloaded transcript may carry no snapshot at all (branch of an older
// session, a call recorded before snapshots existed). The card still has to
// look like a todo card: the arguments say which item changed and how.
test("item update without a snapshot renders the row from its arguments", () => {
  expect(
    buildTodoToolPreview({
      toolName: "foxxycode_todo_item_update",
      argsText: JSON.stringify({ index: 5, status: "in_progress" }),
    }),
  ).toEqual({
    variant: "item",
    position: 6,
    total: 0,
    entries: [{ content: "", status: "in_progress" }],
  });
  expect(
    buildTodoToolPreview({
      toolName: "foxxycode_todo_item_update",
      argsText: JSON.stringify({ index: 0, content: "Renamed step", status: "completed" }),
      planSnapshot: [],
    }),
  ).toEqual({
    variant: "item",
    position: 1,
    total: 0,
    entries: [{ content: "Renamed step", status: "completed" }],
  });
});

test("item update without a snapshot defaults an unknown status to pending", () => {
  expect(
    buildTodoToolPreview({
      toolName: "foxxycode_todo_item_update",
      argsText: JSON.stringify({ index: 2, status: "bogus" }),
    }),
  ).toEqual({
    variant: "item",
    position: 3,
    total: 0,
    entries: [{ content: "", status: "pending" }],
  });
});

test("item update without a snapshot and without a usable index stays generic", () => {
  expect(
    buildTodoToolPreview({
      toolName: "foxxycode_todo_item_update",
      argsText: JSON.stringify({ status: "completed" }),
    }),
  ).toBeNull();
  expect(
    buildTodoToolPreview({
      toolName: "foxxycode_todo_item_update",
      argsText: JSON.stringify({ index: -1 }),
    }),
  ).toBeNull();
  expect(
    buildTodoToolPreview({
      toolName: "foxxycode_todo_item_update",
      argsText: "not json",
    }),
  ).toBeNull();
});

test("plan replacement without a snapshot renders the list from its markdown", () => {
  expect(
    buildTodoToolPreview({
      toolName: "foxxycode_todo_plan_replace",
      argsText: JSON.stringify({
        markdown: "# Plan\n- [x] Inspect cards\n- [ ] Render preview\n\nnotes\n* [X] Add tests\n",
      }),
    }),
  ).toEqual({
    variant: "plan",
    completed: 2,
    total: 3,
    entries: [
      { content: "Inspect cards", status: "completed" },
      { content: "Render preview", status: "pending" },
      { content: "Add tests", status: "completed" },
    ],
  });
  expect(
    buildTodoToolPreview({
      toolName: "foxxycode_todo_plan_replace",
      argsText: "{}",
      planSnapshot: [],
    }),
  ).toBeNull();
});

test("snapshot wins over the arguments when both are present", () => {
  expect(
    buildTodoToolPreview({
      toolName: "foxxycode_todo_item_update",
      argsText: JSON.stringify({ index: 1, content: "argument text", status: "pending" }),
      planSnapshot: plan,
    }),
  ).toEqual({
    variant: "item",
    position: 2,
    total: 3,
    entries: [plan[1]],
  });
});

test("parsePlanMarkdown mirrors the backend checkbox rules", () => {
  expect(parsePlanMarkdown("- [ ] a\n- [x] b\n- [X] c\n-[ ] not a list\n- plain\n* [x]\n")).toEqual([
    { content: "a", status: "pending" },
    { content: "b", status: "completed" },
    { content: "c", status: "completed" },
    { content: "plain", status: "pending" },
  ]);
  expect(parsePlanMarkdown("")).toEqual([]);
});
