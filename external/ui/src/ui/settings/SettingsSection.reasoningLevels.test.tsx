import React from "react";
import { afterEach, expect, test, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { SettingsSection } from "./SettingsSection";
import type { JsonSchema } from "./SchemaForm";
import type { SectionDescriptor } from "./settingsSections";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const modelsSection: SectionDescriptor = {
  id: "models",
  label: "Logical models",
  kind: "array",
  schemaKey: "models",
  labelField: "model",
};

const reasoningModelsSchema: JsonSchema = {
  type: "object",
  properties: {
    models: {
      type: "array",
      title: "Logical models",
      items: {
        type: "object",
        properties: {
          model: { type: "string", title: "Model id" },
          reasoning_levels: {
            type: "array",
            title: "Reasoning levels",
            items: { type: "string" },
          },
          // stream is the one model key whose absence means true, so it is seeded
          // from the schema default. Keeping it here pins that the item factory
          // omits reasoning_levels only, rather than everything it does not know.
          stream: { type: "boolean", title: "Stream responses", default: true },
        },
        "x-foxxycode-property-order": ["model", "reasoning_levels", "stream"],
      },
    },
  },
};

// stubModelsAndLevels answers both fetches the logical-model form makes, routed
// by URL: the provider model list behind "Fetch models" and the detected
// reasoning levels behind "Fetch reasoning levels".
function stubModelsAndLevels(levels: string[]) {
  const fetchMock = vi.fn(async (input: unknown) => {
    const url = String(input);
    if (url.startsWith("/foxxycode/config/reasoning-levels")) {
      return {
        ok: true,
        json: async () => ({
          ok: true,
          levels,
          detected: levels.length > 0,
        }),
      };
    }
    return {
      ok: true,
      json: async () => ({ ok: true, models: [{ id: "qwen3.8-27b" }] }),
    };
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function ReasoningModelsHarness() {
  const [doc, setDoc] = React.useState<Record<string, unknown>>({
    providers: [{ name: "valera", type: "openai" }],
    models: [],
  });
  return (
    <>
      <output data-testid="settings-doc">{JSON.stringify(doc)}</output>
      <SettingsSection
        section={modelsSection}
        schema={reasoningModelsSchema}
        doc={doc}
        setDoc={setDoc}
      />
    </>
  );
}

// Fork note: picking a fetched model also seeds `multimodal` from the catalog
// (ModelField syncsMultimodal), so every expectation below carries it.

// addFetchedModel walks the form the way an operator does: Add, Fetch models,
// then pick the fetched id out of the combobox.
async function addFetchedModel() {
  fireEvent.click(screen.getByTestId("settings-master-add"));
  fireEvent.click(screen.getByTestId("model-field-fetch"));
  await waitFor(() =>
    expect(screen.getByTestId("model-field-fetch").textContent).toBe(
      "Fetch models",
    ),
  );
  fireEvent.focus(screen.getByTestId("model-field-model"));
  fireEvent.mouseDown(await screen.findByText("valera/qwen3.8-27b"));
}

function savedModels(): unknown {
  return JSON.parse(screen.getByTestId("settings-doc").textContent || "{}")
    .models;
}

test("adding a fetched model leaves reasoning auto-detection enabled", async () => {
  stubModelsAndLevels(["low", "medium", "high"]);

  render(<ReasoningModelsHarness />);
  await addFetchedModel();

  // reasoning_levels is absent (auto-detect), while stream keeps its schema
  // default: the item factory omits the one key whose empty value means
  // something, not every key it was not told about.
  expect(savedModels()).toEqual([
    { model: "valera/qwen3.8-27b", multimodal: false, stream: true },
  ]);
});

test("fetch reasoning levels fills the field for the model being edited", async () => {
  const fetchMock = stubModelsAndLevels(["low", "medium", "high"]);

  render(<ReasoningModelsHarness />);
  await addFetchedModel();

  fireEvent.click(screen.getByTestId("reasoning-levels-fetch"));
  await waitFor(() =>
    expect(savedModels()).toEqual([
      {
        model: "valera/qwen3.8-27b",
        multimodal: false,
        stream: true,
        reasoning_levels: ["low", "medium", "high"],
      },
    ]),
  );

  // The id typed into the form is what gets resolved, not a saved models[] row,
  // together with the type of the provider row it points at.
  expect(fetchMock).toHaveBeenCalledWith(
    "/foxxycode/config/reasoning-levels?model=valera%2Fqwen3.8-27b&provider_type=openai",
  );

  // And the operator can hand the decision back to the backend.
  fireEvent.click(screen.getByTestId("reasoning-levels-auto"));
  expect(savedModels()).toEqual([
    { model: "valera/qwen3.8-27b", multimodal: false, stream: true },
  ]);
});

test("fetch reasoning levels sends the type of the provider row in the form", async () => {
  const fetchMock = stubModelsAndLevels(["low", "medium", "high"]);

  render(<ReasoningModelsHarness />);
  await addFetchedModel();
  fireEvent.click(screen.getByTestId("reasoning-levels-fetch"));

  // valera is an openai provider in the (unsaved) settings document, and that
  // is what decides the Codex remap server-side, not the config on disk.
  await waitFor(() =>
    expect(fetchMock).toHaveBeenCalledWith(
      "/foxxycode/config/reasoning-levels?model=valera%2Fqwen3.8-27b&provider_type=openai",
    ),
  );
});

test("a fetch that answers after the model row was removed does not bring it back", async () => {
  let settle: () => void = () => {};
  const fetchMock = vi.fn(async (input: unknown) => {
    const url = String(input);
    if (url.startsWith("/foxxycode/config/reasoning-levels")) {
      await new Promise<void>((r) => {
        settle = r;
      });
      return {
        ok: true,
        json: async () => ({
          ok: true,
          levels: ["low", "medium", "high"],
          detected: true,
        }),
      };
    }
    return {
      ok: true,
      json: async () => ({ ok: true, models: [{ id: "qwen3.8-27b" }] }),
    };
  });
  vi.stubGlobal("fetch", fetchMock);

  render(<ReasoningModelsHarness />);
  await addFetchedModel();
  fireEvent.click(screen.getByTestId("reasoning-levels-fetch"));

  // Back to the list, delete the row while the request is still in flight.
  fireEvent.click(screen.getByTestId("settings-detail-back"));
  fireEvent.click(
    screen.getByRole("button", { name: /Remove valera\/qwen3\.8-27b/ }),
  );
  expect(savedModels()).toEqual([]);

  settle();
  await new Promise((r) => setTimeout(r, 0));
  // The stale answer must not re-create the deleted entry through the
  // captured array callback.
  expect(savedModels()).toEqual([]);
});

test("a fetch that answers after a sibling field changed keeps that change", async () => {
  let settle: () => void = () => {};
  const fetchMock = vi.fn(async (input: unknown) => {
    const url = String(input);
    if (url.startsWith("/foxxycode/config/reasoning-levels")) {
      await new Promise<void>((r) => {
        settle = r;
      });
      return {
        ok: true,
        json: async () => ({
          ok: true,
          levels: ["low", "medium", "high"],
          detected: true,
        }),
      };
    }
    return {
      ok: true,
      json: async () => ({ ok: true, models: [{ id: "qwen3.8-27b" }] }),
    };
  });
  vi.stubGlobal("fetch", fetchMock);

  render(<ReasoningModelsHarness />);
  await addFetchedModel();
  fireEvent.click(screen.getByTestId("reasoning-levels-fetch"));

  // While the request is in flight the operator turns streaming off.
  fireEvent.click(screen.getByRole("switch", { name: /Stream responses/ }));
  expect(savedModels()).toEqual([
    { model: "valera/qwen3.8-27b", multimodal: false, stream: false },
  ]);

  settle();
  await waitFor(() =>
    expect(
      (savedModels() as Array<Record<string, unknown>>)[0]?.[
        "reasoning_levels"
      ],
    ).toEqual(["low", "medium", "high"]),
  );
  // The answer must land on the entry as it is now, not on the snapshot the
  // form held when Fetch was pressed - that snapshot still has stream: true.
  expect(savedModels()).toEqual([
    {
      model: "valera/qwen3.8-27b",
      multimodal: false,
      stream: false,
      reasoning_levels: ["low", "medium", "high"],
    },
  ]);
});

test("a model id with no reasoning family is left without an override", async () => {
  stubModelsAndLevels([]);

  render(<ReasoningModelsHarness />);
  await addFetchedModel();

  fireEvent.click(screen.getByTestId("reasoning-levels-fetch"));
  await waitFor(() =>
    expect(screen.getByTestId("reasoning-levels-status").textContent).toContain(
      "no auto-detected reasoning levels",
    ),
  );
  // An empty list here would hide the composer's reasoning selector for good.
  expect(savedModels()).toEqual([
    { model: "valera/qwen3.8-27b", multimodal: false, stream: true },
  ]);
});
