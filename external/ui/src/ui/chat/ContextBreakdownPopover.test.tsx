import React from "react";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { ContextBreakdownPopover } from "./ContextBreakdownPopover";
import { OpenRailScreen } from "../nav/railEscape.fakes";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

test("context action shows the threshold, posts compaction and holds progress", async () => {
  let finish: ((response: Response) => void) | undefined;
  const pending = new Promise<Response>((resolve) => {
    finish = resolve;
  });
  const fetchMock = vi.fn(() => pending);
  vi.stubGlobal("fetch", fetchMock);
  const onCompacted = vi.fn();
  render(
    <ContextBreakdownPopover
      open
      onClose={() => {}}
      maxContextTokens={1000}
      sessionId="sess_123"
      compactAvailable
      compactAutoEnabled
      compactThreshold={95}
      onCompacted={onCompacted}
    />,
  );

  const action = screen.getByTestId("context-breakdown-compact");
  expect(action).toHaveTextContent("Compact at 95%");
  fireEvent.click(action);
  expect(action).toBeDisabled();
  expect(action).toHaveTextContent("Compacting");
  expect(fetchMock).toHaveBeenCalledWith(
    "/foxxycode/sessions/sess_123/compact",
    expect.objectContaining({
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-FoxxyCode-Session-ID": "sess_123",
      },
      body: "{}",
    }),
  );
  finish!(new Response("{}", { status: 200 }));
  await waitFor(() => expect(onCompacted).toHaveBeenCalledOnce());
  expect(action).not.toBeDisabled();
});

// Older JCEF runtimes have no AbortSignal.timeout (the runtimes the remote
// probes shim an AbortController for); the compact request must still run
// there instead of landing in the error state on every click.
test("context action compacts without AbortSignal.timeout", async () => {
  vi.stubGlobal("AbortSignal", {});
  const fetchMock = vi.fn(async () =>
    new Response(JSON.stringify({ compacted: true }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }),
  );
  vi.stubGlobal("fetch", fetchMock);
  const onCompacted = vi.fn();
  render(
    <ContextBreakdownPopover
      open
      onClose={() => {}}
      maxContextTokens={1000}
      sessionId="sess_123"
      compactAvailable
      compactAutoEnabled
      onCompacted={onCompacted}
    />,
  );

  fireEvent.click(screen.getByTestId("context-breakdown-compact"));
  await waitFor(() => expect(onCompacted).toHaveBeenCalledOnce());
});

test("context action says compact now when automation is disabled", () => {
  render(
    <ContextBreakdownPopover
      open
      onClose={() => {}}
      maxContextTokens={1000}
      sessionId="sess_123"
      compactAvailable
      compactAutoEnabled={false}
    />,
  );
  expect(screen.getByTestId("context-breakdown-compact")).toHaveTextContent(
    "Compact now",
  );
});

test("context action says nothing to compact when the endpoint folds nothing", async () => {
  const fetchMock = vi.fn(async () =>
    Promise.resolve(
      new Response(JSON.stringify({ compacted: false, reason: "nothing_to_compact" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    ),
  );
  vi.stubGlobal("fetch", fetchMock);
  const onCompacted = vi.fn();
  render(
    <ContextBreakdownPopover
      open
      onClose={() => {}}
      maxContextTokens={1000}
      sessionId="sess_123"
      compactAvailable
      compactAutoEnabled
      onCompacted={onCompacted}
    />,
  );

  fireEvent.click(screen.getByTestId("context-breakdown-compact"));
  await waitFor(() =>
    expect(screen.getByText(/nothing to compact/i)).toBeInTheDocument(),
  );
  expect(onCompacted).not.toHaveBeenCalled();
});

// History open beside the chat, the breakdown opened over the composer after
// it: Escape takes the breakdown down first, and History stays for the next one.
test("Escape closes the breakdown before the drawer open beside the chat", () => {
  const onClose = vi.fn();
  const closeHistory = vi.fn();
  render(
    <>
      <OpenRailScreen id="history" onClose={closeHistory} />
      <ContextBreakdownPopover open onClose={onClose} maxContextTokens={128000} />
    </>,
  );
  fireEvent.keyDown(document.body, { key: "Escape" });
  expect(onClose).toHaveBeenCalledTimes(1);
  expect(closeHistory).not.toHaveBeenCalled();
});
