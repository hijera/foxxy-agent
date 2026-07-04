import { expect, test } from "vitest";
import {
  FOXXYCODE_LLM_MODEL_COOKIE,
  readLlmModelCookie,
  writeLlmModelCookie,
} from "./llmModelCookie";

test("write then read llm model cookie", () => {
  document.cookie = `${FOXXYCODE_LLM_MODEL_COOKIE}=; Max-Age=0; Path=/`;
  Object.defineProperty(window, "location", {
    value: new URL("http://127.0.0.1:5173/"),
    configurable: true,
  });
  writeLlmModelCookie("openai/gpt-4o-mini");
  expect(readLlmModelCookie()).toBe("openai/gpt-4o-mini");
});
