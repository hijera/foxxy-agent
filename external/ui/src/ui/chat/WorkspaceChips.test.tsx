import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import React from "react";
import { WorkspaceChips } from "./WorkspaceChips";
import type { WorkspaceContext } from "./workspaceContext";
import { WORKSPACE_RECENTS_KEY, pushWorkspaceRecent } from "./workspaceRecents";
import { isEditorEmbed } from "../embedShell";

// Keep the real embedShell exports; override only the embed predicate so tests
// can flip between browser (false, the jsdom default) and plugin (true).
vi.mock("../embedShell", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../embedShell")>()),
  isEditorEmbed: vi.fn(() => false),
}));

const plainCtx: WorkspaceContext = {
  path: "/repos/plain",
  name: "plain",
  is_git_repo: false,
  is_worktree: false,
};

const gitCtx: WorkspaceContext = {
  path: "/repos/foxxycode-agent",
  name: "foxxycode-agent",
  is_git_repo: true,
  is_worktree: false,
  repo_root: "/repos/foxxycode-agent",
  branch: "main",
  branches: ["main", "feature/login"],
  worktrees: [{ path: "/repos/foxxycode-agent", branch: "main", main: true }],
};

function renderChips(overrides: Partial<React.ComponentProps<typeof WorkspaceChips>> = {}) {
  const props: React.ComponentProps<typeof WorkspaceChips> = {
    context: gitCtx,
    worktreePref: false,
    onPickFolder: vi.fn(),
    onPickBranch: vi.fn(),
    onWorktreeToggle: vi.fn(),
    ...overrides,
  };
  const utils = render(<WorkspaceChips {...props} />);
  return { ...utils, props };
}

beforeEach(() => {
  localStorage.removeItem(WORKSPACE_RECENTS_KEY);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.mocked(isEditorEmbed).mockReturnValue(false);
});

describe("WorkspaceChips", () => {
  it("shows only the environment chip without a context", () => {
    const { container } = renderChips({ context: null });
    // The workspace/branch/worktree chips need a context, but the environment selector stays
    // available (so the environment can be switched from the home screen before a workspace loads).
    expect(container.querySelector(".composer-context-chips")).not.toBeNull();
    expect(container.querySelector(".workspace-chip--env")).not.toBeNull();
    expect(screen.queryByTestId("composer-workspace-chip")).toBeNull();
    expect(screen.queryByTestId("composer-branch-chip")).toBeNull();
  });

  it("shows only the folder chip for a non-git workspace", () => {
    renderChips({ context: plainCtx });
    expect(screen.getByTestId("composer-workspace-chip").textContent).toContain("plain");
    expect(screen.queryByTestId("composer-branch-chip")).toBeNull();
    expect(screen.queryByTestId("composer-worktree-chip")).toBeNull();
  });

  it("hides the folder chip inside an editor embed but keeps branch and worktree", () => {
    vi.mocked(isEditorEmbed).mockReturnValue(true);
    renderChips();
    expect(screen.queryByTestId("composer-workspace-chip")).toBeNull();
    expect(screen.getByTestId("composer-branch-chip")).toBeTruthy();
    expect(screen.getByTestId("composer-worktree-checkbox")).toBeTruthy();
  });

  it("renders no chip row inside an editor embed for a non-git workspace", () => {
    vi.mocked(isEditorEmbed).mockReturnValue(true);
    const { container } = renderChips({ context: plainCtx });
    expect(container.querySelector(".composer-context-chips")).toBeNull();
  });

  it("renders the worktree control as a real checkbox", () => {
    renderChips();
    const box = screen.getByTestId("composer-worktree-checkbox") as HTMLInputElement;
    expect(box.type).toBe("checkbox");
    expect(box.checked).toBe(false);
  });

  it("checks and disables the worktree checkbox when the session lives in a worktree", () => {
    renderChips({ context: { ...gitCtx, is_worktree: true } });
    const box = screen.getByTestId("composer-worktree-checkbox") as HTMLInputElement;
    expect(box.checked).toBe(true);
    expect(box.disabled).toBe(true);
  });

  it("toggles the worktree preference through the checkbox", () => {
    const { props } = renderChips();
    fireEvent.click(screen.getByTestId("composer-worktree-checkbox"));
    expect(props.onWorktreeToggle).toHaveBeenCalledTimes(1);
  });

  it("opens the branch menu and picks a branch with the worktree preference", () => {
    const { props } = renderChips({ worktreePref: true });
    fireEvent.click(screen.getByTestId("composer-branch-chip"));
    const menu = screen.getByTestId("workspace-branch-menu");
    const rows = menu.querySelectorAll("[data-testid^='workspace-branch-row-']");
    expect(rows.length).toBe(2);
    fireEvent.click(screen.getByTestId("workspace-branch-row-feature/login"));
    expect(props.onPickBranch).toHaveBeenCalledWith("feature/login", true);
  });

  it("locks every control once the conversation started", () => {
    renderChips({ locked: true });
    expect(
      (screen.getByTestId("composer-workspace-chip") as HTMLButtonElement).disabled,
    ).toBe(true);
    expect(
      (screen.getByTestId("composer-branch-chip") as HTMLButtonElement).disabled,
    ).toBe(true);
    expect(
      (screen.getByTestId("composer-worktree-checkbox") as HTMLInputElement).disabled,
    ).toBe(true);
    fireEvent.click(screen.getByTestId("composer-workspace-chip"));
    expect(screen.queryByTestId("workspace-folder-menu")).toBeNull();
  });

  it("lists recent folders with the current workspace checked", () => {
    pushWorkspaceRecent({ path: "/repos/other", name: "other" });
    renderChips();
    fireEvent.click(screen.getByTestId("composer-workspace-chip"));
    const menu = screen.getByTestId("workspace-folder-menu");
    expect(menu.textContent).toContain("Recent");
    const current = screen.getByTestId("workspace-recent-row-foxxycode-agent");
    expect(current.className).toContain("is-selected");
    expect(screen.getByTestId("workspace-recent-row-other")).toBeTruthy();
  });

  it("picks a recent folder and remembers it", () => {
    pushWorkspaceRecent({ path: "/repos/other", name: "other" });
    const { props } = renderChips();
    fireEvent.click(screen.getByTestId("composer-workspace-chip"));
    fireEvent.click(screen.getByTestId("workspace-recent-row-other"));
    expect(props.onPickFolder).toHaveBeenCalledWith("/repos/other");
  });

  it("opens the folder browser modal from 'Open folder…' and picks a browsed folder", async () => {
    const listings: Record<string, unknown> = {
      "/repos": {
        path: "/repos",
        parent: "/",
        folders: [{ name: "other", path: "/repos/other" }],
      },
      "/repos/other": {
        path: "/repos/other",
        parent: "/repos",
        folders: [],
      },
      "/": {
        path: "/",
        parent: "/",
        folders: [{ name: "repos", path: "/repos" }],
      },
    };
    const fetchMock = vi.fn().mockImplementation((url: string) => {
      const u = new URL(String(url), "http://localhost");
      const p = u.searchParams.get("path") || "";
      return Promise.resolve({ ok: true, json: async () => listings[p] });
    });
    vi.stubGlobal("fetch", fetchMock);

    const { props } = renderChips();
    fireEvent.click(screen.getByTestId("composer-workspace-chip"));
    fireEvent.click(screen.getByTestId("workspace-open-folder"));

    // The browser starts at the parent of the current workspace.
    await waitFor(() => screen.getByTestId("workspace-folder-modal"));
    await waitFor(() => screen.getByTestId("workspace-modal-row-other"));

    // Navigate up and back down, then open the browsed folder.
    fireEvent.click(screen.getByTestId("workspace-modal-up"));
    await waitFor(() => screen.getByTestId("workspace-modal-row-repos"));
    fireEvent.click(screen.getByTestId("workspace-modal-row-repos"));
    await waitFor(() => screen.getByTestId("workspace-modal-row-other"));
    fireEvent.click(screen.getByTestId("workspace-modal-row-other"));
    await waitFor(() =>
      expect(screen.getByTestId("workspace-modal-open").textContent).toContain("Open"),
    );

    fireEvent.click(screen.getByTestId("workspace-modal-open"));
    expect(props.onPickFolder).toHaveBeenCalledWith("/repos/other");
    expect(screen.queryByTestId("workspace-folder-modal")).toBeNull();
  });

  it("cancels the folder browser modal without picking", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ path: "/repos", parent: "/", folders: [] }),
    });
    vi.stubGlobal("fetch", fetchMock);

    const { props } = renderChips();
    fireEvent.click(screen.getByTestId("composer-workspace-chip"));
    fireEvent.click(screen.getByTestId("workspace-open-folder"));
    await waitFor(() => screen.getByTestId("workspace-folder-modal"));
    fireEvent.click(screen.getByTestId("workspace-modal-cancel"));
    expect(screen.queryByTestId("workspace-folder-modal")).toBeNull();
    expect(props.onPickFolder).not.toHaveBeenCalled();
  });
});
