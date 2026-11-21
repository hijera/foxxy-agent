import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { WorkspaceBar, middleTruncate } from "./WorkspaceBar";
import type { WorkspaceContext } from "./workspaceContext";
import { initLocale } from "../i18n/i18n";

/**
 * The plate over the composer of a running chat names where the chat works:
 * the repository, the branch - its tooltip naming the worktree when the chat
 * runs in a linked one.
 */

afterEach(() => {
  cleanup();
  initLocale("en");
});

const repo: WorkspaceContext = {
  path: "/home/me/src/foxxycode-agent",
  name: "foxxycode-agent",
  is_git_repo: true,
  is_worktree: false,
  repo_root: "/home/me/src/foxxycode-agent",
  branch: "main",
};

const worktree: WorkspaceContext = {
  path: "/home/me/src/foxxycode-agent/.foxxycode/worktrees/feat-session-changes",
  name: "feat-session-changes",
  is_git_repo: true,
  is_worktree: true,
  repo_root: "/home/me/src/foxxycode-agent",
  branch: "feat/session-changes",
};

test("a repository: its name, the branch, and no worktree mark", () => {
  render(<WorkspaceBar context={repo} />);
  const name = screen.getByTestId("workspace-bar-repo");
  expect(name.textContent).toBe("foxxycode-agent");
  expect(name.getAttribute("title")).toBe("/home/me/src/foxxycode-agent");
  const branch = screen.getByTestId("workspace-bar-branch");
  expect(branch.textContent).toBe("main");
  expect(branch.getAttribute("title")).toBe("Branch main");
  expect(screen.queryByTestId("workspace-bar-worktree")).toBeNull();
  expect(screen.queryByTestId("workspace-bar-edits")).toBeNull();
});

test("a linked worktree: the repository's name, the branch glyph, and the worktree in the tooltip", () => {
  const { unmount } = render(<WorkspaceBar context={repo} />);
  const branchGlyph = screen.getByTestId("workspace-bar-branch").querySelector("svg")!.innerHTML;
  unmount();
  render(<WorkspaceBar context={worktree} />);
  expect(screen.getByTestId("workspace-bar-repo").textContent).toBe("foxxycode-agent");
  const branch = screen.getByTestId("workspace-bar-branch");
  const mark = screen.getByTestId("workspace-bar-worktree");
  expect(branch.firstElementChild).toBe(mark);
  // The mark is the branch glyph, in the plate's colour: the tooltip tells
  // a worktree apart.
  expect(mark.innerHTML).toBe(branchGlyph);
  expect(mark.getAttribute("class")).toBe("workspace-bar-icon");
  expect(branch.getAttribute("title")).toBe(
    "Worktree /home/me/src/foxxycode-agent/.foxxycode/worktrees/feat-session-changes on branch feat/session-changes",
  );
});

test("a folder in no repository has a name and nothing else", () => {
  const plain: WorkspaceContext = { path: "/tmp/demo", name: "demo", is_git_repo: false, is_worktree: false };
  render(<WorkspaceBar context={plain} />);
  expect(screen.getByTestId("workspace-bar-repo").textContent).toBe("demo");
  expect(screen.queryByTestId("workspace-bar-branch")).toBeNull();
  expect(screen.queryByTestId("workspace-bar-edits")).toBeNull();
});

test("a cut is never longer than asked, whatever the room", () => {
  for (const max of [1, 2, 3, 4, 5, 6]) {
    const cut = middleTruncate("feature/a-very-long-branch-name", max);
    expect(cut.length).toBeLessThanOrEqual(max);
    expect(cut.length).toBeGreaterThan(0);
  }
});

test("a long branch keeps both ends", () => {
  expect(middleTruncate("main", 24)).toBe("main");
  const long = "feature/a-very-long-branch-name-for-the-bar";
  const cut = middleTruncate(long, 24);
  expect(cut.length).toBe(24);
  expect(cut.startsWith("feature/a-")).toBe(true);
  expect(cut.endsWith("-for-the-bar")).toBe(true);
  expect(cut).toContain("…");
});
