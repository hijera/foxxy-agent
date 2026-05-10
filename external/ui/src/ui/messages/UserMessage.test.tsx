import React from "react";
import { afterEach } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { expect, test } from "vitest";
import { UserMessage } from "./UserMessage";

afterEach(() => cleanup());

test("user bubble chips plain slash commands for Markdown", () => {
  render(<UserMessage content="hi /demo there" />);
  expect(screen.getByTestId("coddy-skill-span")).toHaveTextContent("/demo");
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
