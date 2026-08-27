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
        },
        "x-foxxycode-property-order": ["model", "multimodal"],
      },
    },
  },
};

function Harness() {
  const [doc, setDoc] = React.useState<Record<string, unknown>>({
    providers: [{ name: "neuraldeep", type: "neuraldeep" }],
    models: [{ model: "", multimodal: false }],
    agent: { model: "", max_turns: 20 },
  });
  return (
    <>
      <span data-testid="doc">{JSON.stringify(doc.models)}</span>
      <SettingsSection
        section={modelsSection}
        schema={modelsSchema}
        doc={doc}
        setDoc={setDoc}
      />
    </>
  );
}

async function openRowAndFetch(models: unknown[]) {
  vi.spyOn(globalThis, "fetch").mockResolvedValue({
    ok: true,
    json: async () => ({ ok: true, models }),
  } as unknown as Response);

  render(<Harness />);
  fireEvent.click(screen.getByTestId("settings-master-item-0"));
  fireEvent.click(screen.getByTestId("model-field-fetch"));
  await waitFor(() =>
    expect(screen.getByTestId("model-field-fetch").textContent).toBe("Fetch models"),
  );
}

// The reported config.yaml had multimodal:false under a model that reads images,
// because Settings only ever wrote the model id. Picking a listed model must now
// seed the sibling flag from the catalog in the same edit.
test("picking a vision model in Settings ticks the sibling Multimodal switch", async () => {
  await openRowAndFetch([
    { id: "qwen3.6-35b-a3b", vision: true },
    { id: "gpt-oss-20b" },
  ]);

  const multimodal = () => screen.getByRole("switch", { name: /multimodal/i });
  expect(multimodal().getAttribute("aria-checked")).toBe("false");

  fireEvent.focus(screen.getByTestId("model-field-model"));
  fireEvent.mouseDown(await screen.findByText(/neuraldeep\/qwen3\.6-35b-a3b/));

  await waitFor(() => {
    expect(multimodal().getAttribute("aria-checked")).toBe("true");
  });
  // Both keys land in one document update: the id must not be lost to a stale
  // copy of the row while the sibling is patched.
  expect(JSON.parse(screen.getByTestId("doc").textContent!)).toEqual([
    { model: "neuraldeep/qwen3.6-35b-a3b", multimodal: true },
  ]);
});

test("picking a text-only model clears Multimodal, and the operator can override", async () => {
  await openRowAndFetch([
    { id: "qwen3.6-35b-a3b", vision: true },
    { id: "gpt-oss-120b" },
  ]);

  const multimodal = () => screen.getByRole("switch", { name: /multimodal/i });
  fireEvent.focus(screen.getByTestId("model-field-model"));
  fireEvent.mouseDown(await screen.findByText(/neuraldeep\/qwen3\.6-35b-a3b/));
  await waitFor(() => {
    expect(multimodal().getAttribute("aria-checked")).toBe("true");
  });

  fireEvent.focus(screen.getByTestId("model-field-model"));
  fireEvent.mouseDown(await screen.findByText(/neuraldeep\/gpt-oss-120b/));
  await waitFor(() => {
    expect(multimodal().getAttribute("aria-checked")).toBe("false");
  });

  // gpt-oss-120b advertises no image input but does accept images, so the
  // catalog value is a default, not a gate: the switch stays operator-editable.
  fireEvent.click(multimodal());
  await waitFor(() => {
    expect(multimodal().getAttribute("aria-checked")).toBe("true");
  });
  expect(JSON.parse(screen.getByTestId("doc").textContent!)).toEqual([
    { model: "neuraldeep/gpt-oss-120b", multimodal: true },
  ]);
});

test("a hand-typed model id leaves Multimodal untouched", async () => {
  await openRowAndFetch([{ id: "qwen3.6-35b-a3b", vision: true }]);

  const multimodal = () => screen.getByRole("switch", { name: /multimodal/i });
  fireEvent.click(multimodal());
  await waitFor(() => {
    expect(multimodal().getAttribute("aria-checked")).toBe("true");
  });

  fireEvent.change(screen.getByTestId("model-field-model"), {
    target: { value: "neuraldeep/not-in-the-catalog" },
  });
  await waitFor(() => {
    expect(JSON.parse(screen.getByTestId("doc").textContent!)).toEqual([
      { model: "neuraldeep/not-in-the-catalog", multimodal: true },
    ]);
  });
});
