import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import { useT } from "../i18n/I18nProvider";
import { useEscapeCloses } from "../components/useEscapeCloses";
import { useRightDockEscape } from "../components/useRightDock";
import {
  phoneMaxWidthMediaQuery,
  serverSnapshotShellStack,
  snapshotShellStack,
  subscribeShellStack,
} from "../shellBreakpoint";
import {
  IconExpand,
  IconMore,
  IconRestore,
  IconTree,
} from "../files/windowIcons";
import { DiffFileSection } from "./DiffFileSection";
import { EditsTree } from "./EditsTree";
import {
  readDiffViewCookie,
  readEditsStartExpanded,
  writeDiffViewCookie,
} from "./diffViewPrefs";
import type { DiffView } from "./diffViewPrefs";
import { fetchSessionChangeFile, openSessionDiffInIDE } from "./api";
import { editorEmbedId } from "../embedShell";
import { environmentKey, getEnv } from "../env/remoteEnv";
import { treeOrder } from "./fileTree";
import type { ChangedFile } from "./types";
import { useDiscard } from "./useDiscard";
import { useWorkingCopy } from "./workingCopy";

/** Bound request concurrency when the user explicitly expands several diffs. */
const PATCH_CONCURRENCY = 4;

interface PatchState {
  key: string | ChangedFile[];
  patch?: string;
  error?: string;
  truncated?: boolean;
  revision?: string;
}

interface PatchRequest {
  key: PatchState["key"];
  controller: AbortController;
  result?: PatchState;
}

/** Loads only open text diffs and keeps the last patch visible during a refresh. */
function usePatches(
  sessionId: string,
  files: ChangedFile[],
  report: ChangedFile[],
): Map<string, PatchState> {
  const [patches, setPatches] = useState(() => new Map<string, PatchState>());
  const requests = useRef(new Map<string, PatchRequest>());

  useEffect(() => {
    const pending = requests.current;
    return () => {
      for (const request of pending.values()) request.controller.abort();
      pending.clear();
    };
  }, [sessionId]);

  useEffect(() => {
    const wanted = new Map<string, PatchState["key"]>();
    if (sessionId.trim()) {
      for (const file of files) {
        if (!file.binary) wanted.set(file.path, file.revision || report);
      }
    }
    const pending = requests.current;
    for (const [path, request] of pending) {
      if (wanted.get(path) !== request.key) {
        request.controller.abort();
        pending.delete(path);
      } else if (request.result && request.result === patches.get(path)) {
        pending.delete(path);
      }
    }
    if ([...patches.keys()].some((path) => !wanted.has(path))) {
      setPatches((prev) => new Map([...prev].filter(([path]) => wanted.has(path))));
    }
    for (const [path, key] of wanted) {
      if (pending.size >= PATCH_CONCURRENCY) break;
      const previous = patches.get(path);
      const upToDate = previous?.error
        ? previous.key === report
        : previous?.key === key || (typeof key === "string" && previous?.revision === key);
      if (upToDate || pending.has(path)) continue;
      const controller = new AbortController();
      const request: PatchRequest = { key, controller };
      pending.set(path, request);
      void fetchSessionChangeFile(sessionId, path, controller.signal).then((res) => {
        if (controller.signal.aborted) return;
        const result: PatchState = res.ok
          ? {
              key,
              patch: res.data.patch,
              truncated: res.data.truncated,
              ...(res.data.revision ? { revision: res.data.revision } : {}),
            }
          : { key: report, error: res.message };
        // Keep the request registered until its result reaches the render.
        request.result = result;
        setPatches((prev) => {
          const copy = new Map(prev);
          copy.set(path, result);
          return copy;
        });
      });
    }
  }, [sessionId, files, report, patches]);

  return patches;
}

/** A phone shows the tree or the diffs, one at a time (styles.css, Files window). */
function onPhone(): boolean {
  return (
    typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia(phoneMaxWidthMediaQuery).matches
  );
}

/** The last segment of the workspace folder, the way the window names it. */
function folderName(path: string): string {
  const trimmed = path.replace(/[\\/]+$/, "");
  return trimmed.split(/[\\/]/).pop() || trimmed;
}

function readAccepted(key: string): Map<string, string> {
  try {
    const value = JSON.parse(localStorage.getItem(key) || "[]");
    if (Array.isArray(value)) {
      return new Map(value.filter((pair) =>
        Array.isArray(pair) && typeof pair[0] === "string" && typeof pair[1] === "string",
      ));
    }
  } catch {
    // Storage can be disabled in an embedded webview.
  }
  return new Map();
}

/** How far under the top of the diffs a file still counts as the one on show. */
const ON_SHOW_SLACK_PX = 24;

/** The gap a picked file keeps from the top of the diffs: their own padding,
 *  the filter's 10px from the head. */
const JUMP_GAP_PX = 10;

/**
 * The edits window: file headers and requested diffs in one scrollable
 * document. Framed and headed the way
 * the Files window is (files/FilesView.tsx): the same sheet beside the rail,
 * the tree switch and the title on the left, the menu, the expand button and
 * the close button on the right, the tree of the changed files - and only
 * those - beside the diffs. The menu holds the rest: side by side, folding
 * every diff, discarding everything.
 */
export function EditsView(props: {
  sessionId: string;
  /** The chat's workspace folder, named under the title. */
  workspacePath?: string;
  initialPath?: string;
  cardReview?: boolean;
  onClose: () => void;
}) {
  const { t, tp } = useT();
  const { sessionId, onClose } = props;
  const workspacePath = props.workspacePath || "";
  const [openedPaths, setOpenedPaths] = useState<Set<string> | null>(null);
  // Read as the window opens rather than at module load, so a choice made in
  // another tab is picked up.
  const [view, setView] = useState<DiffView>(readDiffViewCookie);
  // A phone opens on the diffs; the tree switch brings the list over them.
  const [treeOpen, setTreeOpen] = useState(() => !onPhone());
  const [startExpanded] = useState(readEditsStartExpanded);
  const [expandedWindow, setExpandedWindow] = useState(startExpanded);
  const drawer = !!props.initialPath && !expandedWindow;
  const [menuOpen, setMenuOpen] = useState(false);
  const [active, setActive] = useState(props.initialPath || "");
  // A file picked in the tree, scrolled to once the diffs are on screen (on a
  // phone the tree gives way to them first).
  const [jump, setJump] = useState<{ path: string } | null>(() =>
    props.initialPath ? { path: props.initialPath } : null,
  );
  const completedJump = useRef<typeof jump>(null);

  const sectionRefs = useRef<Map<string, HTMLDivElement>>(new Map());
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const moreRef = useRef<HTMLButtonElement | null>(null);
  // The file picked last: marked while it is in sight, even when the diffs
  // cannot scroll it to their top (the last file of a short set).
  const pickedRef = useRef("");
  const wc = useWorkingCopy(sessionId, { enabled: true });
  const changes = wc.changes;
  const discard = useDiscard(sessionId);
  const env = getEnv();
  const embed = editorEmbedId();
  const nativeDiff = env.mode === "local" && (embed === "vscode" || embed === "intellij");
  const acceptanceKey = `foxxycode.accepted-edits:${JSON.stringify([environmentKey(env), sessionId, workspacePath])}`;
  const [accepted, setAccepted] = useState(() => readAccepted(acceptanceKey));

  useEffect(() => {
    setAccepted(readAccepted(acceptanceKey));
  }, [acceptanceKey]);
  const accept = (path: string, revision: string) => {
    const next = new Map(accepted);
    next.set(path, revision);
    setAccepted(next);
    try {
      localStorage.setItem(acceptanceKey, JSON.stringify([...next].slice(-500)));
    } catch {
      // The review choice still applies for this window.
    }
  };


  // Two columns of code need width the stacked shell does not have, so the
  // narrow layout reads every diff inline and hides the choice rather than
  // offering one that makes the diff unreadable.
  const narrow = useSyncExternalStore(
    subscribeShellStack,
    snapshotShellStack,
    serverSnapshotShellStack,
  );
  const effectiveView: DiffView = narrow || drawer ? "unified" : view;

  // Escape closes the window when nothing nearer took it: the confirmation
  // dialog over it answers its own, the tree's filter clears itself, the menu
  // closes itself first.
  useRightDockEscape(true, onClose);
  useEscapeCloses(menuOpen, () => {
    setMenuOpen(false);
    moreRef.current?.focus({ preventScroll: true });
  });

  // The diffs take the focus as the window opens, so the keys scroll them;
  // closing gives the focus back to whatever opened it.
  useEffect(() => {
    const opener =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    scrollRef.current?.focus({ preventScroll: true });
    return () => {
      if (opener?.isConnected) opener.focus({ preventScroll: true });
    };
  }, []);

  // The diffs read in the order the tree lists their files, so the mark that
  // follows the scroll walks the tree from top to bottom.
  const files = useMemo(() => {
    const byPath = new Map(changes.files.map((f) => [f.path, f]));
    return treeOrder(changes.files.map((f) => f.path)).flatMap((path) => {
      const file = byPath.get(path);
      return file ? [file] : [];
    });
  }, [changes.files]);
  // Until the reader picks or scrolls, the file at the top is the one on show.
  const onShow = files.some((f) => f.path === active) ? active : files[0]?.path || "";
  const shownFiles = useMemo(
    () => drawer ? files.filter((file) => file.path === onShow) : files,
    [drawer, files, onShow],
  );
  const firstPath = props.initialPath || (startExpanded ? files[0]?.path : undefined);
  const initialPaths = useMemo(() => new Set(firstPath ? [firstPath] : []), [firstPath]);
  const openPaths = openedPaths ?? initialPaths;
  const openFiles = useMemo(
    () => shownFiles.filter((file) => openPaths.has(file.path)),
    [shownFiles, openPaths],
  );
  const patches = usePatches(sessionId, openFiles, changes.files);

  const registerRef = useCallback((path: string, el: HTMLDivElement | null) => {
    if (el) {
      sectionRefs.current.set(path, el);
    } else {
      sectionRefs.current.delete(path);
    }
  }, []);

  const pick = (path: string) => {
    pickedRef.current = path;
    setActive(path);
    setOpenedPaths((prev) => {
      const current = prev ?? initialPaths;
      return current.size === 1 && current.has(path) ? prev : new Set([path]);
    });
    if (onPhone()) setTreeOpen(false);
    setJump({ path });
  };

  const openNative = (path: string) => {
    void openSessionDiffInIDE(sessionId, path).then((opened) => {
      if (!opened) pick(path);
    });
  };

  // The diffs alone scroll to the file: scrollIntoView would move every
  // scrollable box around them too, the window's own included. The file stays
  // pinned there while the diffs above it load or fold, until the reader
  // scrolls away: a pick made while the patches were still on their way was
  // carried down by every one that landed above it.
  const pinRef = useRef<{ path: string; top: number } | null>(null);
  const align = (path: string) => {
    const scroller = scrollRef.current;
    const section = sectionRefs.current.get(path);
    if (!scroller || !section) return;
    scroller.scrollTop +=
      section.getBoundingClientRect().top - scroller.getBoundingClientRect().top - JUMP_GAP_PX;
    pinRef.current = { path, top: scroller.scrollTop };
  };
  useLayoutEffect(() => {
    if (jump && completedJump.current !== jump && sectionRefs.current.has(jump.path)) {
      align(jump.path);
      completedJump.current = jump;
    }
    // align reads refs only.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [jump, shownFiles]);

  // The tree marks the file at the top of the diffs as the reader scrolls.
  const followScroll = () => {
    const scroller = scrollRef.current;
    if (!scroller) return;
    // A scroll the pin did not make is the reader's: the pin lets go.
    const pin = pinRef.current;
    if (pin && Math.abs(scroller.scrollTop - pin.top) > 1) pinRef.current = null;
    const port = scroller.getBoundingClientRect();
    const picked = pickedRef.current ? sectionRefs.current.get(pickedRef.current) : undefined;
    if (picked) {
      const r = picked.getBoundingClientRect();
      if (r.top < port.bottom && r.bottom > port.top) return;
      pickedRef.current = "";
    }
    const top = port.top + ON_SHOW_SLACK_PX;
    let onShowNow = "";
    for (const file of files) {
      const el = sectionRefs.current.get(file.path);
      if (!el) continue;
      if (el.getBoundingClientRect().top <= top) onShowNow = file.path;
      else break;
    }
    if (onShowNow && onShowNow !== active) setActive(onShowNow);
  };

  useLayoutEffect(() => {
    const pin = pinRef.current;
    if (pin) align(pin.path);
    // align reads refs only.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [patches, openPaths, files]);

  const allCollapsed = openFiles.length === 0;

  const toggleAll = () => {
    setOpenedPaths(
      allCollapsed ? new Set(shownFiles.map((file) => file.path)) : new Set(),
    );
  };

  const toggleOne = (path: string) => {
    setOpenedPaths((prev) => {
      const copy = new Set(prev ?? initialPaths);
      if (copy.has(path)) {
        copy.delete(path);
      } else {
        copy.add(path);
      }
      return copy;
    });
  };

  const toggleView = () => {
    const next: DiffView = view === "split" ? "unified" : "split";
    setView(next);
    writeDiffViewCookie(next);
  };

  const skipped = changes.skipped ?? 0;
  const unversioned = wc.loaded && !changes.vcs;
  // Discard all reaches the new files version control reports but the server did not
  // read, too.
  const discardable = changes.files.length > 0 || skipped > 0;
  const workspaceName = workspacePath ? folderName(workspacePath) : "";
  const fromMenu = (action: () => void) => () => {
    setMenuOpen(false);
    action();
  };

  return (
    <div
      className={[
        "files-dock-cluster edits-window",
        props.cardReview || props.initialPath ? "edits-window--card-review" : "",
        expandedWindow ? "is-expanded" : drawer ? "changes-panel" : "",
      ].filter(Boolean).join(" ")}
      data-testid="edits-view"
      role="dialog"
      aria-label={t("changes.viewer.title")}
    >
      <div className="files-header">
        <div className="files-title-block">
          <button
            type="button"
            className={"files-icon-btn" + (treeOpen ? " is-active" : "")}
            data-testid="edits-toggle-tree"
            aria-pressed={treeOpen}
            aria-label={t(treeOpen ? "changes.viewer.hideFiles" : "changes.viewer.showFiles")}
            title={t(treeOpen ? "changes.viewer.hideFiles" : "changes.viewer.showFiles")}
            onClick={() => setTreeOpen((v) => !v)}
          >
            <IconTree />
          </button>
          <div className="files-title-text">
            <h2 className="files-title">{t("changes.viewer.title")}</h2>
            <p
              className="files-subtitle"
              title={workspacePath ? t("files.workspace", { path: workspacePath }) : undefined}
            >
              {workspaceName ? (
                <span className="edits-folder">{workspaceName}</span>
              ) : null}
              <span className="edits-totals" data-testid="edits-totals">
                <span className="changes-add">{"+" + changes.totals.additions}</span>
                <span className="changes-del">{"−" + changes.totals.deletions}</span>
              </span>
            </p>
          </div>
        </div>
        <div className="files-actions">
          <div className="files-more-host">
            <button
              type="button"
              ref={moreRef}
              className={"files-icon-btn" + (menuOpen ? " is-active" : "")}
              data-testid="edits-more"
              aria-label={t("files.more")}
              title={t("files.more")}
              aria-haspopup="menu"
              aria-expanded={menuOpen}
              onClick={() => setMenuOpen((v) => !v)}
            >
              <IconMore />
            </button>
            {menuOpen ? (
              <>
                <button
                  type="button"
                  className="mode-menu-backdrop"
                  aria-hidden="true"
                  tabIndex={-1}
                  onMouseDown={(e) => {
                    e.preventDefault();
                    setMenuOpen(false);
                  }}
                />
                <div
                  className="mode-menu files-more-menu"
                  role="menu"
                  aria-label={t("files.more")}
                >
                  {narrow || drawer ? null : (
                    <button
                      type="button"
                      role="menuitemcheckbox"
                      aria-checked={view === "split"}
                      className="mode-item files-menu-item"
                      data-testid="edits-split"
                      onClick={fromMenu(toggleView)}
                    >
                      <span className="files-menu-check" aria-hidden>
                        {view === "split" ? "✓" : ""}
                      </span>
                      {t("changes.viewer.split")}
                    </button>
                  )}
                  <button
                    type="button"
                    role="menuitem"
                    className="mode-item files-menu-item"
                    data-testid="edits-toggle-all"
                    disabled={changes.files.length === 0}
                    onClick={fromMenu(toggleAll)}
                  >
                    <span className="files-menu-check" aria-hidden />
                    {allCollapsed
                      ? t("changes.viewer.expandAll")
                      : t("changes.viewer.collapseAll")}
                  </button>
                  {discardable ? (
                    <>
                      <div className="files-menu-sep" role="separator" />
                      <button
                        type="button"
                        role="menuitem"
                        className="mode-item files-menu-item files-menu-item--danger"
                        title={t("changes.discardAllTitle")}
                        disabled={discard.busy}
                        data-testid="edits-discard-all"
                        onClick={fromMenu(() => void discard.discardAll())}
                      >
                        <span className="files-menu-check" aria-hidden />
                        {t("changes.discardAll")}
                      </button>
                    </>
                  ) : null}
                </div>
              </>
            ) : null}
          </div>
          <button
            type="button"
            className={"files-icon-btn" + (expandedWindow ? " is-active" : "")}
            data-testid="edits-expand"
            aria-pressed={expandedWindow}
            aria-label={t(expandedWindow ? "files.restore" : "files.expand")}
            title={t(expandedWindow ? "files.restore" : "files.expand")}
            onClick={() => setExpandedWindow((v) => !v)}
          >
            {expandedWindow ? <IconRestore /> : <IconExpand />}
          </button>
          <button
            type="button"
            className="sessions-close files-close"
            data-testid="edits-close"
            aria-label={t("changes.viewer.close")}
            onClick={onClose}
          >
            ×
          </button>
        </div>
      </div>

      <div className={"files-layout" + (treeOpen ? " has-tree" : "")}>
        {treeOpen ? (
          <EditsTree files={changes.files} active={onShow} onPick={pick} />
        ) : null}

        <section className="files-main" aria-label={t("changes.viewer.title")}>
          <div
            ref={scrollRef}
            className="dv-scroll"
            data-testid="dv-scroll"
            tabIndex={-1}
            onScroll={followScroll}
          >
            {skipped > 0 ? (
              <div className="dv-banner" data-testid="dv-skipped">
                <span>{tp("changes.skipped", skipped)}</span>
              </div>
            ) : null}
            {discard.error ? (
              <div className="dv-note dv-note--error" role="alert" data-testid="dv-discard-error">
                {discard.error}
              </div>
            ) : null}

            {!wc.loaded && wc.error ? (
              <div className="dv-note dv-note--error" role="alert" data-testid="dv-error">
                {wc.error}
              </div>
            ) : unversioned ? (
              <div className="dv-note" data-testid="dv-no-vcs">
                {t("changes.viewer.noVcs")}
              </div>
            ) : changes.files.length === 0 && wc.loaded ? (
              <div className="dv-note" data-testid="dv-empty">
                {t("changes.empty")}
              </div>
            ) : (
              shownFiles.map((file) => {
                const state = patches.get(file.path);
                const collapsed = !openPaths.has(file.path);
                const revision = file.binary || collapsed ? file.revision : state?.revision;
                const isAccepted = !!revision && accepted.get(file.path) === revision;
                // A list refresh can arrive before its replacement patch. Keep the old
                // patch visible while disabling decisions for that superseded version.
                const canReview = !!revision && (!file.revision || revision === file.revision);
                return (
                  <DiffFileSection
                    key={file.path}
                    file={{
                      ...file,
                      truncated: state?.truncated ?? file.truncated,
                    }}
                    patch={state?.patch ?? ""}
                    loading={!state}
                    error={state?.error ?? ""}
                    collapsed={collapsed}
                    view={effectiveView}
                    onToggle={() => toggleOne(file.path)}
                    registerRef={registerRef}
                    discardBusy={discard.busy}
                    accepted={isAccepted}
                    onAccept={canReview && revision && !isAccepted ? () => accept(file.path, revision) : undefined}
                    onOpenNative={nativeDiff && !file.binary ? () => openNative(file.path) : undefined}
                    onDiscard={isAccepted || (!canReview && file.revision) ? undefined : () => void discard.discardFile(file.path, file.status, revision)}
                  />
                );
              })
            )}
          </div>
        </section>
      </div>
    </div>
  );
}
