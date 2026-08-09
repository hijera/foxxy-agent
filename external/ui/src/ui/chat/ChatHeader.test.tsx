import React from "react";
import { afterEach, expect, test } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { ChatHeader } from "./ChatHeader";

afterEach(() => cleanup());

test("edit mode shows full-width title input class", () => {
  render(<ChatHeader title="Hello" editable onTitleSave={() => {}} />);

  fireEvent.click(screen.getByRole("button", { name: /chat title/i }));

  const input = screen.getByRole("textbox");
  expect(input).toHaveClass("chat-title-input");
});

// The header is the flex row that aligns the title with its actions. An action
// rendered as a sibling of the header instead lands on its own line underneath
// the card, so assert it is inside.
test("actions render inside the header row next to the title", () => {
  const { container } = render(
    <ChatHeader
      title="Hello"
      editable
      onTitleSave={() => {}}
      actions={<button data-testid="header-action">go</button>}
    />,
  );

  const header = container.querySelector("header.chat-header");
  expect(header).toBeTruthy();
  expect(header?.contains(screen.getByTestId("header-action"))).toBe(true);
});

test("no actions node renders when none is supplied", () => {
  const { container } = render(
    <ChatHeader title="Hello" editable onTitleSave={() => {}} />,
  );

  expect(container.querySelector("header.chat-header")?.children).toHaveLength(
    1,
  );
});
