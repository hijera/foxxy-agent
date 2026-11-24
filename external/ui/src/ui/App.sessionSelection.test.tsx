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
const PLAN = "sess_parallel_plan";
const CHAT = "sess_parallel_chat";
const MODEL = "rpa/qwen3.8-27b";

let storedModes: Record<string, string>;
let storedReasoning: Record<string, string>;
let settingsVersions: Record<string, number>;
let heldTurnSids: Set<string>;
let openTurns: Map<string, ReadableStreamDefaultController<Uint8Array>>;
/** The next messages read, held back until a test lets it through. */
let holdRead: boolean;
let releaseRead: (() => void) | null;
/** The next settings write's answer, held back the same way. */
let holdPatch: boolean;
let releasePatch: (() => void) | null;

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

/**
 * The settings snapshot a read or a write's answer carries. Built per call, so
 * a response held back keeps the state of its request time - a read that raced
 * with a write served from before it.
 */
const settingsSnap = (sid: string) => ({
  sessionId: sid,
  version: settingsVersions[sid] ?? 1,
  model: MODEL,
  reasoning: storedReasoning[sid] ?? "high",
  mode: storedModes[sid] ?? "agent",
  permissionMode: "ask",
  configuredPermissionMode: "ask",
  overrides: [],
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

const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
  const path = String(input);
  if (path === "/foxxycode/events") return eventsStream();
  if (path === "/v1/responses") {
    const sid = new Headers(init?.headers).get("X-FoxxyCode-Session-ID") ?? "";
    if (!heldTurnSids.has(sid)) return emptyStream();
    return new Response(
      new ReadableStream<Uint8Array>({
        start: (c) => {
          openTurns.set(sid, c);
        },
      }),
      { headers: { "Content-Type": "text/event-stream" } },
    );
  }
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
    return json({
      sessions: [SESS, PLAN, CHAT].map((id) => ({ id, title: id })),
    });
  }
  const match = path.match(/^\/foxxycode\/sessions\/([^/]+)(.*)$/);
  if (match) {
    const sid = decodeURIComponent(match[1]!);
    const suffix = match[2]!.split("?")[0];
    if (suffix === "/messages") {
      // The profile the session was left at on the server: the values a read
      // would put back over whatever the reader picks in between. The response
      // body is built now, so a held read keeps serving its request time.
      const response = json({
        model: MODEL,
        selectedModelId: MODEL,
        mode: storedModes[sid] ?? "agent",
        selectedReasoning: "high",
        settings: settingsSnap(sid),
        messages: [{ role: "user", content: `prompt in ${sid}` }],
      });
      // Only the settings-only read of the shadow open path is held: a
      // transcript page must still paint while it is.
      if (holdRead && match[2] === "/messages?limit=1") {
        holdRead = false;
        return new Promise<Response>((resolve) => {
          releaseRead = () => resolve(response);
        });
      }
      return response;
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
    if (suffix === "/activity")
      return json({ sessionId: sid, turnActive: openTurns.has(sid) });
    if (!suffix && init?.method === "PATCH") {
      const body = JSON.parse(String(init.body)) as {
        mode?: string;
        selectedReasoning?: string;
      };
      if (body.mode) storedModes[sid] = body.mode;
      if (body.selectedReasoning) storedReasoning[sid] = body.selectedReasoning;
      settingsVersions[sid] = (settingsVersions[sid] ?? 1) + 1;
      // The answer is the settings snapshot of the PATCH route: it carries the
      // picked value and the new version the next read is compared against.
      const response = json({ settings: settingsSnap(sid) });
      if (holdPatch) {
        holdPatch = false;
        return new Promise<Response>((resolve) => {
          releasePatch = () => resolve(response);
        });
      }
      return response;
    }
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
  storedModes = { [SESS]: "agent", [PLAN]: "plan", [CHAT]: "ask" };
  storedReasoning = {};
  settingsVersions = {};
  heldTurnSids = new Set();
  openTurns = new Map();
  holdRead = false;
  releaseRead = null;
  holdPatch = false;
  releasePatch = null;
  initLocale("en");
  localStorage.clear();
  document.cookie = "foxxycode_llm_reasoning=; Path=/; Max-Age=0";
  history.replaceState(null, "", `/#/s/${SESS}`);
  fetchMock.mockClear();
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  for (const controller of openTurns.values()) controller.close();
  openTurns.clear();
  cleanup();
  vi.unstubAllGlobals();
});

const reasoningChip = () =>
  screen.getByRole("button", { name: "Reasoning level" });
const modeChip = () => screen.getByRole("button", { name: "Mode" });
const settle = () => act(async () => new Promise((r) => setTimeout(r, 50)));

function mount(hash: string) {
  history.replaceState(null, "", hash);
  render(
    <ConfirmProvider>
      <App />
    </ConfirmProvider>,
  );
}

async function mountSession(sid: string) {
  mount(`/#/s/${sid}`);
  await screen.findByText(`prompt in ${sid}`);
  await settle();
}

async function goTo(hash: string) {
  await act(async () => {
    history.replaceState(null, "", hash);
    window.dispatchEvent(new HashChangeEvent("hashchange"));
  });
}

async function navigate(sid: string) {
  await goTo(`/#/s/${sid}`);
  await screen.findByText(`prompt in ${sid}`);
  await settle();
}

async function pickMode(name: string) {
  fireEvent.click(modeChip());
  fireEvent.click(await screen.findByRole("menuitem", { name }));
  await waitFor(() => expect(modeChip()).toHaveTextContent(name));
}

async function startHeldTurn(sid: string, text: string) {
  heldTurnSids.add(sid);
  fireEvent.change(screen.getByRole("textbox", { name: "Message" }), {
    target: { value: text },
  });
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  await waitFor(() => expect(openTurns.has(sid)).toBe(true));
  await settle();
}

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

test("a hand-picked mode is restored after leaving and reopening its session", async () => {
  await mountSession(SESS);
  await pickMode("Ask");
  expect(storedModes[SESS]).toBe("ask");

  await navigate(PLAN);
  expect(modeChip()).toHaveTextContent("Plan");

  await navigate(SESS);
  await waitFor(() => expect(modeChip()).toHaveTextContent("Ask"));
});

test("two running sessions each show their own mode on every switch", async () => {
  await mountSession(PLAN);
  await startHeldTurn(PLAN, "plan in parallel");

  await navigate(CHAT);
  expect(modeChip()).toHaveTextContent("Ask");
  await startHeldTurn(CHAT, "chat in parallel");

  // Each way back keeps the live transcript from its stream shadow. The
  // session's settings still have to be read: no turn end or permission answer
  // should be necessary to make the chip catch up.
  await goTo(`/#/s/${PLAN}`);
  await waitFor(() => expect(modeChip()).toHaveTextContent("Plan"));

  await goTo(`/#/s/${CHAT}`);
  await waitFor(() => expect(modeChip()).toHaveTextContent("Ask"));
});

test("a mode picked while the open read is in flight is not undone by that read", async () => {
  await mountSession(CHAT);
  await startHeldTurn(CHAT, "chat in parallel");
  await navigate(PLAN);
  expect(modeChip()).toHaveTextContent("Plan");

  // Coming back to the running session keeps its rows from the stream shadow
  // and reads its settings alone - that read, held here, is in flight while
  // the reader picks a Mode.
  holdRead = true;
  holdPatch = true;
  await goTo(`/#/s/${CHAT}`);
  await pickMode("Plan");

  // The read was served before the pick and the write has not been answered
  // yet: neither may move the Mode back over the pick.
  await act(async () => {
    releaseRead?.();
    await new Promise((r) => setTimeout(r, 0));
  });
  await settle();
  expect(modeChip()).toHaveTextContent("Plan");

  await act(async () => {
    releasePatch?.();
    await new Promise((r) => setTimeout(r, 0));
  });
  await settle();
  expect(modeChip()).toHaveTextContent("Plan");
});

test("a read served before the mode write is dropped once the write lands", async () => {
  await mountSession(CHAT);
  await startHeldTurn(CHAT, "chat in parallel");
  await navigate(PLAN);

  holdRead = true;
  await goTo(`/#/s/${CHAT}`);
  // The write answers and settles the chip before the read comes back.
  await pickMode("Plan");

  await act(async () => {
    releaseRead?.();
    await new Promise((r) => setTimeout(r, 0));
  });
  await settle();
  expect(modeChip()).toHaveTextContent("Plan");
});
