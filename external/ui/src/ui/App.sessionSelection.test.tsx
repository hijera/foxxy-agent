import React from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { App } from "./App";
import { ConfirmProvider } from "./components/useConfirm";
import { initLocale } from "./i18n/i18n";

/**
 * The selection a session carries - Mode, Model, Reasoning - is adopted once
 * per session open, and a pick the reader made outranks it. `loadMessages` is
 * not only "open a session": it runs after every turn, on reconnect and on
 * every background refresh, and each read carries the stored selection again.
 * Re-applied every time, it snaps the composer back over whatever the reader
 * chose a moment earlier; applied after a pick was made, it undoes it.
 */

const SESS = "sess_selection";
const MODEL = "rpa/qwen3.8-27b";

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

function emptyStream() {
  return new Response(
    new ReadableStream<Uint8Array>({ start: (c) => c.close() }),
    { headers: { "Content-Type": "text/event-stream" } },
  );
}

/** The server-events stream, held open so a test can push a frame into it. */
let eventsController: ReadableStreamDefaultController<Uint8Array> | null = null;
function eventsStream() {
  return new Response(
    new ReadableStream<Uint8Array>({
      start: (c) => {
        eventsController = c;
      },
    }),
    { headers: { "Content-Type": "text/event-stream" } },
  );
}

/** The models list, held back until a test lets it through. */
let releaseModels: (() => void) | null = null;
let holdModels = false;

const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
  const path = String(input);
  if (path === "/foxxycode/events") return eventsStream();
  if (path === "/v1/models") {
    const answer = () =>
      json({
        data: [
          { id: "agent", owned_by: "foxxycode", max_context_tokens: 128000 },
          {
            id: MODEL,
            owned_by: "rpa",
            max_context_tokens: 128000,
            reasoning_levels: ["low", "medium", "high"],
          },
        ],
      });
    if (holdModels) {
      holdModels = false;
      return new Promise<Response>((resolve) => {
        releaseModels = () => resolve(answer());
      });
    }
    return answer();
  }
  if (path.startsWith("/foxxycode/sessions?")) {
    return json({ sessions: [{ id: SESS, title: SESS }] });
  }
  const match = path.match(/^\/foxxycode\/sessions\/([^/]+)(.*)$/);
  if (match) {
    const sid = decodeURIComponent(match[1]!);
    const suffix = match[2]!.split("?")[0];
    if (suffix === "/messages") {
      return json({
        model: MODEL,
        selectedModelId: MODEL,
        // The profile the session was left at on the server: the values a read
        // would put back over whatever the reader picks in between.
        mode: "agent",
        selectedReasoning: "high",
        messages: [{ role: "user", content: `prompt in ${sid}` }],
      });
    }
    if (suffix === "/composer-stream") return emptyStream();
    if (suffix === "/tool-calls") return json({ toolCalls: [] });
    if (suffix === "/rewind")
      return json({
        object: "foxxycode.session_rewound",
        sessionId: sid,
        messagesRev: 1,
      });
    if (suffix === "/stats") return json({ stats: {} });
    if (suffix === "/background-tasks") return json({ data: [], running: 0 });
    if (suffix === "/activity") return json({ sessionId: sid, turnActive: false });
    if (!suffix) return json({});
  }
  if (path === "/foxxycode/config") return json({});
  if (path.startsWith("/foxxycode/slash-commands")) return json({ items: [] });
  if (path === "/foxxycode/workspace/context")
    return json({ cwd: "/workspace", is_git_repo: false });
  return json({}, 404);
});

beforeEach(() => {
  eventsController = null;
  releaseModels = null;
  holdModels = false;
  initLocale("en");
  localStorage.clear();
  document.cookie = "foxxycode_llm_reasoning=; Path=/; Max-Age=0";
  history.replaceState(null, "", `/#/s/${SESS}`);
  fetchMock.mockClear();
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const reasoningChip = () =>
  screen.getByRole("button", { name: "Reasoning level" });

test("a Mode picked before the session's own arrives is kept", async () => {
  holdModels = true;
  render(
    <ConfirmProvider>
      <App />
    </ConfirmProvider>,
  );
  // The transcript is there and the composer is up; the models list, and with
  // it the read that would apply the stored selection, is still in flight.
  await screen.findByText(`prompt in ${SESS}`);
  const modeChip = screen.getByRole("button", { name: "Mode" });
  fireEvent.click(modeChip);
  fireEvent.click(await screen.findByRole("menuitem", { name: "Plan" }));
  await waitFor(() => expect(modeChip).toHaveTextContent("Plan"));

  await act(async () => {
    releaseModels?.();
    await new Promise((r) => setTimeout(r, 0));
  });
  await new Promise((r) => setTimeout(r, 50));
  expect(modeChip).toHaveTextContent("Plan");
});

test("a later transcript read does not put the stored level back", async () => {
  render(
    <ConfirmProvider>
      <App />
    </ConfirmProvider>,
  );
  await screen.findByText(`prompt in ${SESS}`);
  await waitFor(() => expect(reasoningChip()).toHaveTextContent("High"));

  fireEvent.click(reasoningChip());
  fireEvent.click(await screen.findByRole("menuitem", { name: "Low" }));
  await waitFor(() => expect(reasoningChip()).toHaveTextContent("Low"));

  // A read for the same session - a rewind elsewhere, a reconnect, a refresh -
  // carries the stored selection again, in a snapshot object of its own.
  await act(async () => {
    eventsController?.enqueue(
      new TextEncoder().encode(
        `event: session_rewound\ndata: {"object":"foxxycode.session_rewound","sessionId":"${SESS}","messagesRev":1}\n\n`,
      ),
    );
    await new Promise((r) => setTimeout(r, 0));
  });
  await new Promise((r) => setTimeout(r, 50));
  expect(reasoningChip()).toHaveTextContent("Low");
});
