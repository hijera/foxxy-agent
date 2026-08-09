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

test("author can add and remove inputs and workflow steps", async () => {
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
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input);
      if (path === "/foxxycode/miniapps") {
        return new Response(
          JSON.stringify({
            items: [
              {
                id: app.id,
                name: app.metadata.name,
                description: app.metadata.description,
                state: app.state,
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
      return new Response("{}", { status: 404 });
    }),
  );

  render(<MiniAppsWorkspace open currentSessionId="" onClose={() => {}} />);
  fireEvent.click(await screen.findByRole("button", { name: /Greeting app/ }));

  fireEvent.click(await screen.findByRole("button", { name: "Add input" }));
  expect(screen.getAllByLabelText("Input id")).toHaveLength(2);
  fireEvent.click(screen.getByRole("button", { name: "Remove input Name" }));
  expect(screen.getAllByLabelText("Input id")).toHaveLength(1);

  fireEvent.click(screen.getByRole("button", { name: "Add step" }));
  expect(screen.getByText("Steps (2)")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Remove step Format" }));
  expect(screen.getByText("Steps (1)")).toBeTruthy();
});

test("selected logical model is saved and authoring chat applies tool edits", async () => {
  let app = {
    schema_version: "1.0.0",
    kind: "foxxycode.miniapp",
    id: "greeting-app",
    state: "draft",
    metadata: {
      name: "Greeting app",
      description: "Formats a greeting.",
      goal: "Return a greeting.",
    },
    requirements: {
      model_bindings: [
        {
          id: "primary",
          logical_model: "fake/original-model",
          selection: "fixed",
          provider: {
            type: "openai",
            base_url: "https://example.invalid/v1",
          },
          model: "original-model",
        },
      ],
    },
    permissions: { models: ["primary"] },
    inputs: [],
    workflow: [
      {
        id: "agent-step",
        kind: "agent",
        title: "Draft response",
        model_binding: "primary",
        prompt: "Write a response.",
      },
    ],
    success: { mode: "all", checks: [] },
    outputs: [],
    runtime: { persist_agent_reasoning: false },
  };
  const requests: Array<{ path: string; body: unknown }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      const body = init?.body ? JSON.parse(String(init.body)) : undefined;
      if (init?.method === "POST") {
        requests.push({ path, body });
      }
      if (path === "/foxxycode/miniapps" && !init?.method) {
        return new Response(
          JSON.stringify({
            items: [
              {
                id: app.id,
                name: app.metadata.name,
                description: app.metadata.description,
                state: app.state,
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
        path.endsWith("/greeting-app/model-binding") &&
        init?.method === "POST"
      ) {
        app = {
          ...app,
          requirements: {
            model_bindings: [
              {
                id: "primary",
                logical_model: "fake/reviewed-model",
                selection: "fixed",
                provider: {
                  type: "openai",
                  base_url: "https://example.invalid/v1",
                },
                model: "reviewed-model",
              },
            ],
          },
        };
        return new Response(JSON.stringify(app), { status: 200 });
      }
      if (
        path.endsWith("/greeting-app/authoring/chat") &&
        init?.method === "POST"
      ) {
        app = {
          ...app,
          inputs: [
            {
              id: "style",
              type: "string",
              title: "Style",
              ui: { control: "text" },
            },
          ],
          workflow: [
            ...app.workflow,
            { id: "decorate", kind: "program", title: "Decorate" },
          ],
        };
        return new Response(
          JSON.stringify({
            app,
            message: "Added the Style input and Decorate step.",
            operations: ["upsert_input:style", "upsert_step:decorate"],
            model_binding: "primary",
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
      currentSessionId=""
      availableModels={["fake/original-model", "fake/reviewed-model"]}
      onClose={() => {}}
    />,
  );
  fireEvent.click(await screen.findByRole("button", { name: /Greeting app/ }));

  fireEvent.change(await screen.findByLabelText("Logical model"), {
    target: { value: "fake/reviewed-model" },
  });
  await waitFor(() =>
    expect(requests).toContainEqual({
      path: "/foxxycode/miniapps/greeting-app/model-binding",
      body: expect.objectContaining({ model_ref: "fake/reviewed-model" }),
    }),
  );

  fireEvent.change(screen.getByLabelText("Authoring assistant message"), {
    target: { value: "Add a Style input and a Decorate step." },
  });
  fireEvent.click(screen.getByRole("button", { name: "Send" }));

  await screen.findByText("Added the Style input and Decorate step.");
  expect(screen.getByDisplayValue("style")).toBeTruthy();
  expect(screen.getByText("Decorate")).toBeTruthy();
  expect(requests).toContainEqual({
    path: "/foxxycode/miniapps/greeting-app/authoring/chat",
    body: expect.objectContaining({
      message: "Add a Style input and a Decorate step.",
      draft: expect.objectContaining({ id: "greeting-app" }),
    }),
  });
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
