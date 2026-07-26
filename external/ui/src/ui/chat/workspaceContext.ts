// Workspace context helpers for the composer chips (folder / branch / worktree).
// Shapes mirror GET /foxxycode/workspace/context and /foxxycode/workspace/folders.

export type WorkspaceWorktree = {
  path: string;
  branch: string;
  main: boolean;
};

/** Subversion state of the workspace, detected independently of git. */
export type WorkspaceSvn = {
  available: boolean;
  wc_root?: string;
  url?: string;
  relative_url?: string;
  repository_root?: string;
  revision?: number;
  branch?: string;
  branches?: string[];
  nested?: boolean;
};

export type WorkspaceContext = {
  path: string;
  name: string;
  is_git_repo: boolean;
  is_worktree: boolean;
  repo_root?: string;
  branch?: string;
  branches?: string[];
  worktrees?: WorkspaceWorktree[];
  is_svn_repo?: boolean;
  svn?: WorkspaceSvn;
};

export type WorkspaceFolderRow = { name: string; path: string };

export type WorkspaceFolderListing = {
  path: string;
  parent: string;
  folders: WorkspaceFolderRow[];
};

export function pathBasename(p: string): string {
  const trimmed = (p || "").replace(/[/\\]+$/, "");
  const idx = Math.max(trimmed.lastIndexOf("/"), trimmed.lastIndexOf("\\"));
  return idx >= 0 ? trimmed.slice(idx + 1) : trimmed;
}

export function pathParent(p: string): string {
  const trimmed = (p || "").replace(/[/\\]+$/, "");
  const idx = trimmed.lastIndexOf("/");
  if (idx <= 0) {
    return "/";
  }
  return trimmed.slice(0, idx);
}

export function folderChipLabel(ctx: WorkspaceContext | null): string {
  if (!ctx) {
    return "workspace";
  }
  const name = (ctx.name || "").trim() || pathBasename(ctx.path);
  return name || "workspace";
}

export function branchChipVisible(ctx: WorkspaceContext | null): boolean {
  return Boolean(ctx?.is_git_repo);
}

// sortedBranches lists the current branch first, the rest alphabetically.
export function sortedBranches(ctx: WorkspaceContext): string[] {
  const branches = [...(ctx.branches || [])];
  branches.sort((a, b) => a.localeCompare(b));
  const current = (ctx.branch || "").trim();
  if (!current) {
    return branches;
  }
  return [current, ...branches.filter((b) => b !== current)];
}

// worktreeForBranch returns the linked (non-main) worktree holding branch.
export function worktreeForBranch(
  ctx: WorkspaceContext,
  branch: string,
): WorkspaceWorktree | null {
  for (const wt of ctx.worktrees || []) {
    if (!wt.main && wt.branch === branch) {
      return wt;
    }
  }
  return null;
}

// isWorktreeBadgeActive: the chip lights up when the session already lives in
// a worktree, or when the user opted future branch switches into worktrees.
export function isWorktreeBadgeActive(
  ctx: WorkspaceContext | null,
  worktreePref: boolean,
): boolean {
  if (!ctx || !ctx.is_git_repo) {
    return false;
  }
  return ctx.is_worktree || worktreePref;
}

// The SVN chip renders next to the git chip whenever a working copy was found.
// Both chips can be visible at once: a branch folder checked out from SVN very
// often also holds a git repository.
export function svnChipVisible(ctx: WorkspaceContext | null): boolean {
  return Boolean(ctx?.is_svn_repo);
}

// svnBranchLabel shows the branch (trunk, branches/<name>); an unrecognised
// repository layout falls back to the working copy revision.
export function svnBranchLabel(
  ctx: WorkspaceContext | null,
  fallback: string,
): string {
  const branch = (ctx?.svn?.branch || "").trim();
  if (branch) {
    return branch;
  }
  return fallback;
}

// sortedSvnBranches lists the current branch first, then trunk, then the rest
// alphabetically.
export function sortedSvnBranches(ctx: WorkspaceContext): string[] {
  const branches = [...(ctx.svn?.branches || [])];
  branches.sort((a, b) => {
    if (a === "trunk") return -1;
    if (b === "trunk") return 1;
    return a.localeCompare(b);
  });
  const current = (ctx.svn?.branch || "").trim();
  if (!current) {
    return branches;
  }
  return [current, ...branches.filter((b) => b !== current)];
}

// Subversion has no worktrees: the equivalent is checking a branch out into its
// own folder, which is what the checkbox next to the SVN chip requests.
export function isSvnFolderCheckoutActive(
  ctx: WorkspaceContext | null,
  folderPref: boolean,
): boolean {
  if (!ctx || !ctx.is_svn_repo) {
    return false;
  }
  return folderPref;
}
