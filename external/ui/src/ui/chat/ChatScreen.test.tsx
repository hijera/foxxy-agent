import React from "react";
import { afterEach, expect, test } from "vitest";
import { cleanup, render } from "@testing-library/react";
import { ChatScreen } from "./ChatScreen";

afterEach(() => cleanup());

test("empty hero shows headline with accent span", () => {
  const { getByTestId, getByRole } = render(
    <ChatScreen
      title=""
      sessionId=""
      heroAccentVerb="know"
      heroComposerFocusEpoch={0}
      onTitleSave={() => {}}
      items={[]}
      draft=""
      tokenUsage={null}
      mode="agent"
      modes={["agent", "plan"]}
      onModeChange={() => {}}
      onDraftChange={() => {}}
      onSend={() => {}}
    />,
  );

  expect(getByRole("heading", { level: 1 })).toHaveTextContent(
    "What do you want to know?",
  );
  expect(getByTestId("hero-title-accent")).toHaveTextContent("know");
  expect(getByRole("textbox")).toHaveFocus();
});

test("active chat wraps title in chat-title-column aligned with composer column", () => {
  const { container } = render(
    <ChatScreen
      title="Hi"
      sessionId="s1"
      heroAccentVerb="know"
      heroComposerFocusEpoch={0}
      onTitleSave={() => {}}
      items={[{ type: "user_message", id: "1", content: "x" }]}
      draft=""
      tokenUsage={null}
      mode="agent"
      modes={["agent", "plan"]}
      onModeChange={() => {}}
      onDraftChange={() => {}}
      onSend={() => {}}
    />,
  );

  const col = container.querySelector(".chat-title-column");
  expect(col).toBeTruthy();
  expect(col?.querySelector(".chat-header")).toBeTruthy();
});
