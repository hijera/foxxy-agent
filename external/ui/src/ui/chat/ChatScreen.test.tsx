import React from "react";
import { afterEach, expect, test, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
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

test("empty hero no longer surfaces the GitHub or API docs footer links", () => {
  const { container } = render(
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

  // The whole hero-footer block (GitHub + API docs) was removed so the empty
  // screen is unbranded; docs moved into Settings.
  expect(container.querySelector(".hero-footer")).toBeNull();
  expect(container.querySelector('a[href^="https://github.com/"]')).toBeNull();
  expect(container.querySelector('a[href="/docs/"]')).toBeNull();
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

// The panel offers the download only once there is an answer to download, and
// the control belongs inside the header row rather than under the header card.
test("export action is hidden until an assistant answer exists", () => {
  const { container } = render(
    <ChatScreen
      title="Hi"
      sessionId="s1"
      heroAccentVerb="know"
      heroComposerFocusEpoch={0}
      onTitleSave={() => {}}
      onExportSession={() => {}}
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

  expect(container.querySelector(".session-export")).toBeNull();
});

test("export action renders inside the chat header once an answer arrives", () => {
  const { container } = render(
    <ChatScreen
      title="Hi"
      sessionId="s1"
      heroAccentVerb="know"
      heroComposerFocusEpoch={0}
      onTitleSave={() => {}}
      onExportSession={() => {}}
      items={[
        { type: "user_message", id: "1", content: "x" },
        { type: "assistant_message", id: "2", content: "answer" },
      ]}
      draft=""
      tokenUsage={null}
      mode="agent"
      modes={["agent", "plan"]}
      onModeChange={() => {}}
      onDraftChange={() => {}}
      onSend={() => {}}
    />,
  );

  const header = container.querySelector(".chat-header");
  const exportAction = container.querySelector(".session-export");
  expect(exportAction).toBeTruthy();
  expect(header?.contains(exportAction)).toBe(true);
});

test("a whitespace-only assistant answer does not offer the download", () => {
  const { container } = render(
    <ChatScreen
      title="Hi"
      sessionId="s1"
      heroAccentVerb="know"
      heroComposerFocusEpoch={0}
      onTitleSave={() => {}}
      onExportSession={() => {}}
      items={[{ type: "assistant_message", id: "2", content: "   " }]}
      draft=""
      tokenUsage={null}
      mode="agent"
      modes={["agent", "plan"]}
      onModeChange={() => {}}
      onDraftChange={() => {}}
      onSend={() => {}}
    />,
  );

  expect(container.querySelector(".session-export")).toBeNull();
});

const childTranscript = {
  parentSessionId: "s_parent",
  name: "explore",
  taskId: "bg_3",
};

test("a subagent transcript replaces the docked composer with a read-only notice", () => {
  const onOpenSession = vi.fn();
  const { container } = render(
    <ChatScreen
      title="agent explore"
      sessionId="sub_0a1b2c"
      heroAccentVerb="know"
      heroComposerFocusEpoch={0}
      onTitleSave={() => {}}
      items={[{ type: "user_message", id: "1", content: "survey the repo" }]}
      draft=""
      tokenUsage={null}
      mode="agent"
      modes={["agent", "plan"]}
      onModeChange={() => {}}
      onDraftChange={() => {}}
      onSend={() => {}}
      subagentTranscript={childTranscript}
      onOpenSession={onOpenSession}
    />,
  );

  expect(container.querySelector(".composer-card")).toBeNull();
  expect(screen.getByTestId("subagent-readonly-notice")).toHaveTextContent(
    "Read-only transcript of subagent explore",
  );
  fireEvent.click(screen.getByTestId("subagent-readonly-parent-link"));
  expect(onOpenSession).toHaveBeenCalledWith("s_parent");
});

test("the notice also takes the hero composer's slot on an empty child transcript", () => {
  const { container } = render(
    <ChatScreen
      title=""
      sessionId="sub_0a1b2c"
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
      subagentTranscript={childTranscript}
    />,
  );

  expect(container.querySelector(".composer-card")).toBeNull();
  expect(screen.getByTestId("subagent-readonly-notice")).toBeInTheDocument();
});

// JCEF (Chromium 104) raised "ResizeObserver loop limit exceeded" on every
// transcript open: the host observer wrote the scroll-tail reserve inside the
// observer's own delivery loop. The write must wait for the next frame and a
// repeat of the same height must not touch state at all.
test("the composer reserve is written on the next frame, not inside the resize callback", () => {
  const callbacks: Array<() => void> = [];
  const RO = class {
    constructor(cb: () => void) {
      callbacks.push(cb);
    }
    observe() {}
    disconnect() {}
  };
  const frames: Array<() => void> = [];
  const prevRO = (globalThis as { ResizeObserver?: unknown }).ResizeObserver;
  const prevRaf = globalThis.requestAnimationFrame;
  const prevCaf = globalThis.cancelAnimationFrame;
  (globalThis as { ResizeObserver?: unknown }).ResizeObserver = RO;
  globalThis.requestAnimationFrame = (cb) => {
    frames.push(() => cb(0));
    return frames.length;
  };
  globalThis.cancelAnimationFrame = () => {};
  try {
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
    const host = container.querySelector(".chat-bottom-inner") as HTMLElement;
    const reserveOf = () =>
      (container.querySelector("[style*='--chat-composer-reserve']") as HTMLElement)
        .style.getPropertyValue("--chat-composer-reserve");
    expect(callbacks).toHaveLength(1);
    expect(reserveOf()).toBe("140px");

    host.getBoundingClientRect = () => ({ height: 300 }) as DOMRect;
    act(() => callbacks[0]!());
    // Still the mount-time value: nothing changed inside the delivery loop.
    expect(reserveOf()).toBe("140px");
    expect(frames).toHaveLength(1);

    act(() => frames.shift()!());
    expect(reserveOf()).toBe("310px");

    // The same height again schedules a frame but leaves the DOM alone.
    act(() => callbacks[0]!());
    act(() => frames.shift()!());
    expect(reserveOf()).toBe("310px");
  } finally {
    (globalThis as { ResizeObserver?: unknown }).ResizeObserver = prevRO;
    globalThis.requestAnimationFrame = prevRaf;
    globalThis.cancelAnimationFrame = prevCaf;
  }
});
