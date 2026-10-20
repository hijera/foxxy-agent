import React from "react";
import { afterEach, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SettingsSection } from "./SettingsSection";
import type { JsonSchema } from "./SchemaForm";
import type { SectionDescriptor } from "./settingsSections";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const modelsSection: SectionDescriptor = {
  id: "models",
  label: "Logical models",
  kind: "array",
  schemaKey: "models",
  labelField: "model",
};

// Mirrors what UISchemaMap() emits for models[]: `multimodal` is a sibling of
// `model`, rendered by the schema-driven form, so the model picker can only reach
// it through the field-override seam.
const modelsSchema: JsonSchema = {
  type: "object",
  properties: {
    models: {
      type: "array",
      title: "Logical models",
      items: {
        type: "object",
        properties: {
          model: { type: "string", title: "Model id" },
          multimodal: { type: "boolean", title: "Multimodal" },
          max_context_tokens: { type: "integer", title: "Context window (tokens)" },
        },
        "x-foxxycode-property-order": ["model", "multimodal"],
      },
    },
  },
};

const providersSection: SectionDescriptor = {
  id: "providers", label: "LLM providers", kind: "array", schemaKey: "providers", labelField: "name",
};
modelsSchema.properties!.providers = {
  type: "array", items: { type: "object", properties: { name: { type: "string" }, type: { type: "string" } } },
};
function Harness() {
  const [section, setSection] = React.useState(providersSection);
  const [doc, setDoc] = React.useState<Record<string, unknown>>({
    providers: [{ name: "demo", type: "openai" }], models: [],
  });
  return <>
    <output data-testid="doc">{JSON.stringify(doc.models)}</output>
    <button onClick={() => setSection(modelsSection)}>Edit models</button>
    <SettingsSection section={section} schema={modelsSchema} doc={doc} setDoc={setDoc} />
  </>;
}
async function addAdvertisedModel(vision: boolean) {
  vi.spyOn(globalThis, "fetch").mockResolvedValue({ ok: true,
    json: async () => ({ ok: true, models: [{ id: "m", vision }] }),
  } as Response);
  render(<Harness />);
  fireEvent.click(screen.getByTestId("settings-master-item-0"));
  fireEvent.click(await screen.findByTestId("provider-model-toggle-m"));
  expect(JSON.parse(screen.getByTestId("doc").textContent!)).toEqual([{ model: "demo/m", max_context_tokens: 0, multimodal: vision }]);
  fireEvent.click(screen.getByText("Edit models"));
  fireEvent.click(screen.getByTestId("settings-master-item-0"));
}
test("adding an advertised vision model sets multimodal in the same document update", async () => {
  await addAdvertisedModel(true);
  expect(screen.getByRole("switch", { name: /multimodal/i })).toHaveAttribute("aria-checked", "true");
});
test("the catalog default for a text model can be overridden", async () => {
  await addAdvertisedModel(false);
  const toggle = screen.getByRole("switch", { name: /multimodal/i });
  expect(toggle).toHaveAttribute("aria-checked", "false");
  fireEvent.click(toggle);
  expect(JSON.parse(screen.getByTestId("doc").textContent!)).toEqual([{ model: "demo/m", max_context_tokens: 0, multimodal: true }]);
});
test("typing a model id preserves the operator's multimodal choice", async () => {
  await addAdvertisedModel(true);
  fireEvent.change(screen.getByTestId("model-field-model"), { target: { value: "manual" } });
  expect(JSON.parse(screen.getByTestId("doc").textContent!)).toEqual([{ model: "demo/manual", max_context_tokens: 0, multimodal: true }]);
});


test("a model added from the provider list carries the context window the provider reports", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue({ ok: true,
    json: async () => ({ ok: true, models: [{ id: "m", vision: true, context_window: 131072 }] }),
  } as Response);
  render(<Harness />);
  fireEvent.click(screen.getByTestId("settings-master-item-0"));
  fireEvent.click(await screen.findByTestId("provider-model-toggle-m"));
  expect(JSON.parse(screen.getByTestId("doc").textContent!)).toEqual([
    { model: "demo/m", multimodal: true, max_context_tokens: 131072 },
  ]);
});
