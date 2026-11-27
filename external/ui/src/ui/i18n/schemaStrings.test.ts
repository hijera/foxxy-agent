import { describe, expect, it } from "vitest";

import { initLocale } from "./i18n";
import { tSchemaEnumLabel } from "./schemaStrings";

import uiSchema from "./__fixtures__/ui-schema.json";
import { schemaEnumLabelRu, schemaTextRu } from "./messages/schema.ru";

type Node = Record<string, unknown>;

/**
 * Walk the committed UI-schema snapshot (regenerated from Go via
 * `UPDATE_UI_SCHEMA_FIXTURE=1 go test ./internal/config`) and collect every
 * translatable string: field `title`/`description` and enum tokens. The Russian
 * overlay must cover them all, otherwise the Settings form silently shows English.
 */
function collect(node: unknown, texts: Set<string>, enums: Set<string>): void {
  if (Array.isArray(node)) {
    for (const child of node) {
      collect(child, texts, enums);
    }
    return;
  }
  if (!node || typeof node !== "object") {
    return;
  }
  const obj = node as Node;
  if (typeof obj.title === "string") {
    texts.add(obj.title);
  }
  if (typeof obj.description === "string") {
    texts.add(obj.description);
  }
  if (Array.isArray(obj.enum)) {
    for (const tok of obj.enum) {
      enums.add(String(tok));
    }
  }
  for (const [key, value] of Object.entries(obj)) {
    // Skip meta arrays that are not schema subtrees.
    if (
      key === "enum" ||
      key === "required" ||
      key === "x-foxxycode-property-order"
    ) {
      continue;
    }
    collect(value, texts, enums);
  }
}

describe("schema Russian overlay coverage", () => {
  const texts = new Set<string>();
  const enums = new Set<string>();
  collect(uiSchema, texts, enums);

  it("translates every schema title and description", () => {
    const missing = [...texts].filter((s) => !(s in schemaTextRu));
    expect(
      missing,
      `missing schemaTextRu entries:\n${missing.join("\n")}`,
    ).toEqual([]);
  });

  it("labels every enum token", () => {
    const missing = [...enums].filter((tok) => !(tok in schemaEnumLabelRu));
    expect(
      missing,
      `missing schemaEnumLabelRu entries:\n${missing.join("\n")}`,
    ).toEqual([]);
  });
});

describe("schema enum labels in English", () => {
  it("gives the screenshot modes their English labels and keeps the token for the rest", () => {
    initLocale("en");
    expect(tSchemaEnumLabel("keep")).toBe("Keep screenshots in context");
    expect(tSchemaEnumLabel("latest")).toBe("Remove intermediate screenshots");
    expect(tSchemaEnumLabel("replace_old")).toBe(
      "Replace old screenshots with descriptions",
    );
    expect(tSchemaEnumLabel("separate_session")).toBe(
      "Process screenshots in a separate session",
    );
    // Tokens without an English label fall back to the token itself.
    expect(tSchemaEnumLabel("debug")).toBe("debug");
    // And the Russian overlay wins over the English label when it exists.
    initLocale("ru");
    expect(tSchemaEnumLabel("keep")).toBe(schemaEnumLabelRu["keep"]);
    initLocale("en");
  });
});
