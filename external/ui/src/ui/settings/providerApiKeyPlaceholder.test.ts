import { describe, expect, it } from "vitest";
import {
  providerAPIKeyEnvVarName,
  providerApiKeyFieldPlaceholder,
} from "./providerApiKeyPlaceholder";

describe("providerAPIKeyEnvVarName", () => {
  it("maps hyphenated names to upper snake _API_KEY", () => {
    expect(providerAPIKeyEnvVarName("my-rpa")).toBe("MY_RPA_API_KEY");
    expect(providerAPIKeyEnvVarName("rpa")).toBe("RPA_API_KEY");
  });

  it("returns empty for invalid ids", () => {
    expect(providerAPIKeyEnvVarName("")).toBe("");
    expect(providerAPIKeyEnvVarName("bad name")).toBe("");
    expect(providerAPIKeyEnvVarName("9x")).toBe("");
  });
});

describe("providerApiKeyFieldPlaceholder", () => {
  it("embeds the env name for valid provider names", () => {
    expect(providerApiKeyFieldPlaceholder("rpa")).toMatch(/\$\{RPA_API_KEY\}/);
  });

  it("uses a plain hint when the id does not start with a letter", () => {
    const ph = providerApiKeyFieldPlaceholder("9x");
    expect(ph).toContain("start with a letter");
    expect(ph).not.toMatch(/\$\{<|<PROVIDER>/);
  });
});
