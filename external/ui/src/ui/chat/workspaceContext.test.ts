import { describe, expect, it } from "vitest";
import {
  branchChipVisible,
  folderChipLabel,
  isWorktreeBadgeActive,
  sortedBranches,
  worktreeForBranch,
  type WorkspaceContext,
} from "./workspaceContext";

const gitCtx: WorkspaceContext = {
  path: "/repos/foxxycode-agent",
  name: "foxxycode-agent",
  is_git_repo: true,
  is_worktree: false,
  repo_root: "/repos/foxxycode-agent",
  branch: "main",
  branches: ["zeta", "main", "feature/login"],
  worktrees: [
    { path: "/repos/foxxycode-agent", branch: "main", main: true },
    { path: "/home/.foxxycode/worktrees/foxxycode-agent/feature-login", branch: "feature/login", main: false },
  ],
};

describe("workspaceContext helpers", () => {
  it("labels the folder chip from name, path basename, or fallback", () => {
    expect(folderChipLabel(null)).toBe("workspace");
    expect(folderChipLabel(gitCtx)).toBe("foxxycode-agent");
    expect(
      folderChipLabel({ ...gitCtx, name: "", path: "/tmp/alpha" }),
    ).toBe("alpha");
  });

  it("shows the branch chip only inside git repositories", () => {
    expect(branchChipVisible(null)).toBe(false);
    expect(branchChipVisible({ ...gitCtx, is_git_repo: false })).toBe(false);
    expect(branchChipVisible(gitCtx)).toBe(true);
  });

  it("sorts branches with the current one first", () => {
    expect(sortedBranches(gitCtx)).toEqual(["main", "feature/login", "zeta"]);
    expect(sortedBranches({ ...gitCtx, branch: "zeta" })).toEqual([
      "zeta",
      "feature/login",
      "main",
    ]);
  });

  it("finds a non-main worktree for a branch", () => {
    expect(worktreeForBranch(gitCtx, "feature/login")?.path).toBe(
      "/home/.foxxycode/worktrees/foxxycode-agent/feature-login",
    );
    expect(worktreeForBranch(gitCtx, "main")).toBeNull();
    expect(worktreeForBranch(gitCtx, "zeta")).toBeNull();
  });

  it("marks the worktree badge active from context or preference", () => {
    expect(isWorktreeBadgeActive(null, false)).toBe(false);
    expect(isWorktreeBadgeActive(gitCtx, false)).toBe(false);
    expect(isWorktreeBadgeActive(gitCtx, true)).toBe(true);
    expect(isWorktreeBadgeActive({ ...gitCtx, is_worktree: true }, false)).toBe(true);
  });
});
