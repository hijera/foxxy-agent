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

  // The menu opens from a click, so the focus stays on the chip: Escape has
  // to reach the menu from there too.
  it("closes on Escape with the focus still on the chip", async () => {
    render(<EnvironmentChip />);
    const btn = screen.getByTestId("composer-env-btn");
    btn.getBoundingClientRect = () => new DOMRect(0, 0, 100, 48);
    fireEvent.click(btn);
    await screen.findByTestId("composer-env-menu");
    expect(fireEvent.keyDown(btn, { key: "Escape" })).toBe(false);
    expect(screen.queryByTestId("composer-env-menu")).toBeNull();
  });

  // In the swarm header of a relay the chip is the last thing on the right, so
  // a menu hung from its left edge ran past the window at 1280 px.
  it("stays inside the window when the chip is near the right edge", async () => {
    render(<EnvironmentChip />);
    const btn = screen.getByTestId("composer-env-btn");
    const right = window.innerWidth - 80;
    btn.getBoundingClientRect = () =>
      new DOMRect(right - 76, 0, 76, 48);
    fireEvent.click(btn);
    const menu = await screen.findByTestId("composer-env-menu");
    const width = Math.min(300, window.innerWidth - 24);
    const left = parseFloat(menu.style.left);
    expect(left + width).toBeLessThanOrEqual(window.innerWidth - 12);
    expect(left).toBeGreaterThanOrEqual(12);
  });

  // A menu that opens downward carries only the downward class: with both
  // classes the CSS sets top and bottom on the fixed menu, and its height
  // collapses to zero.
  it("opens downward and upward with one direction class each", async () => {
    render(<EnvironmentChip />);
    const btn = screen.getByTestId("composer-env-btn");

    btn.getBoundingClientRect = () => new DOMRect(0, 0, 100, 48);
    fireEvent.click(btn);
    const down = await screen.findByTestId("composer-env-menu");
    expect(down.classList.contains("opens-down")).toBe(true);
    expect(down.classList.contains("opens-up")).toBe(false);
    fireEvent.click(btn);

    btn.getBoundingClientRect = () =>
      new DOMRect(0, window.innerHeight - 60, 100, 48);
    fireEvent.click(btn);
    const up = await screen.findByTestId("composer-env-menu");
    expect(up.classList.contains("opens-up")).toBe(true);
    expect(up.classList.contains("opens-down")).toBe(false);
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
