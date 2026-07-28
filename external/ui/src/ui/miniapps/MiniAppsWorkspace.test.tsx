import { afterEach, expect, test, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { MiniAppsWorkspace } from "./MiniAppsWorkspace";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

test("workspace separates catalog, workflow editor, and generated runner", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input);
      if (path === "/foxxycode/miniapps") {
        return new Response(
          JSON.stringify({
            object: "foxxycode.miniapp.list",
            items: [
              {
                id: "greeting-app",
                name: "Greeting app",
                description: "Formats a greeting.",
                state: "draft",
                updated_at: "2026-07-27T00:00:00Z",
              },
            ],
          }),
          { status: 200 },
        );
      }
      if (path.endsWith("/greeting-app/draft")) {
        return new Response(
          JSON.stringify({
            schema_version: "1.0.0",
            kind: "foxxycode.miniapp",
            id: "greeting-app",
            state: "draft",
            metadata: {
              name: "Greeting app",
              description: "Formats a greeting.",
              goal: "Return a greeting.",
            },
            inputs: [
              {
                id: "name",
                type: "string",
                title: "Name",
                required: true,
                ui: { control: "text" },
              },
            ],
            workflow: [{ id: "format-step", kind: "program", title: "Format" }],
            success: { mode: "all", checks: [] },
            outputs: [],
            runtime: { persist_agent_reasoning: false },
          }),
          { status: 200 },
        );
      }
      return new Response("{}", { status: 404 });
    }),
  );

  const { container } = render(
    <MiniAppsWorkspace open currentSessionId="session-1" onClose={() => {}} />,
  );

  await waitFor(() =>
    expect(screen.getByRole("button", { name: /Greeting app/ })).toBeTruthy(),
  );
  screen.getByRole("button", { name: /Greeting app/ }).click();

  await waitFor(() =>
    expect(container.querySelector(".miniapps-authoring")).toBeTruthy(),
  );
  expect(container.querySelector(".miniapps-catalog")).toBeTruthy();
  expect(container.querySelector(".miniapps-step-nav")).toBeTruthy();
  expect(screen.getByLabelText("Input id")).toHaveValue("name");

  screen.getByRole("tab", { name: "Run" }).click();
  await waitFor(() =>
    expect(container.querySelector(".miniapps-runner")).toBeTruthy(),
  );
});

test("new mini app opens when the API omits empty optional arrays", async () => {
  let created = false;
  let createdID = "";
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      const method = init?.method || "GET";
      if (path === "/foxxycode/miniapps" && method === "POST") {
        createdID = String(
          (JSON.parse(String(init?.body)) as { id?: unknown }).id || "",
        );
        created = true;
        return new Response(init?.body, { status: 201 });
      }
      if (path === "/foxxycode/miniapps" && method === "GET") {
        return new Response(
          JSON.stringify({
            object: "foxxycode.miniapp.list",
            items: created
              ? [
                  {
                    id: createdID,
                    name: "New mini app",
                    description: "Reusable operator workflow.",
                    state: "draft",
                    updated_at: "2026-07-28T00:00:00Z",
                  },
                ]
              : [],
          }),
          { status: 200 },
        );
      }
      if (
        createdID &&
        path === `/foxxycode/miniapps/${encodeURIComponent(createdID)}/draft`
      ) {
        return new Response(
          JSON.stringify({
            schema_version: "1.0.0",
            kind: "foxxycode.miniapp",
            id: createdID,
            state: "draft",
            metadata: {
              name: "New mini app",
              description: "Reusable operator workflow.",
              goal: "Produce the reviewed result.",
            },
            workflow: [
              {
                id: "produce-result",
                kind: "program",
                title: "Produce result",
              },
            ],
            success: { mode: "all", checks: [] },
            runtime: { persist_agent_reasoning: false },
          }),
          { status: 200 },
        );
      }
      return new Response("{}", { status: 404 });
    }),
  );

  const { container } = render(
    <MiniAppsWorkspace open currentSessionId="" onClose={() => {}} />,
  );

  const newButton = await screen.findByRole("button", { name: "New" });
  newButton.click();

  await waitFor(() =>
    expect(container.querySelector(".miniapps-authoring")).toBeTruthy(),
  );
  expect(screen.getByText("Inputs (0)")).toBeTruthy();
});

test("a chat action distills the current session and opens its draft", async () => {
  const calls: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      calls.push(`${init?.method || "GET"} ${path}`);
      if (
        path === "/foxxycode/sessions/session-1/miniapps/distill" &&
        init?.method === "POST"
      ) {
        return new Response(
          JSON.stringify({
            id: "distill-1",
            session_id: "session-1",
            status: "completed",
            phase: "draft_ready",
            progress: 100,
            app_id: "distilled-app",
          }),
          { status: 202 },
        );
      }
      if (path === "/foxxycode/miniapps") {
        return new Response(JSON.stringify({ items: [] }), { status: 200 });
      }
      if (path.endsWith("/distilled-app/draft")) {
        return new Response(
          JSON.stringify({
            schema_version: "1.0.0",
            kind: "foxxycode.miniapp",
            id: "distilled-app",
            state: "draft",
            metadata: {
              name: "Distilled app",
              description: "Created from the session.",
              goal: "Reproduce the accepted result.",
            },
            workflow: [
              { id: "execute-task", kind: "program", title: "Execute task" },
            ],
            success: { mode: "all", checks: [] },
            runtime: { persist_agent_reasoning: false },
          }),
          { status: 200 },
        );
      }
      return new Response("{}", { status: 404 });
    }),
  );

  render(
    <MiniAppsWorkspace
      open
      currentSessionId="session-1"
      distillRequestEpoch={1}
      onClose={() => {}}
    />,
  );

  await screen.findByDisplayValue("Distilled app");
  expect(calls).toContain(
    "POST /foxxycode/sessions/session-1/miniapps/distill",
  );
});

test("generates and stores an expected result from author expectations", async () => {
  const app = {
    schema_version: "1.0.0",
    kind: "foxxycode.miniapp",
    id: "greeting-app",
    state: "draft",
    metadata: {
      name: "Greeting app",
      description: "Formats a greeting.",
      goal: "Return a greeting.",
    },
    inputs: [],
    workflow: [{ id: "format-step", kind: "program", title: "Format" }],
    success: { mode: "all", checks: [] },
    outputs: [],
    runtime: { persist_agent_reasoning: false },
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      if (path === "/foxxycode/miniapps" && !init?.method) {
        return new Response(
          JSON.stringify({
            items: [
              {
                id: "greeting-app",
                name: "Greeting app",
                description: "Formats a greeting.",
                state: "draft",
                updated_at: "2026-07-28T00:00:00Z",
              },
            ],
          }),
          { status: 200 },
        );
      }
      if (path.endsWith("/greeting-app/draft")) {
        return new Response(JSON.stringify(app), { status: 200 });
      }
      if (
        path.endsWith("/greeting-app/expected-result") &&
        init?.method === "POST"
      ) {
        const updated = {
          ...app,
          success: {
            mode: "all",
            expectations: "The greeting must address the supplied person.",
            expected_result: "A friendly greeting using the supplied name.",
            acceptance_criterion:
              "The output is friendly and contains the supplied name.",
            checks: [],
          },
        };
        return new Response(
          JSON.stringify({
            app: updated,
            suggestion: {
              expectations: updated.success.expectations,
              expected_result: updated.success.expected_result,
              acceptance_criterion: updated.success.acceptance_criterion,
              model_binding: "acceptance-model",
            },
          }),
          { status: 200 },
        );
      }
      return new Response("{}", { status: 404 });
    }),
  );

  render(<MiniAppsWorkspace open currentSessionId="" onClose={() => {}} />);
  fireEvent.click(await screen.findByRole("button", { name: /Greeting app/ }));
  const expectations = await screen.findByLabelText("Author expectations");
  fireEvent.change(expectations, {
    target: {
      value: "The greeting must address the supplied person.",
    },
  });
  fireEvent.click(
    screen.getByRole("button", { name: "Generate expected result with LLM" }),
  );

  await screen.findByDisplayValue(
    "A friendly greeting using the supplied name.",
  );
});
