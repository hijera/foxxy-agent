import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { AppearanceThemePicker } from "./AppearanceModal";
import { I18nProvider } from "../i18n/I18nProvider";
import { initLocale } from "../i18n/i18n";
import { FOXXYCODE_UI_EFFECTS_COOKIE, bootstrapUiEffects } from "./uiEffects";

function renderPicker() {
  return render(
    <I18nProvider>
      <AppearanceThemePicker doc={{ ui: { locale: "" } }} setDoc={() => {}} />
    </I18nProvider>,
  );
}

beforeEach(() => {
  initLocale("en");
});

afterEach(() => {
  cleanup();
  document.cookie = `${FOXXYCODE_UI_EFFECTS_COOKIE}=; Max-Age=0; Path=/`;
  document.documentElement.removeAttribute("data-effects");
  delete document.documentElement.dataset.embed;
  window.sessionStorage.clear();
});

test("the IntelliJ panel shows the effects switch, off by default", () => {
  document.documentElement.dataset.embed = "intellij";
  bootstrapUiEffects();
  renderPicker();

  const sw = screen.getByTestId("appearance-effects-switch");
  expect(sw.getAttribute("role")).toBe("switch");
  expect(sw.getAttribute("aria-checked")).toBe("false");
  expect(screen.getByText("Animations and translucency")).toBeTruthy();
});

test("turning the switch on brings the effects back at once and remembers it", () => {
  document.documentElement.dataset.embed = "intellij";
  bootstrapUiEffects();
  renderPicker();

  fireEvent.click(screen.getByTestId("appearance-effects-switch"));
  expect(document.documentElement.dataset.effects).toBe("full");
  expect(document.cookie).toContain(`${FOXXYCODE_UI_EFFECTS_COOKIE}=full`);
  expect(screen.getByTestId("appearance-effects-switch").getAttribute("aria-checked")).toBe(
    "true",
  );

  fireEvent.click(screen.getByTestId("appearance-effects-switch"));
  expect(document.documentElement.dataset.effects).toBe("reduced");
  expect(document.cookie).toContain(`${FOXXYCODE_UI_EFFECTS_COOKIE}=reduced`);
});

test("the switch saves the choice to config.yaml and into the settings document", async () => {
  const puts: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: string, init?: RequestInit) => {
      if (init?.method === "PUT") {
        puts.push(JSON.parse(String(init.body)));
        return new Response("{}");
      }
      return new Response(JSON.stringify({ ui: { locale: "" } }));
    }),
  );
  const setDoc = vi.fn();
  document.documentElement.dataset.embed = "intellij";
  bootstrapUiEffects();
  render(
    <I18nProvider>
      <AppearanceThemePicker doc={{ ui: { locale: "" } }} setDoc={setDoc} />
    </I18nProvider>,
  );

  fireEvent.click(screen.getByTestId("appearance-effects-switch"));
  // The footer Save sends the whole settings document; mirroring the pick there
  // keeps it from writing the old value back.
  expect(setDoc).toHaveBeenCalledWith({ ui: { locale: "", effects: true } });
  await waitFor(() => expect(puts).toEqual([{ ui: { locale: "", effects: true } }]));
  vi.unstubAllGlobals();
});

// The same switch everywhere; only its starting position differs by host.
test("in the browser and VS Code the switch is there too, on by default", () => {
  bootstrapUiEffects();
  renderPicker();
  expect(document.documentElement.dataset.effects).toBe("full");
  expect(screen.getByTestId("appearance-effects-switch").getAttribute("aria-checked")).toBe("true");

  cleanup();
  document.documentElement.dataset.embed = "vscode";
  window.sessionStorage.clear();
  bootstrapUiEffects();
  renderPicker();
  expect(screen.getByTestId("appearance-effects-switch").getAttribute("aria-checked")).toBe("true");
});
