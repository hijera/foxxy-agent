import { expect, test } from "vitest";
import { rebaseSettings } from "./rebaseSettings";

test("replays nested edits and deletions while preserving concurrent fields", () => {
  const sent = { revision: "old", agent: { max_turns: 45, model: "old", optional: 1 } };
  const edited = { ...sent, agent: { max_turns: 40, model: "old" } };
  const saved = { revision: "new", agent: { max_turns: 45, model: "new", optional: 1, extra: true } };
  expect(rebaseSettings(sent, edited, saved)).toEqual({
    revision: "new", agent: { max_turns: 40, model: "new", extra: true },
  });
  expect(saved.agent.optional).toBe(1);
});

test("keeps edited lists atomic and accepts concurrent changes to untouched lists", () => {
  const sent = { providers: [{ name: "a" }], models: ["a"] };
  const edited = { providers: [{ name: "mine" }], models: ["a"] };
  const saved = { revision: "new", providers: [{ name: "theirs" }], models: ["b"] };
  expect(rebaseSettings(sent, edited, saved)).toEqual({
    revision: "new", providers: [{ name: "mine" }], models: ["b"],
  });
});
