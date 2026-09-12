import { describe, expect, it } from "vitest";

import {
  DISABLED_CONFIG,
  parseClientConfig,
  suggestsWhileTyping,
} from "../src/autocomplete/clientConfig";
import { advanceCached } from "../src/autocomplete/suggestionCache";
import { shouldRequestAutomatically } from "../src/autocomplete/triggerPolicy";
import { retryAfterSeconds } from "../src/autocomplete/completionClient";

describe("429 pause", () => {
  it("follows Retry-After within sane bounds", () => {
    expect(retryAfterSeconds("6")).toBe(6);
    expect(retryAfterSeconds(" 6 ")).toBe(6);
    expect(retryAfterSeconds(undefined)).toBe(10);
    expect(retryAfterSeconds("soon")).toBe(10);
    expect(retryAfterSeconds("0")).toBe(10);
    expect(retryAfterSeconds("86400")).toBe(60);
  });
});

describe("automatic trigger policy", () => {
  it("skips a caret inside a word", () => {
    expect(shouldRequestAutomatically("fo", "o")).toBe(false);
    expect(shouldRequestAutomatically("fo", "_")).toBe(false);
    expect(shouldRequestAutomatically("fo", "9")).toBe(false);
    expect(shouldRequestAutomatically("fo", "Ж")).toBe(false);
  });

  it("skips the keystroke that closed a bracket or ended a statement", () => {
    expect(shouldRequestAutomatically("foo()", "")).toBe(false);
    expect(shouldRequestAutomatically("  }", "")).toBe(false);
    expect(shouldRequestAutomatically("x := 1;", "\n")).toBe(false);
  });

  it("asks again once the user types past the closer", () => {
    expect(shouldRequestAutomatically("foo() ", "")).toBe(true);
    expect(shouldRequestAutomatically("foo(); x", "")).toBe(true);
  });

  it("asks on ordinary typing, at line ends, and on an empty line", () => {
    expect(shouldRequestAutomatically("\treturn ", "")).toBe(true);
    expect(shouldRequestAutomatically("func f() {", "")).toBe(true);
    expect(shouldRequestAutomatically("", "")).toBe(true);
    expect(shouldRequestAutomatically("\t\t", "}")).toBe(true);
    expect(shouldRequestAutomatically("foo(", ")")).toBe(true);
  });
});

describe("autocomplete client config", () => {
  it("parses a full answer", () => {
    const cfg = parseClientConfig(
      JSON.stringify({
        enabled: true,
        trigger: "manual",
        debounce_ms: 700,
        multi_line: false,
        timeout_ms: 2500,
        max_prefix_bytes: 1234,
        max_suffix_bytes: 567,
      }),
    );
    expect(cfg).toEqual({
      enabled: true,
      trigger: "manual",
      debounceMs: 700,
      multiLine: false,
      timeoutMs: 2500,
      maxPrefixBytes: 1234,
      maxSuffixBytes: 567,
    });
  });

  it("falls back per field for missing or nonsensical values", () => {
    const cfg = parseClientConfig(JSON.stringify({ enabled: true, debounce_ms: 0 }))!;
    expect(cfg.debounceMs).toBe(DISABLED_CONFIG.debounceMs);
    expect(cfg.trigger).toBe("auto");
    expect(cfg.maxPrefixBytes).toBe(DISABLED_CONFIG.maxPrefixBytes);
    expect(cfg.multiLine).toBe(true);
  });

  it("rejects a body that is not an object", () => {
    expect(parseClientConfig("not json")).toBeNull();
    expect(parseClientConfig("[1,2,3]")).toBeNull();
  });

  it("never suggests while typing when the trigger is manual or the section is off", () => {
    expect(suggestsWhileTyping({ ...DISABLED_CONFIG, enabled: true })).toBe(true);
    expect(
      suggestsWhileTyping({ ...DISABLED_CONFIG, enabled: true, trigger: "manual" }),
    ).toBe(false);
    expect(suggestsWhileTyping(DISABLED_CONFIG)).toBe(false);
  });

  it("stays off until the backend answers", () => {
    expect(DISABLED_CONFIG.enabled).toBe(false);
    expect(suggestsWhileTyping(DISABLED_CONFIG)).toBe(false);
  });
});

describe("suggestion prefix cache", () => {
  const doc = "func add(a, b int) int {\n\treturn a + b\n}";
  // Offset of the caret right after "return ".
  const anchor = doc.indexOf("a + b");
  const cached = { uri: "file:///main.go", offset: anchor, text: "a + b" };

  it("returns the remainder when the user typed what was suggested", () => {
    expect(advanceCached(cached, cached.uri, anchor + 1, doc)).toBe(" + b");
    expect(advanceCached(cached, cached.uri, anchor + 2, doc)).toBe("+ b");
  });

  it("returns the whole suggestion when nothing was typed yet", () => {
    expect(advanceCached(cached, cached.uri, anchor, doc)).toBe("a + b");
  });

  it("returns empty once the suggestion has been typed out in full", () => {
    expect(advanceCached(cached, cached.uri, anchor + 5, doc)).toBe("");
  });

  it("misses when the typed text diverges from the suggestion", () => {
    const diverged = "func add(a, b int) int {\n\treturn x + b\n}";
    expect(advanceCached(cached, cached.uri, anchor + 1, diverged)).toBeNull();
  });

  it("misses for another document, an earlier caret, or no cache", () => {
    expect(advanceCached(cached, "file:///other.go", anchor + 1, doc)).toBeNull();
    expect(advanceCached(cached, cached.uri, anchor - 1, doc)).toBeNull();
    expect(advanceCached(null, cached.uri, anchor, doc)).toBeNull();
  });

  it("misses when the caret has run past the end of the suggestion", () => {
    expect(advanceCached(cached, cached.uri, anchor + 6, doc)).toBeNull();
  });
});
