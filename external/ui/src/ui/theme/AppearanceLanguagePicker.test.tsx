import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { AppearanceLanguagePicker } from "./AppearanceModal";
import { I18nProvider } from "../i18n/I18nProvider";
import { initLocale } from "../i18n/i18n";
import { UI_LOCALE_IDS } from "../i18n/locales";
import { FOXXYCODE_UI_LANG_COOKIE } from "../i18n/localeCookie";

function configDoc(locale: string): Record<string, unknown> {
  return { ui: { locale }, providers: [] };
}

beforeEach(() => {
  document.cookie = `${FOXXYCODE_UI_LANG_COOKIE}=; Path=/; Max-Age=0`;
  document.documentElement.lang = "en";
  initLocale("en");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  initLocale("en");
});

test("renders one select with Auto and every registered locale", async () => {
  render(
    <I18nProvider>
      <AppearanceLanguagePicker doc={configDoc("")} setDoc={() => {}} />
    </I18nProvider>,
  );

  const select = screen.getByRole("combobox", {
    name: "Language",
  }) as HTMLSelectElement;
  expect(select).toBe(screen.getByTestId("appearance-language-select"));
  // Derived from the registry, so adding a locale cannot silently miss the picker.
  expect([...select.options].map((o) => o.value)).toEqual([
    "auto",
    ...UI_LOCALE_IDS,
  ]);
  expect(select.value).toBe("auto");
});

test("an empty ui.locale shows Auto, an explicit one shows that locale", () => {
  const { rerender } = render(
    <I18nProvider>
      <AppearanceLanguagePicker doc={configDoc("ru")} setDoc={() => {}} />
    </I18nProvider>,
  );
  expect(
    (screen.getByTestId("appearance-language-select") as HTMLSelectElement).value,
  ).toBe("ru");

  rerender(
    <I18nProvider>
      <AppearanceLanguagePicker doc={configDoc("")} setDoc={() => {}} />
    </I18nProvider>,
  );
  expect(
    (screen.getByTestId("appearance-language-select") as HTMLSelectElement).value,
  ).toBe("auto");
});

test("picking Русский persists ui.locale to config and switches the labels", async () => {
  // The picker persists through GET+PUT /foxxycode/config; the loaded doc is
  // mirrored back through setDoc so a later footer Save keeps the choice.
  const fetchMock = vi.fn(async (input: unknown, init?: RequestInit) => {
    if (init?.method === "PUT") {
      return new Response("{}", { status: 200 });
    }
    return new Response(JSON.stringify(configDoc("")), { status: 200 });
  });
  vi.stubGlobal("fetch", fetchMock);
  const setDoc = vi.fn();

  render(
    <I18nProvider>
      <AppearanceLanguagePicker doc={configDoc("")} setDoc={setDoc} />
    </I18nProvider>,
  );

  fireEvent.change(screen.getByTestId("appearance-language-select"), {
    target: { value: "ru" },
  });

  await waitFor(() => expect(document.documentElement.lang).toBe("ru"));
  expect(screen.getByText("Язык")).toBeTruthy();
  expect(setDoc).toHaveBeenCalledWith(
    expect.objectContaining({ ui: expect.objectContaining({ locale: "ru" }) }),
  );

  await waitFor(() => {
    const put = fetchMock.mock.calls.find((c) => (c[1] as RequestInit)?.method === "PUT");
    expect(put).toBeTruthy();
    expect(JSON.parse((put![1] as RequestInit).body as string).ui.locale).toBe("ru");
  });
});

test("picking Auto stores an empty preference and drops the language cookie", async () => {
  const fetchMock = vi.fn(async (input: unknown, init?: RequestInit) => {
    if (init?.method === "PUT") {
      return new Response("{}", { status: 200 });
    }
    return new Response(JSON.stringify(configDoc("ru")), { status: 200 });
  });
  vi.stubGlobal("fetch", fetchMock);
  const setDoc = vi.fn();

  render(
    <I18nProvider>
      <AppearanceLanguagePicker doc={configDoc("ru")} setDoc={setDoc} />
    </I18nProvider>,
  );

  fireEvent.change(screen.getByTestId("appearance-language-select"), {
    target: { value: "auto" },
  });

  expect(setDoc).toHaveBeenCalledWith(
    expect.objectContaining({ ui: expect.objectContaining({ locale: "" }) }),
  );
  // Auto must not leave the resolved language pinned in the cookie, or the next
  // reload would bootstrap it instead of following the system.
  expect(document.cookie).not.toContain(`${FOXXYCODE_UI_LANG_COOKIE}=ru`);

  await waitFor(() => {
    const put = fetchMock.mock.calls.find((c) => (c[1] as RequestInit)?.method === "PUT");
    expect(put).toBeTruthy();
    expect(JSON.parse((put![1] as RequestInit).body as string).ui.locale).toBe("");
  });
});
