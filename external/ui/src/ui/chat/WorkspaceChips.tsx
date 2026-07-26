import React, { useState, useSyncExternalStore } from "react";
import { createPortal } from "react-dom";
import {
  branchChipVisible,
  folderChipLabel,
  isSvnFolderCheckoutActive,
  isWorktreeBadgeActive,
  pathBasename,
  pathParent,
  sortedBranches,
  sortedSvnBranches,
  svnBranchLabel,
  svnChipVisible,
  type WorkspaceContext,
} from "./workspaceContext";
import {
  pushWorkspaceRecent,
  readWorkspaceRecents,
  type WorkspaceRecent,
} from "./workspaceRecents";
import { EnvironmentChip } from "./EnvironmentChip";
import { WorkspaceFolderModal } from "./WorkspaceFolderModal";
import {
  serverSnapshotShellStack,
  snapshotShellStack,
  subscribeShellStack,
} from "../shellBreakpoint";
import { isEditorEmbed } from "../embedShell";
import { useT } from "../i18n/I18nProvider";

type Props = {
  context: WorkspaceContext | null;
  worktreePref: boolean;
  svnFolderPref: boolean;
  onPickFolder: (path: string) => void;
  onPickBranch: (branch: string, worktree: boolean) => void;
  onWorktreeToggle: () => void;
  onPickSvnBranch: (branch: string, separateFolder: boolean) => void;
  onSvnFolderToggle: () => void;
  // Anchored dropdown direction; the docked composer opens the menu upward.
  opensUp?: boolean;
  // The workspace is chosen once: locked as soon as the conversation starts.
  locked?: boolean;
};

type MenuKind = "folder" | "branch" | "svn" | null;

// WorkspaceChips renders the workspace context row above the composer field:
// a folder chip (recent folders + "Open folder…" browser), a branch chip
// (branch list inside git repos), a worktree checkbox, and — when an svn
// working copy is detected — an SVN chip with its own branch list and a
// separate-folder checkbox (Subversion's equivalent of a worktree).
export function WorkspaceChips(props: Props) {
  const { t } = useT();
  const [menuOpen, setMenuOpen] = useState<MenuKind>(null);
  const [menuAnchorRect, setMenuAnchorRect] = useState<DOMRect | null>(null);
  const [recents, setRecents] = useState<WorkspaceRecent[]>([]);
  const [folderModalOpen, setFolderModalOpen] = useState(false);
  const isMobileShell = useSyncExternalStore(
    subscribeShellStack,
    snapshotShellStack,
    serverSnapshotShellStack,
  );
  const menuUseSheet = isMobileShell;
  // Editor plugins (VS Code / IntelliJ) fix the working directory to the open
  // IDE project, so folder switching is hidden there; branch/worktree stay.
  const hideFolderChip = isEditorEmbed();

  const ctx = props.context;
  if (!ctx) {
    // The environment selector stays available even before a workspace context loads (or in an
    // API-only/remote state), so users can switch environments from the home screen.
    return (
      <div className="composer-context-chips">
        <EnvironmentChip />
      </div>
    );
  }
  const locked = Boolean(props.locked);

  const closeMenu = () => {
    setMenuOpen(null);
    setMenuAnchorRect(null);
  };

  const toggleMenu = (kind: Exclude<MenuKind, null>, trigger: HTMLElement) => {
    if (locked) {
      return;
    }
    if (menuOpen === kind) {
      closeMenu();
      return;
    }
    setMenuOpen(kind);
    setMenuAnchorRect(trigger.getBoundingClientRect());
    if (kind === "folder") {
      setRecents(readWorkspaceRecents());
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
  const worktreeActive = isWorktreeBadgeActive(ctx, props.worktreePref);
  const showSvn = svnChipVisible(ctx);
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
    <div className="composer-context-chips">
      <EnvironmentChip />
      {!hideFolderChip ? (
        <button
          type="button"
          className="workspace-chip"
          data-testid="composer-workspace-chip"
          title={ctx.path}
          aria-haspopup="menu"
          disabled={locked}
          onClick={(e) => toggleMenu("folder", e.currentTarget)}
        >
          <span className="workspace-chip-icon" aria-hidden="true">
            <svg viewBox="0 0 16 16" width="12" height="12" fill="currentColor">
              <path d="M1.75 2.5h4.3l1.4 1.5h6.8c.41 0 .75.34.75.75v8c0 .41-.34.75-.75.75H1.75a.75.75 0 0 1-.75-.75v-9.5c0-.41.34-.75.75-.75Z" />
            </svg>
          </span>
          <span className="workspace-chip-label">{folderChipLabel(ctx)}</span>
        </button>
      ) : null}

      {showBranch ? (
        <button
          type="button"
          className="workspace-chip"
          data-testid="composer-branch-chip"
          title={ctx.branch || t("composer.workspace.detached")}
          aria-haspopup="menu"
          disabled={locked}
          onClick={(e) => toggleMenu("branch", e.currentTarget)}
        >
          <span className="workspace-chip-icon" aria-hidden="true">
            <svg viewBox="0 0 16 16" width="12" height="12" fill="currentColor">
              <path d="M5 3.25a1.75 1.75 0 1 1-2.5-1.58V3.25a3.25 3.25 0 0 0 3.25 3.25h3.5c.97 0 1.75.78 1.75 1.75v.42a1.75 1.75 0 1 1-1.5 0V8.25a.25.25 0 0 0-.25-.25h-3.5A4.73 4.73 0 0 1 3.5 7.1v3.23a1.75 1.75 0 1 1-1.5 0V4.83A1.75 1.75 0 0 1 5 3.25Z" />
            </svg>
          </span>
          <span className="workspace-chip-label">
            {ctx.branch || t("composer.workspace.detached")}
          </span>
        </button>
      ) : null}

      {showBranch ? (
        <label
          className={`workspace-chip workspace-chip--check ${worktreeActive ? "is-active" : ""} ${locked || ctx.is_worktree ? "is-locked" : ""}`}
          data-testid="composer-worktree-chip"
          title={
            ctx.is_worktree
              ? t("composer.workspace.worktreeSessionTitle")
              : t("composer.workspace.worktreeToggleTitle")
          }
        >
          <input
            type="checkbox"
            className="workspace-chip-checkbox"
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
          className="workspace-chip"
          data-testid="composer-svn-chip"
          title={svnTitle}
          aria-haspopup="menu"
          disabled={locked}
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
          className={`workspace-chip workspace-chip--check ${svnFolderActive ? "is-active" : ""} ${locked ? "is-locked" : ""}`}
          data-testid="composer-svn-folder-chip"
          title={t("composer.workspace.svnFolderToggleTitle")}
        >
          <input
            type="checkbox"
            className="workspace-chip-checkbox"
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
                    <div className="mode-menu-scroll">
                      {recentRows.map((r) => (
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
                          <span className="workspace-recent-name">{r.name}</span>
                          {r.path === ctx.path ? (
                            <span className="workspace-recent-check" aria-hidden="true">
                              ✓
                            </span>
                          ) : null}
                        </button>
                      ))}
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
                  <div className="mode-menu-scroll">
                    {sortedBranches(ctx).map((b) => (
                      <button
                        key={b}
                        type="button"
                        role="menuitem"
                        title={b}
                        className={`mode-item ${b === ctx.branch ? "is-selected" : ""}`}
                        data-testid={`workspace-branch-row-${b}`}
                        onClick={() => {
                          if (b !== ctx.branch) {
                            props.onPickBranch(b, props.worktreePref);
                          }
                          closeMenu();
                        }}
                      >
                        {b}
                      </button>
                    ))}
                    {(ctx.branches || []).length === 0 ? (
                      <div className="mode-menu-empty">
                        {t("composer.workspace.noBranches")}
                      </div>
                    ) : null}
                  </div>
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
