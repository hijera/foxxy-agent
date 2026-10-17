import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { App } from "./App";
import { ConfirmProvider } from "./components/useConfirm";
import { initLocale } from "./i18n/i18n";
import { applyNotificationConfig } from "./desktop/notificationConfig";

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), {
  status, headers: { "Content-Type": "application/json" },
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  delete window.foxxycodeHostNotify;
  delete window.foxxycodeHostIsActive;
  applyNotificationConfig({});
  sessionStorage.clear();
  history.replaceState(null, "", "/");
});

test.each([
  { name: "background", focused: false, whenFocused: false, delivered: true },
  { name: "foreground by default", focused: true, whenFocused: false, delivered: false },
  { name: "foreground enabled", focused: true, whenFocused: true, delivered: true },
])("a completed task notifies once: $name", async ({ focused, whenFocused, delivered }) => {
  const sid = `sess_notification_${focused}_${whenFocused}`;
  let active = false;
  let finished = false;
  let events: ReadableStreamDefaultController<Uint8Array> | undefined;
  const host = vi.fn().mockResolvedValue(true);
  window.foxxycodeHostNotify = host;
  window.foxxycodeHostIsActive = () => focused;
  initLocale("en");
  localStorage.clear();
  sessionStorage.clear();
  history.replaceState(null, "", `/?desktop=1#/s/${sid}`);
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const path = String(input);
    if (path === "/foxxycode/events") return new Response(new ReadableStream<Uint8Array>({
      start: (controller) => { events = controller; },
    }), { headers: { "Content-Type": "text/event-stream" } });
    if (path === "/v1/models") return json({ default_agent_model: "test/model", data: [
      { id: "agent", owned_by: "foxxycode" }, { id: "test/model", owned_by: "test" },
    ] });
    if (path.startsWith("/foxxycode/sessions?")) return json({ sessions: [{ id: sid, title: "Notification chat" }] });
    if (path === "/foxxycode/config") return json({ ui: { notifications_when_focused: whenFocused } });
    if (path === "/foxxycode/workspace/context") return json({ cwd: "/workspace", is_git_repo: false });
    if (path.startsWith("/foxxycode/slash-commands")) return json({ items: [] });
    if (path === `/foxxycode/sessions/${sid}/messages`) return json({
      model: "test/model", selectedModelId: "test/model",
      messages: [{ role: "user", content: "Please finish this task." },
        ...(finished ? [{ role: "assistant", content: "The change is ready." }] : [])],
    });
    if (path === `/foxxycode/sessions/${sid}/activity`) return json({ sessionId: sid, turnActive: active });
    if (path === `/foxxycode/sessions/${sid}/queue`) return json({ messages: [], version: 0 });
    if (path === `/foxxycode/sessions/${sid}/composer-stream`) return new Response(new ReadableStream<Uint8Array>({ start: (controller) => controller.close() }), { headers: { "Content-Type": "text/event-stream" } });
    if (path.endsWith("/tool-calls")) return json({ toolCalls: [] });
    if (path.endsWith("/branches")) return json({ branchPoints: [] });
    if (path.endsWith("/stats")) return json({ stats: {} });
    if (path.endsWith("/background-tasks")) return json({ data: [], running: 0 });
    return json({}, 404);
  });
  vi.stubGlobal("fetch", fetchMock);
  render(<ConfirmProvider><App /></ConfirmProvider>);
  await screen.findByText("Please finish this task.");
  await waitFor(() => expect(events).toBeDefined());
  active = true;
  await act(async () => { events!.enqueue(new TextEncoder().encode(`event: turn_started\ndata: {"sessionId":"${sid}"}\n\n`)); });
  await waitFor(() => expect(fetchMock.mock.calls.some(([url]) => String(url).endsWith("/composer-stream"))).toBe(true));
  active = false;
  finished = true;
  await act(async () => { events!.enqueue(new TextEncoder().encode(`event: turn_ended\ndata: {"sessionId":"${sid}"}\n\n`)); });
  await screen.findByText("The change is ready.");
  if (delivered) {
    await waitFor(() => expect(host).toHaveBeenCalledTimes(1));
    expect(host).toHaveBeenCalledWith(expect.objectContaining({
      title: "FoxxyCode — Notification chat", body: "Task finished\nThe change is ready.", whenFocused,
    }));
  } else expect(host).not.toHaveBeenCalled();
  await act(async () => { events!.enqueue(new TextEncoder().encode(`event: turn_ended\ndata: {"sessionId":"${sid}"}\n\n`)); });
  expect(host).toHaveBeenCalledTimes(delivered ? 1 : 0);
});
