/** Pure helpers for the IDE terminal-state reporter. Kept free of any `vscode`
 *  import so they can be unit-tested in a plain Node environment (mirrors
 *  `ide/editorStatePayload.ts`). */

export interface TerminalEntry {
  /** Reporter-stable id for the terminal. */
  id: string;
  /** Terminal title. */
  name: string;
  /** Shell path or name, when known. */
  shell?: string;
  /** Terminal working directory, when known. */
  cwd?: string;
  /** Most recently run command, when known. */
  lastCommand?: string;
  /** Bounded tail of recent output (may be empty without shell integration). */
  output: string;
  /** Whether this is the focused terminal. */
  active: boolean;
}

export interface TerminalSnapshot {
  terminals: TerminalEntry[];
}

/** Appends `chunk` to `prev`, keeping only the last `maxBytes` characters so a
 *  long-running terminal cannot grow the buffer without bound. */
export function appendBounded(prev: string, chunk: string, maxBytes: number): string {
  const combined = prev + chunk;
  if (combined.length <= maxBytes) {
    return combined;
  }
  return combined.slice(combined.length - maxBytes);
}

// OSC sequences (Operating System Command), BEL () or ST (\\)
// terminated. Covers VS Code shell-integration OSC 633 markers plus title /
// hyperlink OSCs. Built from string escapes so the source has no raw control
// bytes.
const OSC_RE = new RegExp(
  "\\u001b\\][^\\u0007\\u001b]*(?:\\u0007|\\u001b\\\\)",
  "g",
);
// CSI and other ANSI escape sequences (colours, cursor moves, etc.).
const CSI_RE = new RegExp(
  "[\\u001b\\u009b][[\\]()#;?]*(?:\\d{1,4}(?:;\\d{0,4})*)?[0-9A-PR-TZcf-ntqry=><~]",
  "g",
);
// Bare carriage returns from progress bars (keep CRLF newlines intact).
const CR_RE = new RegExp("\\r(?=[^\\n])", "g");

/** Removes ANSI colour codes and shell-integration OSC sequences from terminal
 *  output so the model sees plain text. */
export function stripAnsi(s: string): string {
  return s.replace(OSC_RE, "").replace(CSI_RE, "").replace(CR_RE, "");
}

/** Deep-equality check used to skip redundant POSTs when nothing changed. */
export function sameTerminalSnapshot(a: TerminalSnapshot, b: TerminalSnapshot): boolean {
  if (a.terminals.length !== b.terminals.length) {
    return false;
  }
  for (let i = 0; i < a.terminals.length; i++) {
    const x = a.terminals[i];
    const y = b.terminals[i];
    if (
      x.id !== y.id ||
      x.name !== y.name ||
      x.active !== y.active ||
      (x.lastCommand ?? "") !== (y.lastCommand ?? "") ||
      x.output !== y.output
    ) {
      return false;
    }
  }
  return true;
}

/** Serializes a snapshot to the `/foxxycode/ide/terminal-state` request body. */
export function terminalStateRequestBody(snap: TerminalSnapshot): string {
  return JSON.stringify({ terminals: snap.terminals });
}
