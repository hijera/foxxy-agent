import { afterEach, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, waitFor } from "@testing-library/react";
import { Settings } from "./Settings";

afterEach(() => {
  cleanup();
  // Only clear stubbed globals (fetch); restoreAllMocks would also reset the
  // shared window.matchMedia mock from vitest.setup.ts and break later renders.
  vi.unstubAllGlobals();
});

// Keep the config schema loading (never-resolving fetch) so the Appearance tab
// renders its client-side content without a backend.
function stubPendingFetch() {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => new Promise<Response>(() => {})),
  );
}

test("Appearance tab shows the restart button and invokes the callback", async () => {
  stubPendingFetch();
  const onRestartOnboarding = vi.fn();
  const { getByTestId } = render(
    <Settings
      onClose={() => {}}
      initialSection="appearance"
      onRestartOnboarding={onRestartOnboarding}
    />,
  );

  await waitFor(() =>
    expect(getByTestId("settings-restart-onboarding")).toBeTruthy(),
  );
  fireEvent.click(getByTestId("settings-restart-onboarding"));
  expect(onRestartOnboarding).toHaveBeenCalledTimes(1);
});

test("restart button is absent when no callback is provided", async () => {
  stubPendingFetch();
  const { container, queryByTestId } = render(
    <Settings onClose={() => {}} initialSection="appearance" />,
  );

  await waitFor(() =>
    expect(container.querySelector(".appearance-swatch-grid")).toBeTruthy(),
  );
  expect(queryByTestId("settings-restart-onboarding")).toBeNull();
});
