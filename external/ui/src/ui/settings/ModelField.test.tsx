import React from "react";
import { afterEach, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { ModelField } from "./ModelField";
import type { FetchedModel } from "./useProviderModels";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function Harness({
  providers = ["openai"],
  syncsMultimodal,
  onPick,
}: {
  providers?: string[];
  syncsMultimodal?: boolean;
  onPick?: (v: string, picked: FetchedModel | undefined) => void;
}) {
  const [val, setVal] = React.useState("");
  return (
    <>
      <ModelField
        value={val}
        onChange={(v, picked) => {
          setVal(v);
          onPick?.(v, picked);
        }}
        providers={providers}
        syncsMultimodal={syncsMultimodal}
      />
      <span data-testid="val">{val}</span>
    </>
  );
}

test("fetch populates the model combobox and picking writes provider/id", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue({
    ok: true,
    json: async () => ({ ok: true, models: [{ id: "gpt-4o" }, { id: "gpt-4o-mini" }] }),
  } as unknown as Response);

  render(<Harness />);
  fireEvent.click(screen.getByTestId("model-field-fetch"));
  await waitFor(() =>
    expect(screen.getByTestId("model-field-fetch").textContent).toBe("Fetch models"),
  );

  // Open the model combobox and pick the fetched option.
  fireEvent.focus(screen.getByTestId("model-field-model"));
  const opt = await screen.findByText("openai/gpt-4o-mini");
  fireEvent.mouseDown(opt);
  expect(screen.getByTestId("val").textContent).toBe("openai/gpt-4o-mini");
});

test("fetch failure falls back to manual typing in the combobox", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue({
    ok: true,
    json: async () => ({ ok: false, error: "bad key", models: [] }),
  } as unknown as Response);

  render(<Harness />);
  fireEvent.click(screen.getByTestId("model-field-fetch"));
  await screen.findByText(/Couldn't fetch models/);

  fireEvent.change(screen.getByTestId("model-field-model"), {
    target: { value: "openai/custom" },
  });
  expect(screen.getByTestId("val").textContent).toBe("openai/custom");
});

// The provider catalog advertises image input per model (capabilities.vision /
// modalities.input). Surfacing it in the option label is what stops an operator
// from adding a vision model and leaving models[].multimodal at false.
test("fetched models that accept images are badged in the dropdown", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue({
    ok: true,
    json: async () => ({
      ok: true,
      models: [
        { id: "qwen3.6-35b-a3b", vision: true },
        { id: "gpt-oss-20b" },
      ],
    }),
  } as unknown as Response);

  render(<Harness providers={["neuraldeep"]} />);
  fireEvent.click(screen.getByTestId("model-field-fetch"));
  await waitFor(() =>
    expect(screen.getByTestId("model-field-fetch").textContent).toBe("Fetch models"),
  );

  fireEvent.focus(screen.getByTestId("model-field-model"));
  const vision = await screen.findByText(/neuraldeep\/qwen3\.6-35b-a3b/);
  expect(vision.textContent).toMatch(/vision/i);
  const textOnly = screen.getByText(/neuraldeep\/gpt-oss-20b/);
  expect(textOnly.textContent).not.toMatch(/vision/i);
});

test("picking a listed model reports its catalog entry so siblings can follow", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue({
    ok: true,
    json: async () => ({
      ok: true,
      models: [{ id: "qwen3.6-35b-a3b", vision: true }, { id: "gpt-oss-20b" }],
    }),
  } as unknown as Response);

  const picks: [string, FetchedModel | undefined][] = [];
  render(
    <Harness
      providers={["neuraldeep"]}
      onPick={(v, picked) => picks.push([v, picked])}
    />,
  );
  fireEvent.click(screen.getByTestId("model-field-fetch"));
  await waitFor(() =>
    expect(screen.getByTestId("model-field-fetch").textContent).toBe("Fetch models"),
  );

  fireEvent.focus(screen.getByTestId("model-field-model"));
  fireEvent.mouseDown(await screen.findByText(/neuraldeep\/qwen3\.6-35b-a3b/));
  expect(picks.at(-1)).toEqual([
    "neuraldeep/qwen3.6-35b-a3b",
    { id: "qwen3.6-35b-a3b", vision: true },
  ]);

  // A hand-typed id is not in the catalog: no entry, so nothing is inferred.
  fireEvent.change(screen.getByTestId("model-field-model"), {
    target: { value: "neuraldeep/typed-by-hand" },
  });
  expect(picks.at(-1)).toEqual(["neuraldeep/typed-by-hand", undefined]);
});

// The hub's own flag is not authoritative (gpt-oss-120b advertises vision:false and
// still reads images), so the note has to say the value came from the catalog and
// can be overridden - a silent toggle would look like a bug.
test("a synced field explains that multimodal came from the catalog", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue({
    ok: true,
    json: async () => ({
      ok: true,
      models: [{ id: "qwen3.6-35b-a3b", vision: true }, { id: "gpt-oss-20b" }],
    }),
  } as unknown as Response);

  render(<Harness providers={["neuraldeep"]} syncsMultimodal />);
  fireEvent.click(screen.getByTestId("model-field-fetch"));
  await waitFor(() =>
    expect(screen.getByTestId("model-field-fetch").textContent).toBe("Fetch models"),
  );
  expect(screen.queryByTestId("model-field-multimodal-note")).toBeNull();

  fireEvent.focus(screen.getByTestId("model-field-model"));
  fireEvent.mouseDown(await screen.findByText(/neuraldeep\/qwen3\.6-35b-a3b/));
  expect(
    (await screen.findByTestId("model-field-multimodal-note")).textContent,
  ).toMatch(/on/i);

  fireEvent.focus(screen.getByTestId("model-field-model"));
  fireEvent.mouseDown(await screen.findByText(/neuraldeep\/gpt-oss-20b/));
  expect(
    screen.getByTestId("model-field-multimodal-note").textContent,
  ).toMatch(/off/i);

  // Typing an id the catalog does not list leaves the flag alone - no note.
  fireEvent.change(screen.getByTestId("model-field-model"), {
    target: { value: "neuraldeep/typed-by-hand" },
  });
  expect(screen.queryByTestId("model-field-multimodal-note")).toBeNull();
});
