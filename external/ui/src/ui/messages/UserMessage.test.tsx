import React from "react";
import { afterEach } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import { UserMessage } from "./UserMessage";

afterEach(() => cleanup());

test("user bubble preserves multiline text without markdown pipeline", () => {
  const yaml = [
    "---",
    "services:",
    "  qbittorrent:",
    "    volumes:",
    "      - /path/to/downloads:/downloads",
  ].join("\n");
  render(<UserMessage content={yaml} />);
  const body = screen.getByTestId("user-message-body");
  expect(body).toHaveTextContent("services:");
  expect(body).toHaveTextContent("/path/to/downloads:/downloads");
  expect(body.textContent).toBe(yaml);
  expect(screen.queryByTestId("coddy-skill-span")).toBeNull();
});

test("user bubble does not treat path slashes as skill chips", () => {
  render(<UserMessage content="hi /demo there" />);
  expect(screen.getByTestId("user-message-body")).toHaveTextContent(
    "hi /demo there",
  );
  expect(screen.queryByTestId("coddy-skill-span")).toBeNull();
});

test("copy sends raw user text not display-only slash chip source", () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(globalThis.navigator, "clipboard", {
    value: { writeText },
    configurable: true,
    writable: true,
  });
  render(<UserMessage content="hi /demo there" />);
  const copyBtn = screen.getByTestId("user-message-copy");
  expect(copyBtn).toHaveAttribute("title", "Copy message");
  copyBtn.click();
  expect(writeText).toHaveBeenCalledWith("hi /demo there");
});

test("persisted hydrated attachments render as compact @ paths", () => {
  const blob =
    "read this\n\n" +
    '<coddy_attachment path="note.txt" name="note.txt">\n' +
    "<![CDATA[secret body]]>\n" +
    "</coddy_attachment>";
  render(<UserMessage content={blob} />);
  expect(screen.getByText(/read this/)).toBeInTheDocument();
  expect(screen.getByText(/@note\.txt/)).toBeInTheDocument();
  expect(screen.queryByText(/secret body/)).toBeNull();
});
