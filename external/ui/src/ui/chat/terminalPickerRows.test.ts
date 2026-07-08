import { expect, test } from "vitest";
import { terminalPickerRows } from "./terminalPickerRows";

const terms = [
  { id: "1", name: "zsh", active: true },
  { id: "2", name: "dev server" }, // has a space → not addressable by name
  { id: "3", name: "node" },
];

test("empty prefix offers bare @terminal plus whitespace-free named rows", () => {
  const rows = terminalPickerRows("", terms);
  expect(rows.map((r) => r.path_rel)).toEqual([
    "terminal",
    "terminal:zsh",
    "terminal:node",
  ]);
  // "dev server" is dropped (contains a space).
  expect(rows.find((r) => r.name === "dev server")).toBeUndefined();
});

test("prefix that leads toward 'terminal' keeps rows", () => {
  expect(terminalPickerRows("ter", terms).length).toBeGreaterThan(0);
  expect(terminalPickerRows("terminal", terms)[0]?.path_rel).toBe("terminal");
});

test("non-terminal prefix returns [] so file search is unaffected", () => {
  expect(terminalPickerRows("src", terms)).toEqual([]);
  expect(terminalPickerRows("app.tsx", terms)).toEqual([]);
});

test("name selector filters named rows and drops the bare row", () => {
  const rows = terminalPickerRows("terminal:no", terms);
  expect(rows.map((r) => r.path_rel)).toEqual(["terminal:node"]);
});

test("name selector with empty name lists all named rows", () => {
  const rows = terminalPickerRows("terminal:", terms);
  expect(rows.map((r) => r.path_rel)).toEqual(["terminal:zsh", "terminal:node"]);
});

test("no terminals reported yields no rows for any prefix", () => {
  expect(terminalPickerRows("", [])).toEqual([]);
  expect(terminalPickerRows("terminal", [])).toEqual([]);
});
