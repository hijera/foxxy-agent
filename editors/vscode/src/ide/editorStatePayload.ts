/** Pure helpers for the IDE editor-state reporter. Kept free of any `vscode`
 *  import so they can be unit-tested in a plain Node environment (mirrors how
 *  `diff/lineFragments.ts` is tested without the editor API). */

export interface EditorStateSnapshot {
  /** Absolute paths of the open editor tabs, de-duplicated, focus first. */
  openFiles: string[];
  /** Absolute path of the focused editor, or "" when none. */
  activeFile: string;
}

/** Normalizes raw path candidates into a snapshot: trims, drops blanks, and
 *  de-duplicates open files while preserving order. The active file (when set)
 *  is guaranteed to appear first in `openFiles`. */
export function buildEditorStateSnapshot(
  rawOpenFiles: readonly (string | undefined | null)[],
  rawActiveFile: string | undefined | null,
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
  return { openFiles, activeFile };
}

/** Deep-equality check used to skip redundant POSTs when nothing changed. */
export function sameSnapshot(a: EditorStateSnapshot, b: EditorStateSnapshot): boolean {
  if (a.activeFile !== b.activeFile) return false;
  if (a.openFiles.length !== b.openFiles.length) return false;
  for (let i = 0; i < a.openFiles.length; i++) {
    if (a.openFiles[i] !== b.openFiles[i]) return false;
  }
  return true;
}

/** Serializes a snapshot to the `/foxxycode/ide/editor-state` request body. */
export function editorStateRequestBody(snap: EditorStateSnapshot): string {
  return JSON.stringify({ openFiles: snap.openFiles, activeFile: snap.activeFile });
}
