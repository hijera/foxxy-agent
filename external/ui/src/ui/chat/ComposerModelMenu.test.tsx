import React from "react";
import { afterEach, expect, test, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { Composer } from "./Composer";

afterEach(() => cleanup());

const MANY_MODELS = [
  "opencode-go/deepseek-v4-pro",
  "opencode-go/deepseek-v4-flash",
  "opencode-go/kimi-k2.7-code",
  "opencode-go/kimi-k2.6",
  "opencode-go/glm-5.1",
  "opencode-go/glm-5",
  "opencode-go/mimo-v2.5-pro",
  "opencode-go/mimo-v2.5",
  "opencode-go/minimax-m3",
  "opencode-go/minimax-m2.7",
  "cliproxyapi/gpt-5.5",
  "cliproxyapi/gpt-5.4",
  "cliproxyapi/gpt-5.4-mini",
  "cliproxyapi/gemini-3.1-pro-preview",
  "cliproxyapi/gemini-2.5-pro",
];

function renderModelMenu(opts: {
  models?: string[];
  model?: string;
  onChange?: (id: string) => void;
}) {
  return render(
    <Composer
      value=""
      isEmpty={false}
      mode="agent"
      modes={["agent", "plan"]}
      llmModels={opts.models ?? MANY_MODELS}
      llmModel={opts.model ?? MANY_MODELS[0]!}
      onLlmModelChange={opts.onChange ?? (() => {})}
      onModeChange={() => {}}
      onChange={() => {}}
      onSend={() => {}}
    />,
  );
}

function openModelMenu() {
  fireEvent.click(screen.getByRole("button", { name: "Model" }));
}

test("model menu shows a filter input when there are more than 5 models", () => {
  renderModelMenu({});
  openModelMenu();
  expect(screen.getByTestId("model-menu-filter")).toBeTruthy();
});

test("model menu omits the filter input for a short list", () => {
  renderModelMenu({
    models: ["opencode-go/glm-5", "cliproxyapi/gpt-5.5"],
    model: "opencode-go/glm-5",
  });
  openModelMenu();
  expect(screen.queryByTestId("model-menu-filter")).toBeNull();
});

test("exactly 5 models still omits the filter but keeps grouping", () => {
  renderModelMenu({
    models: [
      "opencode-go/glm-5",
      "opencode-go/glm-5.1",
      "opencode-go/kimi-k2.6",
      "cliproxyapi/gpt-5.5",
      "cliproxyapi/gpt-5.4",
    ],
    model: "opencode-go/glm-5",
  });
  openModelMenu();
  expect(screen.queryByTestId("model-menu-filter")).toBeNull();
  expect(screen.getByText("opencode-go")).toBeTruthy();
  expect(screen.getByText("cliproxyapi")).toBeTruthy();
});

test("6 models cross the threshold and reveal the filter", () => {
  renderModelMenu({
    models: [
      "opencode-go/glm-5",
      "opencode-go/glm-5.1",
      "opencode-go/kimi-k2.6",
      "cliproxyapi/gpt-5.5",
      "cliproxyapi/gpt-5.4",
      "cliproxyapi/gpt-5.4-mini",
    ],
    model: "opencode-go/glm-5",
  });
  openModelMenu();
  expect(screen.getByTestId("model-menu-filter")).toBeTruthy();
});

test("desktop shell renders the menu as an anchored dropdown (not a sheet)", () => {
  renderModelMenu({});
  openModelMenu();
  const menu = screen.getByRole("menu");
  expect(menu.className).toContain("mode-menu--portal");
  expect(menu.className).not.toContain("mode-menu--sheet");
});

test("narrow shell renders the model menu as a full-width sheet", () => {
  const original = window.matchMedia;
  // Force the mobile/narrow shell breakpoint (max-width: 1199px) to match.
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches: true,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })) as unknown as typeof window.matchMedia;
  try {
    renderModelMenu({});
    openModelMenu();
    const menu = screen.getByRole("menu");
    expect(menu.className).toContain("mode-menu--sheet");
    expect(menu.className).not.toContain("mode-menu--portal");
    // Filter still applies on mobile (15 > 5).
    expect(screen.getByTestId("model-menu-filter")).toBeTruthy();
  } finally {
    window.matchMedia = original;
  }
});

test("model menu groups rows under vendor headers when several vendors exist", () => {
  renderModelMenu({});
  openModelMenu();
  expect(screen.getByText("opencode-go")).toBeTruthy();
  expect(screen.getByText("cliproxyapi")).toBeTruthy();
});

test("typing in the filter narrows the listed models", () => {
  renderModelMenu({});
  openModelMenu();
  fireEvent.change(screen.getByTestId("model-menu-filter"), {
    target: { value: "gemini" },
  });
  expect(screen.getByRole("menuitem", { name: "gemini-2.5-pro" })).toBeTruthy();
  expect(
    screen.queryByRole("menuitem", { name: "deepseek-v4-pro" }),
  ).toBeNull();
  // The non-matching vendor header is gone too.
  expect(screen.queryByText("opencode-go")).toBeNull();
});

test("filtering by vendor keeps that vendor's rows", () => {
  renderModelMenu({});
  openModelMenu();
  fireEvent.change(screen.getByTestId("model-menu-filter"), {
    target: { value: "cliproxyapi" },
  });
  const menu = screen.getByRole("menu");
  expect(within(menu).getByRole("menuitem", { name: "gpt-5.5" })).toBeTruthy();
  expect(within(menu).queryByRole("menuitem", { name: "glm-5" })).toBeNull();
});

test("selecting a filtered model calls onLlmModelChange with the full id", () => {
  const onChange = vi.fn();
  renderModelMenu({ onChange });
  openModelMenu();
  fireEvent.change(screen.getByTestId("model-menu-filter"), {
    target: { value: "5.4-mini" },
  });
  fireEvent.click(screen.getByRole("menuitem", { name: "gpt-5.4-mini" }));
  expect(onChange).toHaveBeenCalledWith("cliproxyapi/gpt-5.4-mini");
});

test("empty filter result shows a no-models notice", () => {
  renderModelMenu({});
  openModelMenu();
  fireEvent.change(screen.getByTestId("model-menu-filter"), {
    target: { value: "no-such-model" },
  });
  expect(screen.getByTestId("model-menu-empty")).toBeTruthy();
});

// A menu of the composer answers Escape itself: a short list has no filter to
// take the key, and a long one closes the same way. The key is claimed, so a
// screen of the rail under the chat would stay.
test("Escape closes the model menu, with a filter or without one", () => {
  renderModelMenu({
    models: ["opencode-go/glm-5", "cliproxyapi/gpt-5.5"],
    model: "opencode-go/glm-5",
  });
  openModelMenu();
  expect(screen.getByRole("menu")).toBeTruthy();
  expect(fireEvent.keyDown(document.body, { key: "Escape" })).toBe(false);
  expect(screen.queryByRole("menu")).toBeNull();
  cleanup();

  renderModelMenu({});
  openModelMenu();
  fireEvent.keyDown(screen.getByTestId("model-menu-filter"), { key: "Escape" });
  expect(screen.queryByRole("menu")).toBeNull();
});

// Models added to a provider later are appended to models[] and used to land at
// the bottom of their group, below names that sort after them. Within a vendor
// the menu reads alphabetically, versions compared as numbers; the vendors keep
// the order of the configuration.
test("model menu lists the models of each vendor alphabetically, versions as numbers", () => {
  renderModelMenu({
    models: [
      "codex/gpt-6-astra",
      "codex/gpt-5.6-sol",
      "codex/gpt-5.6-terra",
      "codex/gpt-5.6-luna",
      "codex/gpt-6-luna",
      "codex/gpt-6-sol",
      "codex/gpt-5.10-mini",
      "neuraldeep/qwen3.8-27b",
      "neuraldeep/gpt-oss-120b",
    ],
    model: "codex/gpt-6-astra",
  });
  openModelMenu();
  const menu = screen.getByRole("menu");
  const rows = within(menu)
    .getAllByRole("menuitem")
    .map((el) => el.textContent);
  expect(rows).toEqual([
    "gpt-5.6-luna",
    "gpt-5.6-sol",
    "gpt-5.6-terra",
    "gpt-5.10-mini",
    "gpt-6-astra",
    "gpt-6-luna",
    "gpt-6-sol",
    "gpt-oss-120b",
    "qwen3.8-27b",
  ]);
  const labels = within(menu)
    .getAllByText(/^(codex|neuraldeep)$/)
    .map((el) => el.textContent);
  expect(labels).toEqual(["codex", "neuraldeep"]);
});

// Enter takes the first row the reader sees, so the order is the menu's order.
test("Enter in the filter picks the first model as the menu orders it", () => {
  const onChange = vi.fn();
  renderModelMenu({
    models: [
      "codex/gpt-6-sol",
      "codex/gpt-6-astra",
      "codex/gpt-5.6-luna",
      "stub/a",
      "stub/b",
      "stub/c",
    ],
    model: "stub/a",
    onChange,
  });
  openModelMenu();
  const filter = screen.getByTestId("model-menu-filter");
  fireEvent.change(filter, { target: { value: "gpt-6" } });
  fireEvent.keyDown(filter, { key: "Enter" });
  expect(onChange).toHaveBeenCalledWith("codex/gpt-6-astra");
});
