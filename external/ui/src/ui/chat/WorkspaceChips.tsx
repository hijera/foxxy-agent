import { BranchIcon, FolderIcon, RemoteBranchIcon } from "./workspaceIcons";
import React, { useRef, useState, useSyncExternalStore } from "react";
import { createPortal } from "react-dom";
import {
  branchChipVisible,
  branchRows,
  firstLine,
  folderChipLabel,
  isSvnFolderCheckoutActive,
  isWorktreeBadgeActive,
  pathBasename,
  pathParent,
  sortedBranches,
  sortedSvnBranches,
  svnBranchLabel,
  svnChipVisible,
  type WorkspaceBranchFetch,
  type WorkspaceContext,
} from "./workspaceContext";
import {
  pushWorkspaceRecent,
  readWorkspaceRecents,
  type WorkspaceRecent,
} from "./workspaceRecents";
import { WorkspaceFolderModal } from "./WorkspaceFolderModal";
import { FitMiddleText } from "./FitMiddleText";
import {
  serverSnapshotShellStack,
  snapshotShellStack,
  subscribeShellStack,
} from "../shellBreakpoint";
import { isEditorEmbed } from "../embedShell";
import { useT } from "../i18n/I18nProvider";
import { useEscapeCloses } from "../components/useEscapeCloses";

type Props = {
  context: WorkspaceContext | null;
  worktreePref: boolean;
  svnFolderPref: boolean;
  onPickFolder: (path: string) => void;
  onPickBranch: (branch: string, worktree: boolean, remoteRef?: string) => void;
  onWorktreeToggle: () => void;
  onPickSvnBranch: (branch: string, separateFolder: boolean) => void;
  onSvnFolderToggle: () => void;
  /**
   * Fetches the workspace's remotes before the branch list shows: it resolves
   * once the context handed in is the one read after the fetch, with the
   * outcome to report. Without it the list shows the context as it is.
   */
  onRefreshBranches?: () => Promise<WorkspaceBranchFetch | null>;
  // Anchored dropdown direction; the docked composer opens the menu upward.
  opensUp?: boolean;
  // The workspace is chosen once: locked as soon as the conversation starts.
  locked?: boolean;
};

type MenuKind = "folder" | "branch" | "svn" | null;

/** Where the refresh of the open branch list stands. */
type BranchRefresh =
  | { state: "idle" }
  | { state: "loading" }
  | { state: "done"; outcome: WorkspaceBranchFetch | null };

// WorkspaceChips renders the picks of the plate over the composer before a
// WorkspaceChips renders the workspace context row above the composer field:
// a folder chip (recent folders + "Open folder…" browser), a branch chip
// (branch list inside git repos), a worktree checkbox, and — when an svn
// working copy is detected — an SVN chip with its own branch list and a
// separate-folder checkbox (Subversion's equivalent of a worktree).
export function WorkspaceChips(props: Props) {
  const { t } = useT();
  const [menuOpen, setMenuOpen] = useState<MenuKind>(null);
  const [menuAnchorRect, setMenuAnchorRect] = useState<DOMRect | null>(null);
  const [menuFilter, setMenuFilter] = useState("");
  const [recents, setRecents] = useState<WorkspaceRecent[]>([]);
  const [folderModalOpen, setFolderModalOpen] = useState(false);
  const [branchRefresh, setBranchRefresh] = useState<BranchRefresh>({
    state: "idle",
  });
  // Each opening of the branch list owns one refresh: an answer that arrives
  // after the list closed, or after it was opened again, is dropped.
  const refreshSeq = useRef(0);
  const isMobileShell = useSyncExternalStore(
    subscribeShellStack,
    snapshotShellStack,
    serverSnapshotShellStack,
  );
  const menuUseSheet = isMobileShell;
  // Editor plugins (VS Code / IntelliJ) fix the working directory to the open
  // IDE project, so folder switching is hidden there; branch/worktree stay.
  const hideFolderChip = isEditorEmbed();
  const closeMenu = () => {
    refreshSeq.current++;
    setBranchRefresh({ state: "idle" });
    setMenuOpen(null);
    setMenuAnchorRect(null);
    setMenuFilter("");
  };
  // Escape closes the folder or branch menu that is open.
  useEscapeCloses(menuOpen !== null, closeMenu);

  const ctx = props.context;
  if (!ctx) return null;
  const locked = Boolean(props.locked);

  // The branch list waits for the remotes to be fetched, so a branch pushed
  // since the last fetch is in it; a failed fetch keeps the list it had.
  const startBranchRefresh = () => {
    const refresh = props.onRefreshBranches;
    if (!refresh) {
      return;
    }
    const seq = refreshSeq.current;
    setBranchRefresh({ state: "loading" });
    const settle = (outcome: WorkspaceBranchFetch | null) => {
      if (seq === refreshSeq.current) {
        setBranchRefresh({ state: "done", outcome });
      }
    };
    refresh().then(settle, () => settle({ status: "failed" }));
  };

  const toggleMenu = (kind: Exclude<MenuKind, null>, trigger: HTMLElement) => {
    if (locked) {
      return;
    }
    if (menuOpen === kind) {
      closeMenu();
      return;
    }
    refreshSeq.current++;
    setBranchRefresh({ state: "idle" });
    setMenuOpen(kind);
    setMenuAnchorRect(trigger.getBoundingClientRect());
    setMenuFilter("");
    if (kind === "folder") {
      setRecents(readWorkspaceRecents());
    } else {
      startBranchRefresh();
    }
  };

  const pickFolder = (path: string) => {
    props.onPickFolder(path);
    setRecents(pushWorkspaceRecent({ path, name: pathBasename(path) || path }));
    setFolderModalOpen(false);
    closeMenu();
  };

  // The current workspace always appears in the Recent list (checked).
  const recentRows: WorkspaceRecent[] = recents.some((r) => r.path === ctx.path)
    ? recents
    : [{ path: ctx.path, name: folderChipLabel(ctx) }, ...recents];
  const filter = menuFilter.trim().toLocaleLowerCase();
  const filteredRecents = filter
    ? recentRows.filter((row) => row.name.toLocaleLowerCase().includes(filter))
    : recentRows;
  const branches = branchRows(ctx);
  const filteredBranches = filter
    ? branches.filter((row) => row.name.toLocaleLowerCase().includes(filter))
    : branches;
  // The cloud of a remote-only branch opens its row; while the list has any,
  // every row keeps that slot, so the names stay in one column as a filter
  // narrows the list.
  const markSlot = branches.some((row) => row.remoteOnly);
  const refreshing = branchRefresh.state === "loading";
  const refreshFailure =
    branchRefresh.state === "done" && branchRefresh.outcome?.status === "failed"
      ? branchRefresh.outcome
      : null;

  const dirClass = props.opensUp ? "opens-up" : "opens-down";
  const menuStyle =
    menuUseSheet || !menuAnchorRect
      ? undefined
      : props.opensUp
        ? {
            left: menuAnchorRect.left,
            bottom: window.innerHeight - menuAnchorRect.top + 8,
          }
        : { left: menuAnchorRect.left, top: menuAnchorRect.bottom + 8 };

  const showBranch = branchChipVisible(ctx);
  const worktreeActive = isWorktreeBadgeActive(
    ctx,
    !locked && props.worktreePref,
  );
  const showSvn = svnChipVisible(ctx);
  const pickClass = "workspace-chip workspace-bar-item workspace-bar-pick";
  const checkboxClass = "workspace-chip-checkbox workspace-bar-checkbox";
  const svnFolderActive = isSvnFolderCheckoutActive(ctx, props.svnFolderPref);
  const svnLabel = svnBranchLabel(ctx, t("composer.workspace.svnWorkingCopy"));
  const svnRevision = ctx.svn?.revision || 0;
  const svnTitle = svnRevision
    ? `${ctx.svn?.url || svnLabel} @ r${svnRevision}`
    : ctx.svn?.url || svnLabel;

  // In an editor embed with the folder chip hidden and no branch/worktree/svn
  // chips (plain workspace) there is nothing left to show — skip the empty row.
  if (hideFolderChip && !showBranch && !showSvn) {
    return null;
  }

  return (
    <div className="workspace-bar-picks">
      {!hideFolderChip ? (
        <button
          type="button"
          className={pickClass}
          data-testid="composer-workspace-chip"
          title={ctx.is_worktree && ctx.repo_root ? ctx.repo_root : ctx.path}
          aria-haspopup="menu"
          disabled={locked}
          aria-expanded={menuOpen === "folder"}
          onClick={(e) => toggleMenu("folder", e.currentTarget)}
        >
          <FolderIcon />
          <span className="workspace-bar-text">{folderChipLabel(ctx)}</span>
        </button>
      ) : null}

      {showBranch ? (
        <button
          type="button"
          className={pickClass}
          data-testid="composer-branch-chip"
          title={ctx.branch || t("composer.workspace.detached")}
          aria-haspopup="menu"
          disabled={locked}
          aria-expanded={menuOpen === "branch"}
          onClick={(e) => toggleMenu("branch", e.currentTarget)}
        >
          <BranchIcon worktree={ctx.is_worktree === true} />
          <FitMiddleText
            className="workspace-bar-text"
            text={ctx.branch || t("workspace.detached")}
          />
        </button>
      ) : null}

      {showBranch ? (
        <label
          className={`${pickClass} workspace-chip--check workspace-bar-check ${worktreeActive ? "is-active" : ""} ${locked || ctx.is_worktree ? "is-locked" : ""}`}
          data-testid="composer-worktree-chip"
          title={
            ctx.is_worktree
              ? t("composer.workspace.worktreeSessionTitle")
              : t("composer.workspace.worktreeToggleTitle")
          }
        >
          <input
            type="checkbox"
            className={checkboxClass}
            data-testid="composer-worktree-checkbox"
            checked={worktreeActive}
            disabled={locked || ctx.is_worktree}
            onChange={() => props.onWorktreeToggle()}
          />
          <span className="workspace-chip-label">
            {t("composer.workspace.worktree")}
          </span>
        </label>
      ) : null}

      {showSvn ? (
        <button
          type="button"
          className={pickClass}
          data-testid="composer-svn-chip"
          title={svnTitle}
          aria-haspopup="menu"
          disabled={locked}
          aria-expanded={menuOpen === "svn"}
          onClick={(e) => toggleMenu("svn", e.currentTarget)}
        >
          <span className="workspace-chip-icon" aria-hidden="true">
            <svg viewBox="0 0 16 16" width="12" height="12" fill="currentColor">
              <path d="M8 1.5 2.5 4v8L8 14.5 13.5 12V4L8 1.5Zm0 1.65 3.9 1.77L8 6.7 4.1 4.92 8 3.15ZM3.75 5.9 7.4 7.56v5.06L3.75 11V5.9Zm4.85 6.72V7.56L12.25 5.9V11L8.6 12.62Z" />
            </svg>
          </span>
          <span className="workspace-chip-label">{svnLabel}</span>
        </button>
      ) : null}

      {showSvn ? (
        <label
          className={`${pickClass} workspace-chip--check workspace-bar-check ${svnFolderActive ? "is-active" : ""} ${locked ? "is-locked" : ""}`}
          data-testid="composer-svn-folder-chip"
          title={t("composer.workspace.svnFolderToggleTitle")}
        >
          <input
            type="checkbox"
            className={checkboxClass}
            data-testid="composer-svn-folder-checkbox"
            checked={svnFolderActive}
            disabled={locked}
            onChange={() => props.onSvnFolderToggle()}
          />
          <span className="workspace-chip-label">
            {t("composer.workspace.svnFolder")}
          </span>
        </label>
      ) : null}

      {menuOpen && (menuUseSheet || menuAnchorRect)
        ? createPortal(
            <>
              <button
                type="button"
                className={`mode-menu-backdrop ${menuUseSheet ? "mode-menu-backdrop--scrim" : ""}`}
                aria-hidden="true"
                tabIndex={-1}
                onMouseDown={(e) => {
                  e.preventDefault();
                  closeMenu();
                }}
              />
              <div
                className={`mode-menu workspace-menu ${menuUseSheet ? "mode-menu--sheet" : `mode-menu--portal ${dirClass}`}`}
                role="menu"
                data-testid={
                  menuOpen === "folder"
                    ? "workspace-folder-menu"
                    : menuOpen === "svn"
                      ? "workspace-svn-menu"
                      : "workspace-branch-menu"
                }
                style={menuStyle}
              >
                {menuOpen === "folder" ? (
                  <>
                    <div className="mode-menu-group-label">
                      {t("composer.workspace.recent")}
                    </div>
                    <input
                      className="mode-menu-filter"
                      data-testid="workspace-recent-filter"
                      value={menuFilter}
                      placeholder={t("workspace.filterRecent")}
                      aria-label={t("workspace.filterRecent")}
                      autoFocus
                      onChange={(event) => setMenuFilter(event.target.value)}
                    />
                    <div className="mode-menu-scroll">
                      {filteredRecents.map((r) => (
                        <button
                          key={r.path}
                          type="button"
                          role="menuitem"
                          className={`mode-item workspace-recent-item ${r.path === ctx.path ? "is-selected" : ""}`}
                          data-testid={`workspace-recent-row-${r.name}`}
                          title={r.path}
                          onClick={() => {
                            if (r.path !== ctx.path) {
                              pickFolder(r.path);
                            } else {
                              closeMenu();
                            }
                          }}
                        >
                          <span className="workspace-recent-name">
                            {r.name}
                          </span>
                          {r.path === ctx.path ? (
                            <span
                              className="workspace-recent-check"
                              aria-hidden="true"
                            >
                              ✓
                            </span>
                          ) : null}
                        </button>
                      ))}
                      {filteredRecents.length === 0 ? (
                        <div
                          className="mode-menu-empty"
                          data-testid="workspace-recent-empty"
                        >
                          {t("workspace.noRecentMatch")}
                        </div>
                      ) : null}
                    </div>
                    <div className="workspace-menu-sep" aria-hidden="true" />
                    <button
                      type="button"
                      role="menuitem"
                      className="mode-item workspace-open-folder"
                      data-testid="workspace-open-folder"
                      onClick={() => {
                        closeMenu();
                        setFolderModalOpen(true);
                      }}
                    >
                      {t("composer.workspace.openFolder")}
                    </button>
                  </>
                ) : null}
                {menuOpen === "branch" ? (
                  <>
                    <input
                      className="mode-menu-filter"
                      data-testid="workspace-branch-filter"
                      value={menuFilter}
                      placeholder={t("workspace.filterBranches")}
                      aria-label={t("workspace.filterBranches")}
                      autoFocus
                      onChange={(event) => setMenuFilter(event.target.value)}
                    />
                    {refreshFailure ? (
                      <div
                        className="workspace-branch-warning"
                        data-testid="workspace-branch-refresh-warning"
                        role="status"
                        title={refreshFailure.error || undefined}
                      >
                        <span>{t("workspace.fetchFailed")}</span>
                        {firstLine(refreshFailure.error) ? (
                          <span className="workspace-branch-warning-detail">
                            {firstLine(refreshFailure.error)}
                          </span>
                        ) : null}
                      </div>
                    ) : null}
                    <div className="mode-menu-scroll">
                      {refreshing ? (
                        <div
                          className="mode-menu-empty workspace-branch-refreshing"
                          data-testid="workspace-branch-refreshing"
                          role="status"
                        >
                          {t("workspace.fetchingBranches")}
                        </div>
                      ) : (
                        <>
                          {filteredBranches.map(({ name, remoteOnly }) => (
                            <button
                              key={name}
                              type="button"
                              role="menuitem"
                              title={
                                remoteOnly
                                  ? `${name}\n${t("workspace.remoteOnlyHint")}`
                                  : name
                              }
                              className={`mode-item workspace-branch-item${name === ctx.branch ? " is-selected" : ""}`}
                              data-testid={`workspace-branch-row-${name}`}
                              onClick={() => {
                                if (name !== ctx.branch) {
                                  const ref = remoteOnly
                                    ? ctx.remote_branch_refs?.[name]
                                    : undefined;
                                  if (ref) {
                                    props.onPickBranch(
                                      name,
                                      props.worktreePref,
                                      ref,
                                    );
                                  } else {
                                    props.onPickBranch(
                                      name,
                                      props.worktreePref,
                                    );
                                  }
                                }
                                closeMenu();
                              }}
                            >
                              {markSlot ? (
                                <span className="workspace-branch-mark">
                                  {remoteOnly ? (
                                    <RemoteBranchIcon
                                      label={t("workspace.remoteOnly")}
                                    />
                                  ) : null}
                                </span>
                              ) : null}
                              <span className="workspace-branch-name">
                                {name}
                              </span>
                            </button>
                          ))}
                          {branches.length === 0 ? (
                            <div className="mode-menu-empty">
                              {t("workspace.noBranches")}
                            </div>
                          ) : null}
                          {branches.length > 0 &&
                          filteredBranches.length === 0 ? (
                            <div
                              className="mode-menu-empty"
                              data-testid="workspace-branch-empty"
                            >
                              {t("workspace.noBranchesMatch")}
                            </div>
                          ) : null}
                        </>
                      )}
                    </div>
                  </>
                ) : null}
                {menuOpen === "svn" ? (
                  <div className="mode-menu-scroll">
                    {sortedSvnBranches(ctx).map((b) => (
                      <button
                        key={b}
                        type="button"
                        role="menuitem"
                        title={b}
                        className={`mode-item ${b === ctx.svn?.branch ? "is-selected" : ""}`}
                        data-testid={`workspace-svn-branch-row-${b}`}
                        onClick={() => {
                          if (b !== ctx.svn?.branch || props.svnFolderPref) {
                            props.onPickSvnBranch(b, props.svnFolderPref);
                          }
                          closeMenu();
                        }}
                      >
                        {b}
                      </button>
                    ))}
                    {(ctx.svn?.branches || []).length === 0 ? (
                      <div className="mode-menu-empty">
                        {t("composer.workspace.svnNoBranches")}
                      </div>
                    ) : null}
                  </div>
                ) : null}
              </div>
            </>,
            document.body,
          )
        : null}

      <WorkspaceFolderModal
        open={folderModalOpen}
        startPath={pathParent(ctx.path)}
        onClose={() => setFolderModalOpen(false)}
        onPick={pickFolder}
      />
    </div>
  );
}
