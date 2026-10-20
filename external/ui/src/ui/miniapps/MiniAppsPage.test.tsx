import React from "react";
import { afterEach, expect, test, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { MiniAppsPage } from "./MiniAppsPage";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

test("catalog is visible and can filter apps", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(
    json({
      apps: [
        {
          id: "hello",
          name: "Greeting",
          description: "Write a greeting",
          state: "released",
          version: "1.0.0",
        },
      ],
    }),
  );
  render(<MiniAppsPage onNavigate={() => {}} onClose={() => {}} />);
  expect(
    await screen.findByRole("heading", { name: "Mini Apps" }),
  ).toBeInTheDocument();
  expect(screen.getByText("Greeting")).toBeInTheDocument();
  fireEvent.change(screen.getByLabelText("Search Mini Apps"), {
    target: { value: "missing" },
  });
  expect(screen.queryByText("Greeting")).toBeNull();
  expect(screen.getByText("No Mini Apps yet")).toBeInTheDocument();
});

test("selected app renders typed run form and sends values", async () => {
  const fetchMock = vi.spyOn(globalThis, "fetch");
  fetchMock.mockImplementation(async (input) => {
    const path = String(input);
    if (path === "/foxxycode/miniapps")
      return json({
        apps: [
          {
            id: "hello",
            name: "Greeting",
            state: "released",
            version: "1.0.0",
          },
        ],
      });
    if (path.includes("/draft"))
      return json({
        id: "hello",
        state: "draft",
        revision: "r1",
        metadata: { name: "Greeting" },
        inputs: [
          { id: "message", type: "string", title: "Message", required: true },
          { id: "token", type: "secret", title: "Token" },
        ],
        workflow: [],
        success: {},
      });
    if (path.includes("/versions/"))
      return json({
        id: "hello",
        state: "released",
        version: "1.0.0",
        revision: "release-r1",
        metadata: { name: "Greeting" },
        inputs: [],
        workflow: [],
        success: {},
      });
    if (path.includes("/authoring/source")) return json({});
    if (path.includes("/runs")) return json({ runs: [] });
    return json({});
  });
  render(
    <MiniAppsPage
      selectedAppId="hello"
      onNavigate={() => {}}
      onClose={() => {}}
    />,
  );
  expect(
    await screen.findByRole("heading", { name: "Greeting" }),
  ).toBeInTheDocument();
  expect(screen.getByText("Test generated workflow")).toBeInTheDocument();
  expect(screen.getByText("Run released version")).toBeInTheDocument();
  fireEvent.click(screen.getByText("Run released version"));
  await waitFor(() =>
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("/versions/1.0.0"),
      undefined,
    ),
  );
  fireEvent.click(screen.getByText("Close"));
  fireEvent.click(screen.getByText("Test generated workflow"));
  expect(
    await screen.findByRole("heading", { name: "Greeting" }),
  ).toBeInTheDocument();
  expect(screen.getByLabelText("Message")).toBeInTheDocument();
  expect(screen.getByLabelText("Token")).toHaveAttribute("type", "password");
  fireEvent.change(screen.getByLabelText("Message"), {
    target: { value: "Hello" },
  });
  fireEvent.click(screen.getByText("Test generated workflow"));
  await waitFor(() =>
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("/test-runs"),
      expect.objectContaining({ method: "POST" }),
    ),
  );
});

test("mini-agent proposes a draft change that can be applied locally", async () => {
  const fetchMock = vi.spyOn(globalThis, "fetch");
  fetchMock.mockImplementation(async (input, init) => {
    const path = String(input);
    if (path === "/foxxycode/miniapps")
      return json({
        apps: [{ id: "hello", name: "Greeting", state: "draft" }],
      });
    if (path.includes("/assistant")) {
      const body = JSON.parse(String(init?.body)) as {
        draft: Record<string, unknown>;
      };
      const draft = body.draft as {
        inputs?: unknown[];
      };
      return json({
        reply: "Добавил обязательное поле проекта.",
        changes: ["Added project input"],
        draft: {
          ...body.draft,
          inputs: [
            ...(draft.inputs ?? []),
            { id: "project", type: "string", title: "Project", required: true },
          ],
        },
      });
    }
    if (path.includes("/draft"))
      return json({
        id: "hello",
        state: "draft",
        revision: "r1",
        metadata: { name: "Greeting", goal: "Write greeting" },
        inputs: [],
        workflow: [],
        success: {},
      });
    if (path.includes("/authoring/source")) return json({});
    if (path.includes("/runs")) return json({ runs: [] });
    return json({});
  });

  render(
    <MiniAppsPage
      selectedAppId="hello"
      onNavigate={() => {}}
      onClose={() => {}}
    />,
  );
  expect(
    await screen.findByRole("heading", { name: "Mini App assistant" }),
  ).toBeInTheDocument();
  fireEvent.change(screen.getByLabelText("Message to the Mini App assistant"), {
    target: { value: "Добавь поле проекта" },
  });
  fireEvent.click(screen.getByText("Ask agent"));
  expect(
    await screen.findByText("Добавил обязательное поле проекта."),
  ).toBeInTheDocument();
  fireEvent.click(screen.getByText("Apply changes"));
  expect(screen.getByDisplayValue("Project")).toBeInTheDocument();
  expect(screen.getByText("Changes applied to the draft")).toBeInTheDocument();
});

test("mini-agent errors are announced as errors", async () => {
  const fetchMock = vi.spyOn(globalThis, "fetch");
  fetchMock.mockImplementation(async (input) => {
    const path = String(input);
    if (path === "/foxxycode/miniapps")
      return json({
        apps: [{ id: "hello", name: "Greeting", state: "draft" }],
      });
    if (path.includes("/assistant"))
      return json(
        { error: { message: "Assistant response is invalid" } },
        422,
      );
    if (path.includes("/draft"))
      return json({
        id: "hello",
        state: "draft",
        revision: "r1",
        metadata: { name: "Greeting", goal: "Write greeting" },
        inputs: [],
        workflow: [],
        success: {},
      });
    if (path.includes("/authoring/source")) return json({});
    if (path.includes("/runs")) return json({ runs: [] });
    return json({});
  });

  render(
    <MiniAppsPage
      selectedAppId="hello"
      onNavigate={() => {}}
      onClose={() => {}}
    />,
  );
  expect(
    await screen.findByRole("heading", { name: "Mini App assistant" }),
  ).toBeInTheDocument();
  fireEvent.change(screen.getByLabelText("Message to the Mini App assistant"), {
    target: { value: "Add a file name" },
  });
  fireEvent.click(screen.getByText("Ask agent"));
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Assistant response is invalid",
  );
});
