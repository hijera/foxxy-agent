import React from "react";
import { afterEach, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { ChatHeader } from "./ChatHeader";

afterEach(() => cleanup());

test("edit mode shows full-width title input class", () => {
  render(<ChatHeader title="Hello" editable onTitleSave={() => {}} />);

  fireEvent.click(screen.getByRole("button", { name: /chat title/i }));

  const input = screen.getByRole("textbox");
  expect(input).toHaveClass("chat-title-input");
});

test("starts mini-app distillation for the open session", () => {
  const onCreateMiniApp = vi.fn();
  render(
    <ChatHeader
      title="Completed session"
      editable
      onTitleSave={() => {}}
      onCreateMiniApp={onCreateMiniApp}
    />,
  );

  fireEvent.click(
    screen.getByRole("button", { name: "Create mini app from this session" }),
  );

  expect(onCreateMiniApp).toHaveBeenCalledOnce();
});
