import { describe, expect, it } from "vitest";

import uiSchema from "./__fixtures__/ui-schema.json";
import { initLocale } from "./i18n";
import { schemaTextRu } from "./messages/schema.ru";
import { tSchemaText } from "./schemaStrings";

/**
 * Regression guard for Settings → Skills: the auto-discovery row renders the
 * backend schema description, so that exact string must have a Russian overlay
 * entry and must resolve through tSchemaText under the ru locale.
 */
describe("skills auto_discovery schema description", () => {
  const desc = (
    uiSchema as unknown as {
      properties: {
        skills: { properties: { auto_discovery: { description: string } } };
      };
    }
  ).properties.skills.properties.auto_discovery.description;

  it("is present in the committed schema fixture", () => {
    expect(desc).toContain("load_skill");
  });

  it("has a Russian overlay entry keyed by the exact schema string", () => {
    expect(schemaTextRu[desc]).toBeDefined();
  });

  it("resolves to Russian through tSchemaText under the ru locale", () => {
    initLocale("ru");
    const out = tSchemaText(desc);
    expect(out).not.toBe(desc);
    expect(out).toContain("load_skill");
  });
});
