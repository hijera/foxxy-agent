import React from "react";
import { afterEach, expect, test } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { SchemaForm, type JsonSchema } from "./SchemaForm";

afterEach(cleanup);

// Regression: rendering an array field with existing rows used to crash the whole
// SPA ("t is not a function") because SchemaField shadowed the i18n t() with the
// schema type string, and the per-row remove button calls t("settings.remove").
// Tools -> command_allowlist (or an MCP server's args/env) hit this in the wild.
const toolsSchema: JsonSchema = {
  type: "object",
  properties: {
    command_allowlist: {
      type: "array",
      title: "Command allowlist",
      items: { type: "string" },
    },
  },
} as unknown as JsonSchema;

test("array field with populated rows renders remove buttons instead of crashing", () => {
  function Harness() {
    const [doc, setDoc] = React.useState<Record<string, unknown>>({
      command_allowlist: ["go build", "go test"],
    });
    return <SchemaForm schema={toolsSchema} value={doc} onChange={setDoc} />;
  }
  render(<Harness />);
  expect(screen.getByDisplayValue("go build")).toBeTruthy();
  expect(screen.getByDisplayValue("go test")).toBeTruthy();
  expect(screen.getAllByRole("button", { name: /remove/i })).toHaveLength(2);
});
