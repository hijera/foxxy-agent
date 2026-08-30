import { describe, expect, it } from "vitest";
import {
  branchChipVisible,
  cleanPathInput,
  folderChipLabel,
  isSvnFolderCheckoutActive,
  isWorktreeBadgeActive,
  pathParent,
  sortedBranches,
  sortedSvnBranches,
  svnBranchLabel,
  svnChipVisible,
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

// A branch folder checked out from SVN that also holds a git repository.
const svnCtx: WorkspaceContext = {
  ...gitCtx,
  is_svn_repo: true,
  svn: {
    available: true,
    wc_root: "/repos/foxxycode-agent",
    url: "https://svn.example.test/repo/branches/feature-x",
    relative_url: "^/branches/feature-x",
    repository_root: "https://svn.example.test/repo",
    revision: 42,
    branch: "branches/feature-x",
    branches: ["branches/release-1", "trunk", "branches/feature-x"],
  },
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

  it("shows the svn chip only inside svn working copies", () => {
    expect(svnChipVisible(null)).toBe(false);
    expect(svnChipVisible(gitCtx)).toBe(false);
    expect(svnChipVisible(svnCtx)).toBe(true);
  });

  it("keeps git and svn detection independent in a mixed workspace", () => {
    // The same folder is a git repository and an svn branch folder.
    expect(branchChipVisible(svnCtx)).toBe(true);
    expect(svnChipVisible(svnCtx)).toBe(true);
    expect(svnChipVisible({ ...gitCtx, is_git_repo: false, is_svn_repo: true })).toBe(
      true,
    );
  });

  it("labels the svn chip from the branch, falling back when the layout is unknown", () => {
    expect(svnBranchLabel(svnCtx, "svn")).toBe("branches/feature-x");
    expect(
      svnBranchLabel({ ...svnCtx, svn: { available: true, branch: "" } }, "svn"),
    ).toBe("svn");
    expect(svnBranchLabel(null, "svn")).toBe("svn");
  });

  it("sorts svn branches with the current one first, then trunk", () => {
    expect(sortedSvnBranches(svnCtx)).toEqual([
      "branches/feature-x",
      "trunk",
      "branches/release-1",
    ]);
    expect(
      sortedSvnBranches({
        ...svnCtx,
        svn: { ...svnCtx.svn!, branch: "trunk" },
      }),
    ).toEqual(["trunk", "branches/feature-x", "branches/release-1"]);
  });

  it("marks the svn branch-folder badge active from the preference", () => {
    expect(isSvnFolderCheckoutActive(null, true)).toBe(false);
    expect(isSvnFolderCheckoutActive(gitCtx, true)).toBe(false);
    expect(isSvnFolderCheckoutActive(svnCtx, false)).toBe(false);
    expect(isSvnFolderCheckoutActive(svnCtx, true)).toBe(true);
  });

  it("walks up posix paths", () => {
    expect(pathParent("/repos/coddy-agent")).toBe("/repos");
    expect(pathParent("/repos/coddy-agent/")).toBe("/repos");
    expect(pathParent("/repos")).toBe("/");
    expect(pathParent("/")).toBe("/");
    expect(pathParent("")).toBe("");
  });

  it("walks up windows paths without changing drive", () => {
    expect(pathParent("H:\\PycharmProjects\\work")).toBe("H:\\PycharmProjects");
    // The parent of a first-level folder is the drive root, not "H:" (which
    // means the drive's current directory) and not "/" (another volume).
    expect(pathParent("H:\\PycharmProjects")).toBe("H:\\");
    expect(pathParent("H:\\")).toBe("H:\\");
    expect(pathParent("C:/repos/app")).toBe("C:/repos");
  });

  it("stops at the top of a UNC share", () => {
    expect(pathParent("\\\\server\\share\\dir")).toBe("\\\\server\\share");
    expect(pathParent("\\\\server\\share")).toBe("\\\\server\\share");
  });

  it("cleans typed and pasted paths", () => {
    expect(cleanPathInput('  D:\\work  ')).toBe("D:\\work");
    expect(cleanPathInput('"D:\\work with spaces"')).toBe("D:\\work with spaces");
    expect(cleanPathInput('"')).toBe('"');
    expect(cleanPathInput("")).toBe("");
  });
});
