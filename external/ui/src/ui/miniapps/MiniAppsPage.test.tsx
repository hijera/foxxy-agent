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
