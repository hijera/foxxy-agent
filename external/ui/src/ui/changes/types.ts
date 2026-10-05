/** Wire types for GET /foxxycode/sessions/{id}/changes and its detail route. */

export type ChangeStatus = "added" | "modified" | "deleted";

/** One file in the session change set, net of every turn. */
export interface ChangedFile {
  path: string;
  status: ChangeStatus;
  additions: number;
  deletions: number;
  /** No line diff exists; additions, deletions and patch are empty. */
  binary: boolean;
  /** The patch was cut at the server cap; the line counts still cover the file. */
  truncated: boolean;
  /** Present only when the request asked for it (list) or always (detail). */
  patch?: string;
}

export interface SessionChangeTotals {
  files: number;
  additions: number;
  deletions: number;
}

/**
 * Which edits a change set describes. The card always asks for the session; the
 * review window's switcher offers the other two.
 */
export type ChangeScope = "session" | "turn" | "uncommitted" | "all";

export const CHANGE_SCOPES: ChangeScope[] = [
  "session",
  "turn",
  "uncommitted",
  "all",
];

export interface SessionChanges {
  sessionId: string;
  files: ChangedFile[];
  totals: SessionChangeTotals;
  /** Echoed by the server; absent on a response from an older backend. */
  scope?: ChangeScope;
  /** Working-copy scopes only: files not shown because they are untracked -
   *  all of them under `uncommitted`, only those past the cap under `all`. */
  untracked?: number;
  /** Working-copy scopes only: false when the folder is under no VCS at all. */
  vcsAvailable?: boolean;
  /** Which system answered: "git", "svn", or "" when the folder is under none. */
  vcs?: string;
}

/** A single file with its unified patch, from the detail route. */
export interface SessionChangeDetail extends ChangedFile {
  patch: string;
}

export const EMPTY_SESSION_CHANGES: SessionChanges = {
  sessionId: "",
  files: [],
  totals: { files: 0, additions: 0, deletions: 0 },
};
