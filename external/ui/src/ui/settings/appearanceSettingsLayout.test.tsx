import { afterEach, expect, test, vi } from "vitest";
import { cleanup, render, waitFor } from "@testing-library/react";
import { Settings } from "./Settings";

afterEach(() => {
  cleanup();
  // Only clear stubbed globals (fetch); restoreAllMocks would also reset the
  // shared window.matchMedia mock from vitest.setup.ts and break later renders.
  vi.unstubAllGlobals();
});

// Regression: while the config schema is still loading (fetch pending, no error),
// the Appearance section rendered inside `.settings-scroll-placeholder` — a
// `display:flex; align-items:center; justify-content:center` box meant for the
// "Loading…" spinner. That shrank the theme swatch grid to its content width and
// off-centered it, so the swatches looked crookedly placed. Appearance is real
// (client-side) content and must render in the normal scroll flow so the grid
// fills the panel width.
test("appearance renders outside the centered loading placeholder", async () => {
  // Never-resolving fetch keeps schema null and loadErr null: the loading window.
  vi.stubGlobal(
    "fetch",
    vi.fn(() => new Promise<Response>(() => {})),
  );

  const { container } = render(
    <Settings onClose={() => {}} initialSection="appearance" />,
  );

  await waitFor(() =>
    expect(container.querySelector(".appearance-swatch-grid")).toBeTruthy(),
  );
  // The language picker sits in Appearance, directly under the theme grid.
  expect(
    container.querySelector('[data-testid="appearance-language-picker"]'),
  ).toBeTruthy();

  // The swatch grid must not be nested inside the centered placeholder box.
  expect(
    container.querySelector(
      ".settings-scroll-placeholder .appearance-swatch-grid",
    ),
  ).toBeNull();
  expect(
    container.querySelector(
      '.settings-scroll-placeholder [data-testid="appearance-language-picker"]',
    ),
  ).toBeNull();
});

test("general renders outside the centered loading placeholder", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => new Promise<Response>(() => {})),
  );

  const { container } = render(
    <Settings onClose={() => {}} initialSection="general" />,
  );

  await waitFor(() =>
    expect(
      container.querySelector('[data-testid="general-send-mode-picker"]'),
    ).toBeTruthy(),
  );
  // General keeps the send-mode and status-line pickers; language is in Appearance.
  expect(
    container.querySelector('[data-testid="appearance-language-picker"]'),
  ).toBeNull();
  expect(
    container.querySelector(
      '.settings-scroll-placeholder [data-testid="general-send-mode-picker"]',
    ),
  ).toBeNull();
});
