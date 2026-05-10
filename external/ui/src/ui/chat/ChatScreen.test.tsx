import React from "react";
import { afterEach, expect, test } from "vitest";
import { cleanup, render } from "@testing-library/react";
import { ChatScreen } from "./ChatScreen";

afterEach(() => cleanup());

test("active chat wraps title in chat-title-column aligned with composer column", () => {
  const { container } = render(
    <ChatScreen
      title="Hi"
      sessionId="s1"
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
