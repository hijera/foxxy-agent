/**
 * Expands short dropped-file **`@label`** mentions back to their full
 * workspace-relative **`@path`** just before the composer sends. Keeps the visible
 * chip compact while the model/server sees the full relative path (which the server
 * resolves via **`ExtractAtFilePathsFromText`**).
 */

import { listAtPathSpans } from "./draftAt";
import { normalizeRelPath, type MentionEntry } from "./uniqueMentionLabel";

/**
 * Replaces each completed **`@label`** mention whose token equals a mapped drop label
 * with **`@<pathRel>`**. Uses the same **`@`**-token grammar as the backend
 * (**`listAtPathSpans`**), so tokens inside code fences / blockquotes and plain text
 * are left untouched. Non-matching **`@`** tokens are preserved verbatim.
 */
export function expandDroppedMentions(text: string, entries: MentionEntry[]): string {
  if (entries.length === 0) {
    return text;
  }
  const byLabel = new Map<string, string>();
  for (const e of entries) {
    byLabel.set(e.label, normalizeRelPath(e.pathRel));
  }
  const spans = listAtPathSpans(text);
  if (spans.length === 0) {
    return text;
  }
  let out = "";
  let last = 0;
  for (const sp of spans) {
    out += text.slice(last, sp.start);
    const rel = byLabel.get(sp.path);
    out += rel !== undefined ? `@${rel}` : text.slice(sp.start, sp.end);
    last = sp.end;
  }
  out += text.slice(last);
  return out;
}
