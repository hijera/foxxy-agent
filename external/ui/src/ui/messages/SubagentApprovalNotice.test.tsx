import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { initLocale } from "../i18n/i18n";
import { SubagentApprovalNotice } from "./SubagentApprovalNotice";

beforeEach(() => {
  initLocale("en");
});

afterEach(() => {
  cleanup();
  initLocale("en");
  vi.unstubAllGlobals();
});

function catalog(needsApproval: boolean) {
  return {
    object: "foxxycode.subagent_list",
    workspace: "/work/repo",
    policy: "ask",
    items: [
      {
        name: "reviewer",
        description: "Reviews a diff.",
        scope: "project",
        path: "/work/repo/.foxxycode/agents/reviewer.md",
        builtin: false,
        hidden: false,
        trust: needsApproval ? "needs_approval" : "trusted",
        trusted: !needsApproval,
        needs_approval: needsApproval,
      },
    ],
  };
}

function stubFetch(responses: unknown[]) {
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
      const body = responses[Math.min(index++, responses.length - 1)];
      return Promise.resolve({ ok: true, json: async () => body });
    }),
  );
  return calls;
}

test("offers the approval when the catalog says the definition is waiting", async () => {
  stubFetch([catalog(true)]);
  render(<SubagentApprovalNotice agentName="reviewer" workspacePath="/work/repo" />);

  const notice = await screen.findByTestId("subagent-approval-reviewer");
  expect(notice).toHaveTextContent("reviewer");
  expect(notice).toHaveTextContent("/work/repo/.foxxycode/agents/reviewer.md");
  expect(screen.getByTestId("subagent-approval-approve-reviewer")).toBeInTheDocument();
  expect(screen.getByTestId("subagent-approval-settings-reviewer")).toBeInTheDocument();
});

test("renders nothing when the spawn failed for some other reason", async () => {
  const calls = stubFetch([catalog(false)]);
  render(<SubagentApprovalNotice agentName="reviewer" workspacePath="/work/repo" />);
  await waitFor(() => expect(calls.length).toBe(1));
  expect(screen.queryByTestId("subagent-approval-reviewer")).toBeNull();
});

test("renders nothing when the catalog cannot be reached", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation(() => Promise.reject(new Error("offline"))),
  );
  render(<SubagentApprovalNotice agentName="reviewer" workspacePath="/work/repo" />);
  await waitFor(() =>
    expect(screen.queryByTestId("subagent-approval-reviewer")).toBeNull(),
  );
});

test("approving posts the workspace and never retries the spawn", async () => {
  const calls = stubFetch([catalog(true), { object: "foxxycode.subagent", item: {} }]);
  render(<SubagentApprovalNotice agentName="reviewer" workspacePath="/work/repo" />);
  fireEvent.click(await screen.findByTestId("subagent-approval-approve-reviewer"));

  await waitFor(() => expect(calls.length).toBe(2));
  expect(calls[1]).toMatchObject({
    url: "/foxxycode/subagents/reviewer/trust",
    method: "POST",
    body: JSON.stringify({ cwd: "/work/repo" }),
  });
  // The confirmation asks the user to request the run again; nothing is
  // started on their behalf.
  await waitFor(() =>
    expect(screen.getByTestId("subagent-approval-reviewer")).toHaveTextContent(
      "Ask again to run it",
    ),
  );
  expect(screen.queryByTestId("subagent-approval-approve-reviewer")).toBeNull();
  expect(calls.filter((c) => c.method === "POST").length).toBe(1);
});
