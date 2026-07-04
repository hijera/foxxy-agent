import { afterEach, expect, test } from "vitest";
import {
  FOXXYCODE_LLM_REASONING_COOKIE,
  readReasoningCookie,
  writeReasoningCookie,
} from "./reasoningCookie";

afterEach(() => {
  document.cookie = `${FOXXYCODE_LLM_REASONING_COOKIE}=; Path=/; Max-Age=0`;
});

test("write then read round-trips the level", () => {
  writeReasoningCookie("high");
  expect(readReasoningCookie()).toBe("high");
});

test("empty value is not written", () => {
  writeReasoningCookie("");
  expect(readReasoningCookie()).toBeNull();
});
