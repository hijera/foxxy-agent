import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { initLocale } from "../i18n/i18n";
import { SubagentsSection } from "./SubagentsSection";
import type { JsonSchema } from "./SchemaForm";

beforeEach(() => {
  initLocale("en");
});

afterEach(() => {
  cleanup();
  initLocale("en");
  vi.unstubAllGlobals();
});

const schema: JsonSchema = {
  type: "object",
  properties: {
    project_trust: { type: "string", title: "Project trust", enum: ["ask", "allow", "deny"] },
  },
};

const listResponse = {
  object: "foxxycode.subagent_list",
  workspace: "/work/repo",
  policy: "ask",
  items: [
    {
      name: "general",
      description: "General-purpose worker.",
      scope: "builtin",
      builtin: true,
      hidden: false,
      trust: "trusted",
      trusted: true,
      needs_approval: false,
    },
    {
      name: "reviewer",
      description: "Reviews a diff for correctness.",
      scope: "project",
      path: "/work/repo/.foxxycode/agents/reviewer.md",
      digest: "9f2ca1b3d4e5f607",
      builtin: false,
      hidden: false,
      trust: "needs_approval",
      trusted: false,
      needs_approval: true,
      tools: ["read", "grep"],
      permission_mode: "ask",
      timeout_seconds: 600,
      role_bytes: 4210,
    },
  ],
};

function stubFetch(responses?: unknown[]) {
  const calls: Array<{ url: string; method: string; body?: string }> = [];
  let index = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      calls.push({
        url: String(url),
        method: init?.method ?? "GET",
        ...(typeof init?.body === "string" ? { body: init.body } : {}),
      });
      const body = responses && index < responses.length ? responses[index++] : listResponse;
      return Promise.resolve({ ok: true, json: async () => body });
    }),
  );
  return calls;
}

function renderSection(workspacePath?: string) {
  return render(
    <SubagentsSection
      schema={schema}
      value={{ project_trust: "ask" }}
      onChange={() => {}}
      workspacePath={workspacePath}
    />,
  );
}

test("lists every definition and offers the shield only on the project one", async () => {
  stubFetch();
  renderSection("/work/repo");
  await waitFor(() => expect(screen.getByTestId("subagents-list")).toBeInTheDocument());

  expect(screen.getByTestId("subagent-row-general")).toBeInTheDocument();
  expect(screen.getByTestId("subagent-row-reviewer")).toBeInTheDocument();
  // A built-in needs no approval, so it carries no control at all.
  expect(screen.queryByTestId("subagent-trust-general")).toBeNull();
  expect(screen.getByTestId("subagent-trust-reviewer")).toBeInTheDocument();
  expect(screen.getByTestId("subagents-pending-hint")).toHaveTextContent("1 definition");
});

test("an unapproved definition shows its bounds and withholds its description", async () => {
  stubFetch();
  renderSection("/work/repo");
  const note = await screen.findByTestId("subagent-trust-note-reviewer");

  expect(note).toHaveTextContent("/work/repo/.foxxycode/agents/reviewer.md");
  expect(note).toHaveTextContent("read, grep");
  expect(note).toHaveTextContent("10m");
  expect(note).toHaveTextContent("4 KiB");
  // The file's own description is the place a hostile checkout would put
  // instructions; it stays out until the receipt exists.
  expect(screen.getByTestId("subagent-row-reviewer")).not.toHaveTextContent(
    "Reviews a diff for correctness",
  );
});

test("approving posts the workspace and reloads the catalog", async () => {
  const approved = {
    ...listResponse,
    items: listResponse.items.map((i) =>
      i.name === "reviewer"
        ? { ...i, trust: "trusted", trusted: true, needs_approval: false }
        : i,
    ),
  };
  // list, trust, list again
  const calls = stubFetch([listResponse, { object: "foxxycode.subagent", item: {} }, approved]);
  renderSection("/work/repo");
  await waitFor(() => expect(screen.getByTestId("subagent-trust-reviewer")).toBeInTheDocument());

  fireEvent.click(screen.getByTestId("subagent-trust-reviewer"));

  await waitFor(() => expect(calls.length).toBe(3));
  expect(calls[0]?.url).toBe("/foxxycode/subagents?cwd=%2Fwork%2Frepo");
  expect(calls[1]).toMatchObject({
    url: "/foxxycode/subagents/reviewer/trust",
    method: "POST",
    body: JSON.stringify({ cwd: "/work/repo" }),
  });
  // The refreshed row drops the approval notice.
  await waitFor(() =>
    expect(screen.queryByTestId("subagent-trust-note-reviewer")).toBeNull(),
  );
});

test("without a session workspace the server is left to answer for its own", async () => {
  const calls = stubFetch();
  renderSection(undefined);
  await waitFor(() => expect(calls.length).toBe(1));
  expect(calls[0]?.url).toBe("/foxxycode/subagents");
});

test("a failed catalog load says so instead of rendering an empty list", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation(() =>
      Promise.resolve({
        ok: false,
        status: 400,
        json: async () => ({ error: { message: "cwd must be an absolute path" } }),
      }),
    ),
  );
  renderSection("relative");
  await waitFor(() =>
    expect(screen.getByText("Could not load the subagent catalog.")).toBeInTheDocument(),
  );
  expect(screen.getByTestId("subagents-empty")).toBeInTheDocument();
});
