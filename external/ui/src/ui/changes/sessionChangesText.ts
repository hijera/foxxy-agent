import type { ChangeStatus } from "./types";

/**
 * Plural bucket for a file count.
 *
 * Russian needs three forms, so the count picks the message key instead of an
 * "s" being appended to a translated noun (same approach as tasks.chip.*).
 * English maps Few and Many onto the same string.
 */
export function fileCountKeySuffix(count: number): "One" | "Few" | "Many" {
  const mod100 = Math.abs(count) % 100;
  if (mod100 >= 11 && mod100 <= 14) {
    return "Many";
  }
  switch (mod100 % 10) {
    case 1:
      return "One";
    case 2:
    case 3:
    case 4:
      return "Few";
    default:
      return "Many";
  }
}

/** i18n key for "N files changed" in the card header. */
export function fileCountKey(count: number): string {
  return "changes.card.files" + fileCountKeySuffix(count);
}

/** i18n key for a file's status badge. */
export function statusKey(status: ChangeStatus): string {
  return "changes.status." + status;
}

/**
 * The last path segment, used as the row label with the folder shown beside it.
 * Both separators are handled: the server reports workspace-relative paths, and
 * on Windows those carry backslashes.
 */
export function baseName(path: string): string {
  const normalized = path.replace(/\\/g, "/");
  const cut = normalized.lastIndexOf("/");
  return cut === -1 ? normalized : normalized.slice(cut + 1);
}

/** The directory part of a path, or "" when the file sits at the workspace root. */
export function dirName(path: string): string {
  const normalized = path.replace(/\\/g, "/");
  const cut = normalized.lastIndexOf("/");
  return cut === -1 ? "" : normalized.slice(0, cut);
}
