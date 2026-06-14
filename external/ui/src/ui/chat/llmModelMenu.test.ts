import { expect, test } from "vitest";
import {
  LLM_MENU_FILTER_THRESHOLD,
  filterLlmModels,
  groupLlmModelsByVendor,
  llmMenuVendorCount,
  llmModelNameOf,
  llmVendorOf,
  shouldGroupLlmModels,
  shouldShowLlmFilter,
} from "./llmModelMenu";

const sample = [
  "opencode-go/deepseek-v4-pro",
  "opencode-go/kimi-k2.6",
  "cliproxyapi/gpt-5.5",
  "cliproxyapi/gemini-2.5-pro",
] as const;

test("llmVendorOf splits on the first slash, empty when no slash", () => {
  expect(llmVendorOf("opencode-go/deepseek-v4-pro")).toBe("opencode-go");
  expect(llmVendorOf("cliproxyapi/gpt-5.5")).toBe("cliproxyapi");
  expect(llmVendorOf("gpt-4o")).toBe("");
  expect(llmVendorOf("")).toBe("");
});

test("llmModelNameOf returns the part after the first slash", () => {
  expect(llmModelNameOf("opencode-go/deepseek-v4-pro")).toBe("deepseek-v4-pro");
  expect(llmModelNameOf("gpt-4o")).toBe("gpt-4o");
});

test("shouldShowLlmFilter only when count exceeds the threshold (5)", () => {
  expect(LLM_MENU_FILTER_THRESHOLD).toBe(5);
  expect(shouldShowLlmFilter(5)).toBe(false); // exactly at the cap: no filter
  expect(shouldShowLlmFilter(6)).toBe(true); // one over: filter appears
  expect(shouldShowLlmFilter(3)).toBe(false);
});

test("llmMenuVendorCount counts distinct vendors", () => {
  expect(llmMenuVendorCount(sample)).toBe(2);
  expect(llmMenuVendorCount(["a/x", "a/y"])).toBe(1);
  expect(llmMenuVendorCount(["plain"])).toBe(1); // "" vendor still counts as one bucket
});

test("shouldGroupLlmModels only when more than one vendor", () => {
  expect(shouldGroupLlmModels(sample)).toBe(true);
  expect(shouldGroupLlmModels(["a/x", "a/y"])).toBe(false);
  expect(shouldGroupLlmModels([])).toBe(false);
});

test("filterLlmModels matches vendor or model name, case-insensitive", () => {
  // by model name
  expect(filterLlmModels(sample, "gpt")).toEqual(["cliproxyapi/gpt-5.5"]);
  // by vendor prefix
  expect(filterLlmModels(sample, "opencode")).toEqual([
    "opencode-go/deepseek-v4-pro",
    "opencode-go/kimi-k2.6",
  ]);
  // case-insensitive + trimmed
  expect(filterLlmModels(sample, "  GEMINI ")).toEqual([
    "cliproxyapi/gemini-2.5-pro",
  ]);
});

test("filterLlmModels with empty query returns all in order", () => {
  expect(filterLlmModels(sample, "")).toEqual([...sample]);
  expect(filterLlmModels(sample, "   ")).toEqual([...sample]);
});

test("filterLlmModels returns empty array when nothing matches", () => {
  expect(filterLlmModels(sample, "zzz-nope")).toEqual([]);
});

test("groupLlmModelsByVendor keeps first-seen vendor order and member order", () => {
  const groups = groupLlmModelsByVendor([
    "cliproxyapi/gpt-5.5",
    "opencode-go/kimi-k2.6",
    "cliproxyapi/gemini-2.5-pro",
    "opencode-go/deepseek-v4-pro",
  ]);
  expect(groups).toEqual([
    {
      vendor: "cliproxyapi",
      models: ["cliproxyapi/gpt-5.5", "cliproxyapi/gemini-2.5-pro"],
    },
    {
      vendor: "opencode-go",
      models: ["opencode-go/kimi-k2.6", "opencode-go/deepseek-v4-pro"],
    },
  ]);
});

test("groupLlmModelsByVendor buckets slashless ids under empty vendor", () => {
  const groups = groupLlmModelsByVendor(["plain-a", "plain-b"]);
  expect(groups).toEqual([{ vendor: "", models: ["plain-a", "plain-b"] }]);
});
