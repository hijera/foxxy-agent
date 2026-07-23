import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import React from "react";
import { EnvironmentChip } from "./EnvironmentChip";
import { initLocale, setLocale } from "../i18n/i18n";

// The chip reads the local config for configured remotes; an empty answer keeps the menu at
// "Local" + the add form, which is the state this test is about.
function stubConfigFetch() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => ({
      ok: true,
      json: async () => ({}),
    })) as unknown as typeof fetch,
  );
}

beforeEach(() => {
  stubConfigFetch();
  initLocale("en");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  setLocale("en");
});

describe("EnvironmentChip", () => {
  it("renders the menu in English by default", () => {
    render(<EnvironmentChip />);
    fireEvent.click(screen.getByTestId("composer-env-btn"));
    const menu = screen.getByTestId("composer-env-menu");
    expect(menu.textContent).toContain("Local (this origin)");
    expect(menu.textContent).toContain("+ Add remote…");
  });

  it("renders the chip and menu in Russian", () => {
    setLocale("ru");
    render(<EnvironmentChip />);
    const btn = screen.getByTestId("composer-env-btn");
    expect(btn.getAttribute("aria-label")).toBe("Окружение");
    expect(btn.textContent).toContain("Локальное");

    fireEvent.click(btn);
    const menu = screen.getByTestId("composer-env-menu");
    expect(menu.textContent).toContain("Локальное (этот origin)");
    expect(menu.textContent).toContain("+ Добавить удалённое…");
    // No English leaks left in the menu (the tail this change closes).
    expect(menu.textContent).not.toContain("Local (this origin)");
    expect(menu.textContent).not.toContain("Add remote");
  });

  it("localizes the add-remote form without changing its test ids", () => {
    setLocale("ru");
    render(<EnvironmentChip />);
    fireEvent.click(screen.getByTestId("composer-env-btn"));
    fireEvent.click(screen.getByTestId("composer-env-add"));

    const menu = screen.getByTestId("composer-env-menu");
    expect(menu.textContent).toContain("Добавить удалённое");
    expect(menu.textContent).toContain("Подключить");
    expect(menu.textContent).toContain("Отмена");
    // The URL field keeps its literal example: the same syntax in every locale.
    expect(
      screen.getByTestId("composer-env-add-url").getAttribute("placeholder"),
    ).toBe("https://box.example:12345");
  });
});
