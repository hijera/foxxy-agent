/** Pure helper for the `@terminal` section of the composer `@`-menu. Kept free
 *  of any React import so it can be unit-tested in isolation (mirrors how the
 *  workspace picker helpers live outside `Composer.tsx`).
 *
 *  The mention grammar the backend resolves (see `terminalMentionRe` in
 *  `internal/agent/react.go`) is a bare `@terminal` (the active terminal) or
 *  `@terminal:<name>` where `<name>` is a whitespace-free token. So this helper
 *  only offers `@terminal:<name>` rows for terminals whose name has no spaces;
 *  the bare `@terminal` covers everything else. */

export interface TerminalRef {
  id: string;
  name: string;
  active?: boolean;
}

/** A synthetic picker row shaped like a workspace-file row so it can share the
 *  composer `@`-menu render path. `path_rel` is the literal mention token that
 *  gets inserted (without the leading `@`). */
export interface TerminalPickRow {
  name: string;
  path_rel: string;
  kind: "terminal";
}

const KEYWORD = "terminal";

/** Rows for the current `@`-prefix (the text after `@`, before the caret).
 *  Returns `[]` when the prefix cannot lead to a `@terminal` mention, so the
 *  file-search path is left untouched. Otherwise returns a bare `@terminal`
 *  row (while no `:name` selector has been typed) plus one `@terminal:<name>`
 *  row per matching, whitespace-free terminal name. */
export function terminalPickerRows(
  prefix: string,
  terminals: readonly TerminalRef[],
): TerminalPickRow[] {
  const p = prefix.trim().toLowerCase();
  const colon = p.indexOf(":");
  const beforeColon = colon >= 0 ? p.slice(0, colon) : p;
  // Engage only while the typed prefix is itself a prefix of "terminal"
  // (empty, "t", "te", ... "terminal"), or already past the ":" name selector.
  if (beforeColon !== "" && !KEYWORD.startsWith(beforeColon)) {
    return [];
  }
  if (terminals.length === 0) {
    return []; // no IDE terminals reported → no terminal rows at all
  }
  const namePart = colon >= 0 ? p.slice(colon + 1) : "";
  const rows: TerminalPickRow[] = [];
  if (colon < 0) {
    rows.push({ name: KEYWORD, path_rel: KEYWORD, kind: "terminal" });
  }
  for (const term of terminals) {
    const name = term.name.trim();
    if (name === "" || /\s/.test(name)) {
      continue; // names with spaces are not addressable via @terminal:<name>
    }
    if (namePart !== "" && !name.toLowerCase().includes(namePart)) {
      continue;
    }
    rows.push({ name, path_rel: `${KEYWORD}:${name}`, kind: "terminal" });
  }
  return rows;
}
