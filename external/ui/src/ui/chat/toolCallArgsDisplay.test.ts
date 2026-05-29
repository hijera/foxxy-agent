import { expect, test } from "vitest";
import { toolCallArgsDisplay } from "./toolCallArgsDisplay";

test("run_command shows command line only", () => {
  expect(
    toolCallArgsDisplay('{"command":"ls -la"}', { kind: "run_command" }),
  ).toBe("ls -la");
});

test("unknown tool keeps pretty JSON", () => {
  const out = toolCallArgsDisplay('{"foo":1}', { kind: "other" });
  expect(out).toContain('"foo"');
});
