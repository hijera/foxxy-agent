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
  expect(screen.queryByTestId("foxxycode-skill-span")).toBeNull();
});

test("user bubble does not treat path slashes as skill chips without knownSkillNames", () => {
  render(<UserMessage content="hi /demo there" />);
  expect(screen.getByTestId("user-message-body")).toHaveTextContent(
    "hi /demo there",
  );
  expect(screen.queryByTestId("foxxycode-skill-span")).toBeNull();
});

test("user bubble renders known skill as chip when knownSkillNames provided", () => {
  const known = new Set(["generate-rules"]);
  render(<UserMessage content="please /generate-rules for me" knownSkillNames={known} />);
  const chip = screen.getByTestId("foxxycode-skill-span");
  expect(chip).toHaveTextContent("/generate-rules");
  expect(chip).toHaveAttribute("data-skill-name", "generate-rules");
});

test("user bubble does not chip /name absent from knownSkillNames", () => {
  const known = new Set(["generate-rules"]);
  render(<UserMessage content="see /unknown-cmd here" knownSkillNames={known} />);
  expect(screen.queryByTestId("foxxycode-skill-span")).toBeNull();
  expect(screen.getByTestId("user-message-body")).toHaveTextContent(
    "see /unknown-cmd here",
  );
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

test("edit button is absent when onEdit is not provided", () => {
  render(<UserMessage content="hello" />);
  expect(screen.queryByTestId("user-message-edit")).toBeNull();
});

test("edit button is visible when onEdit is provided", () => {
  render(<UserMessage content="hello" onEdit={vi.fn()} />);
  expect(screen.getByTestId("user-message-edit")).toBeInTheDocument();
});

test("edit button calls onEdit with message content", () => {
  const onEdit = vi.fn();
  render(<UserMessage content="edit me" onEdit={onEdit} />);
  screen.getByTestId("user-message-edit").click();
  expect(onEdit).toHaveBeenCalledWith("edit me");
});

test("persisted hydrated attachments render as compact @ paths", () => {
  const blob =
    "read this\n\n" +
    '<foxxycode_attachment path="note.txt" name="note.txt">\n' +
    "<![CDATA[secret body]]>\n" +
    "</foxxycode_attachment>";
  render(<UserMessage content={blob} />);
  expect(screen.getByText(/read this/)).toBeInTheDocument();
  expect(screen.getByText(/@note\.txt/)).toBeInTheDocument();
  expect(screen.queryByText(/secret body/)).toBeNull();
});
