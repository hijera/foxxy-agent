/** Pure helpers for the IDE editor-state reporter. Kept free of any `vscode`
 *  import so they can be unit-tested in a plain Node environment (mirrors how
 *  `diff/lineFragments.ts` is tested without the editor API). */

/** Cap on the reported selection text; the tail is kept (the server re-caps too). */
export const MAX_SELECTION_BYTES = 16 * 1024;

export interface EditorSelectionPayload {
  /** Absolute path of the file the selection is in. */
  file: string;
  /** 1-based first selected line. */
  startLine: number;
  /** 1-based last selected line, inclusive. */
  endLine: number;
  /** The selected text (capped, tail kept). */
  text: string;
}

export interface EditorStateSnapshot {
  /** Absolute paths of the open editor tabs, de-duplicated, focus first. */
  openFiles: string[];
  /** Absolute path of the focused editor, or "" when none. */
  activeFile: string;
  /** Current text selection, when non-empty. */
  selection?: EditorSelectionPayload;
}

/** Builds the selection payload from VS Code's 0-based selection coordinates.
 *  A selection whose end sits at column 0 of a later line does not include that
 *  line (the usual shift-down-to-line-start gesture). Empty or whitespace-only
 *  selections yield `undefined`. */
export function buildSelectionPayload(
  file: string,
  startLine0: number,
  endLine0: number,
  endChar: number,
  text: string,
): EditorSelectionPayload | undefined {
  const f = file.trim();
  if (f === "" || text.trim() === "") return undefined;
  let end0 = endLine0;
  if (endChar === 0 && end0 > startLine0) end0 -= 1;
  if (end0 < startLine0) return undefined;
  let t = text;
  if (t.length > MAX_SELECTION_BYTES) t = t.slice(t.length - MAX_SELECTION_BYTES);
  return { file: f, startLine: startLine0 + 1, endLine: end0 + 1, text: t };
}

/** Normalizes raw path candidates into a snapshot: trims, drops blanks, and
 *  de-duplicates open files while preserving order. The active file (when set)
 *  is guaranteed to appear first in `openFiles`. */
export function buildEditorStateSnapshot(
  rawOpenFiles: readonly (string | undefined | null)[],
  rawActiveFile: string | undefined | null,
  selection?: EditorSelectionPayload,
): EditorStateSnapshot {
  const activeFile = (rawActiveFile ?? "").trim();
  const seen = new Set<string>();
  const openFiles: string[] = [];
  const push = (p: string): void => {
    const t = p.trim();
    if (t !== "" && !seen.has(t)) {
      seen.add(t);
      openFiles.push(t);
    }
  };
  if (activeFile !== "") push(activeFile);
  for (const p of rawOpenFiles) {
    if (p) push(p);
  }
  return { openFiles, activeFile, ...(selection ? { selection } : {}) };
}

/** Deep-equality check used to skip redundant POSTs when nothing changed.
 *  Selection changes fire constantly in VS Code, so the selection must take
 *  part here or the debounce cannot contain the POST rate. */
export function sameSnapshot(a: EditorStateSnapshot, b: EditorStateSnapshot): boolean {
  if (a.activeFile !== b.activeFile) return false;
  if (a.openFiles.length !== b.openFiles.length) return false;
  for (let i = 0; i < a.openFiles.length; i++) {
    if (a.openFiles[i] !== b.openFiles[i]) return false;
  }
  const sa = a.selection;
  const sb = b.selection;
  if ((sa === undefined) !== (sb === undefined)) return false;
  if (sa && sb) {
    if (
      sa.file !== sb.file ||
      sa.startLine !== sb.startLine ||
      sa.endLine !== sb.endLine ||
      sa.text !== sb.text
    ) {
      return false;
    }
  }
  return true;
}

/** Serializes a snapshot to the `/foxxycode/ide/editor-state` request body. */
export function editorStateRequestBody(snap: EditorStateSnapshot): string {
  return JSON.stringify({
    openFiles: snap.openFiles,
    activeFile: snap.activeFile,
    ...(snap.selection ? { selection: snap.selection } : {}),
  });
}
