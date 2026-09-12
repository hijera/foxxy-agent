import { expect, test } from "vitest";
import {
  isSpawnAgentRefusal,
  parseSpawnAgentName,
  refusedSpawnAgentName,
} from "./spawnAgentApproval";

test("only a failed spawn_agent call is a refusal", () => {
  expect(isSpawnAgentRefusal({ title: "spawn_agent", status: "failed" })).toBe(true);
  expect(isSpawnAgentRefusal({ kind: "spawn_agent", status: "FAILED" })).toBe(true);
  expect(isSpawnAgentRefusal({ title: "spawn_agent", status: "completed" })).toBe(false);
  expect(isSpawnAgentRefusal({ title: "spawn_agent", status: "in_progress" })).toBe(false);
  expect(isSpawnAgentRefusal({ title: "run_command", status: "failed" })).toBe(false);
  expect(isSpawnAgentRefusal({})).toBe(false);
});

test("the agent name comes from the call's own arguments", () => {
  expect(parseSpawnAgentName('{"agent":"reviewer","prompt":"go"}')).toBe("reviewer");
  expect(parseSpawnAgentName('{"agent":"  spacey  "}')).toBe("spacey");
});

test("arguments that are absent, truncated or the wrong shape yield no name", () => {
  // A streamed call can render before its arguments finish arriving.
  expect(parseSpawnAgentName('{"agent":"revie')).toBe("");
  expect(parseSpawnAgentName("")).toBe("");
  expect(parseSpawnAgentName(undefined)).toBe("");
  expect(parseSpawnAgentName("[1,2]")).toBe("");
  expect(parseSpawnAgentName('{"agent":42}')).toBe("");
  expect(parseSpawnAgentName('{"prompt":"no agent here"}')).toBe("");
});

test("a name is offered only when both halves hold", () => {
  expect(
    refusedSpawnAgentName({
      title: "spawn_agent",
      status: "failed",
      argsText: '{"agent":"reviewer"}',
    }),
  ).toBe("reviewer");
  expect(
    refusedSpawnAgentName({
      title: "spawn_agent",
      status: "completed",
      argsText: '{"agent":"reviewer"}',
    }),
  ).toBe("");
});
