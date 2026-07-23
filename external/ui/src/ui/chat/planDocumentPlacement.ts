import type { TranscriptItem } from "./types";

/**
 * Plan cards always render as the LAST rows of the turn they belong to.
 *
 * The server appends the plan_document message the moment plan_write runs (mid-turn,
 * right after the tool call), while the transcript rebuild defers all assistant text of
 * a turn into a single bubble flushed at the turn boundary. Without this normalization
 * the card lands ABOVE the prose that introduces it, so "Run plan" ends up in the middle
 * of the answer instead of at the end of the turn.
 *
 * A turn is the run of rows from a user_message up to (excluding) the next one; rows
 * before the first user_message form their own leading segment. Relative order of several
 * plan cards inside one turn is preserved. Returns the input array unchanged (same
 * reference) when nothing has to move — this runs on every streamed token and callers
 * compare transcripts by index.
 */
export function pinPlanDocumentsToTurnEnd(
  items: TranscriptItem[],
): TranscriptItem[] {
  if (!needsReorder(items)) {
    return items;
  }
  const out: TranscriptItem[] = [];
  let pending: TranscriptItem[] = [];
  for (const it of items) {
    if (it.type === "user_message" && pending.length > 0) {
      out.push(...pending);
      pending = [];
    }
    if (it.type === "plan_document") {
      pending.push(it);
      continue;
    }
    out.push(it);
  }
  out.push(...pending);
  return out;
}

/** True when at least one plan_document row is followed by a non-plan row in the same turn. */
function needsReorder(items: TranscriptItem[]): boolean {
  let sawPlanInTurn = false;
  for (const it of items) {
    if (it.type === "user_message") {
      sawPlanInTurn = false;
      continue;
    }
    if (it.type === "plan_document") {
      sawPlanInTurn = true;
      continue;
    }
    if (sawPlanInTurn) {
      return true;
    }
  }
  return false;
}
