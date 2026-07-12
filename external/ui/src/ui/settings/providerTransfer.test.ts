import { describe, expect, it } from "vitest";
import {
  parseProviderTransfer,
  providerToClipboard,
  providerToJson,
  sanitizeProvider,
  uniqueProviderName,
} from "./providerTransfer";

const fullProvider = {
  name: "openrouter",
  type: "openai",
  api_base: "https://openrouter.ai/api/v1",
  api_key: "sk-secret-123",
  api_key_command: "op read op://vault/key",
  proxy: "socks5://127.0.0.1:1080",
};

describe("sanitizeProvider", () => {
  it("keeps only safe non-empty fields and drops secrets", () => {
    expect(sanitizeProvider(fullProvider)).toEqual({
      name: "openrouter",
      type: "openai",
      api_base: "https://openrouter.ai/api/v1",
    });
  });

  it("omits empty transfer fields", () => {
    expect(sanitizeProvider({ name: "p", type: "anthropic", api_base: "" })).toEqual({
      name: "p",
      type: "anthropic",
    });
  });
});

describe("providerToJson", () => {
  it("serializes without secrets", () => {
    const json = providerToJson(fullProvider);
    const parsed = JSON.parse(json) as Record<string, unknown>;
    expect(parsed).toEqual({
      name: "openrouter",
      type: "openai",
      api_base: "https://openrouter.ai/api/v1",
    });
    expect(json).not.toContain("sk-secret");
    expect(json).not.toContain("socks5");
    expect(json).not.toContain("op read");
  });
});

describe("providerToClipboard", () => {
  it("produces a foxxycode://provider query without secrets", () => {
    const s = providerToClipboard(fullProvider);
    expect(s.startsWith("foxxycode://provider?")).toBe(true);
    expect(s).not.toContain("sk-secret");
    expect(s).not.toContain("socks5");
    expect(s).not.toContain("api_key");
    expect(s).not.toContain("proxy");
  });

  it("round-trips through parseProviderTransfer", () => {
    const s = providerToClipboard(fullProvider);
    expect(parseProviderTransfer(s)).toEqual([
      {
        name: "openrouter",
        type: "openai",
        api_base: "https://openrouter.ai/api/v1",
      },
    ]);
  });
});

describe("parseProviderTransfer", () => {
  it("parses a bare query string", () => {
    expect(
      parseProviderTransfer("name=foo&type=openai&api_base=https://x/v1"),
    ).toEqual([{ name: "foo", type: "openai", api_base: "https://x/v1" }]);
  });

  it("parses the foxxycode:// scheme form", () => {
    expect(
      parseProviderTransfer("foxxycode://provider?name=bar&type=anthropic"),
    ).toEqual([{ name: "bar", type: "anthropic" }]);
  });

  it("parses a single JSON object and drops secrets", () => {
    const text = JSON.stringify(fullProvider);
    expect(parseProviderTransfer(text)).toEqual([
      {
        name: "openrouter",
        type: "openai",
        api_base: "https://openrouter.ai/api/v1",
      },
    ]);
  });

  it("parses a JSON array of providers", () => {
    const text = JSON.stringify([
      { name: "a", type: "openai" },
      { name: "b", type: "anthropic", api_key: "leak" },
    ]);
    expect(parseProviderTransfer(text)).toEqual([
      { name: "a", type: "openai" },
      { name: "b", type: "anthropic" },
    ]);
  });

  it("returns empty for blank or field-less input", () => {
    expect(parseProviderTransfer("   ")).toEqual([]);
    expect(parseProviderTransfer("foxxycode://provider?nope=1")).toEqual([]);
  });

  it("throws on malformed JSON", () => {
    expect(() => parseProviderTransfer("{ not json")).toThrow();
  });
});

describe("uniqueProviderName", () => {
  it("returns the name when unused", () => {
    expect(uniqueProviderName("foo", ["bar"])).toBe("foo");
  });

  it("appends -1 on first collision", () => {
    expect(uniqueProviderName("foo", ["foo"])).toBe("foo-1");
  });

  it("increments until free", () => {
    expect(uniqueProviderName("foo", ["foo", "foo-1", "foo-2"])).toBe("foo-3");
  });

  it("leaves an empty name unchanged", () => {
    expect(uniqueProviderName("", ["foo"])).toBe("");
  });
});
