import React from "react";
import { afterEach, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { SessionExportMenu } from "./SessionExportMenu";

afterEach(() => cleanup());

test("the dropdown stays closed until the toggle is pressed", () => {
  render(<SessionExportMenu onExport={() => {}} />);

  expect(screen.queryByRole("menu")).toBeNull();

  fireEvent.click(screen.getByTestId("session-export-toggle"));

  expect(screen.getByRole("menu")).toBeTruthy();
  expect(screen.getAllByRole("menuitem")).toHaveLength(4);
});

test("picking a format reports it once and closes the dropdown", () => {
  const onExport = vi.fn();
  render(<SessionExportMenu onExport={onExport} />);

  fireEvent.click(screen.getByTestId("session-export-toggle"));
  fireEvent.click(screen.getByTestId("session-export-docx"));

  expect(onExport).toHaveBeenCalledTimes(1);
  expect(onExport).toHaveBeenCalledWith("docx");
  expect(screen.queryByRole("menu")).toBeNull();
});

test("Escape closes the dropdown without exporting", () => {
  const onExport = vi.fn();
  render(<SessionExportMenu onExport={onExport} />);

  fireEvent.click(screen.getByTestId("session-export-toggle"));
  fireEvent.keyDown(document, { key: "Escape" });

  expect(screen.queryByRole("menu")).toBeNull();
  expect(onExport).not.toHaveBeenCalled();
});

test("a running export disables the toggle so a second one cannot start", () => {
  const onExport = vi.fn();
  render(<SessionExportMenu onExport={onExport} busy />);

  const toggle = screen.getByTestId("session-export-toggle");
  expect(toggle).toBeDisabled();

  fireEvent.click(toggle);

  expect(screen.queryByRole("menu")).toBeNull();
  expect(onExport).not.toHaveBeenCalled();
});
