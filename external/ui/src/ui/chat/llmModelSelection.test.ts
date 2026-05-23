import { expect, test } from "vitest";
import {
  pickDefaultLlmModelForNewChat,
  pickLlmModelForOpenSession,
} from "./llmModelSelection";

const backends = ["openai/gpt-4o", "openai/gpt-4o-mini"] as const;

test("new chat prefers cookie over config default", () => {
  expect(
    pickDefaultLlmModelForNewChat({
      backends,
      cookie: "openai/gpt-4o-mini",
      defaultAgentModel: "openai/gpt-4o",
    }),
  ).toBe("openai/gpt-4o-mini");
});

test("open session uses stored model even when cookie differs", () => {
  expect(
    pickLlmModelForOpenSession({
      backends,
      sessionModel: "openai/gpt-4o-mini",
      cookie: "openai/gpt-4o",
      defaultAgentModel: "openai/gpt-4o",
    }),
  ).toBe("openai/gpt-4o-mini");
});

test("open session without stored model falls back to new-chat default", () => {
  expect(
    pickLlmModelForOpenSession({
      backends,
      sessionModel: "",
      cookie: "openai/gpt-4o-mini",
      defaultAgentModel: "openai/gpt-4o",
    }),
  ).toBe("openai/gpt-4o-mini");
});
