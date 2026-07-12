/**
 * Short, unique **`@`**-mention labels for dropped files. The composer textarea
 * holds the short label (e.g. **`@foo.ts`**) so the chip stays compact; the full
 * workspace-relative path is kept in a side map and only substituted back into the
 * outgoing text at send time (see **`expandDroppedMentions`**).
 */

export type MentionEntry = { label: string; pathRel: string };

/** Forward slashes, no leading **`./`** / slashes, no trailing slash. */
export function normalizeRelPath(pathRel: string): string {
  return pathRel
    .replace(/\\/g, "/")
    .replace(/^\.\//, "")
    .replace(/^\/+/, "")
    .replace(/\/+$/, "");
}

/**
 * Shortest unique label for **`pathRel`** given the already-mapped **`existing`** entries.
 * Starts from the basename and extends leftward by path segments (**`foo.ts`** →
 * **`bar/foo.ts`**) until the label is not already used by a **different** path. When
 * **`pathRel`** is already mapped, its current label is returned (so re-dropping the
 * same file reuses the same chip).
 */
export function uniqueMentionLabel(pathRel: string, existing: MentionEntry[]): string {
  const norm = normalizeRelPath(pathRel);
  for (const e of existing) {
    if (normalizeRelPath(e.pathRel) === norm) {
      return e.label;
    }
  }
  const segs = norm.split("/").filter((s) => s !== "");
  if (segs.length === 0) {
    return norm;
  }
  const usedByOther = new Set(
    existing
      .filter((e) => normalizeRelPath(e.pathRel) !== norm)
      .map((e) => e.label),
  );
  for (let take = 1; take <= segs.length; take++) {
    const label = segs.slice(segs.length - take).join("/");
    if (!usedByOther.has(label)) {
      return label;
    }
  }
  // Every suffix collides only if another entry already maps the full path to a
  // different label, which cannot happen (same path reuses its label above).
  return norm;
}
