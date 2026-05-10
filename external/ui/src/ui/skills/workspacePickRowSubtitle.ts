/**
 * Secondary line under a workspace file/folder picker row: parent **`folder/`** only.
 * Root-level **`path_rel`** returns an empty string (no subtitle).
 */
export function workspacePickRowSubtitle(row: { path_rel: string }): string {
  const trimmed = row.path_rel.replace(/\/+$/, "");
  const lastSlash = trimmed.lastIndexOf("/");
  if (lastSlash < 0) {
    return "";
  }
  return trimmed.slice(0, lastSlash + 1);
}
