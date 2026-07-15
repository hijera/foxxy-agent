import type { TranscriptItem } from "./types";

/**
 * Cut the transcript right after its last user_message (keeping any branch_nav
 * rows attached to it). Used as the baseline when re-attaching to an in-flight
 * turn: the composer relay replays the turn's SSE bytes from the beginning, so
 * partial turn output loaded from disk would be duplicated by the replay.
 * (If the relay buffer overflowed, head-of-turn content reappears when the
 * turn ends and loadMessages reconciles from disk.)
 */
export function trimTranscriptForTurnReplay(
  items: TranscriptItem[],
): TranscriptItem[] {
  for (let i = items.length - 1; i >= 0; i--) {
    if (items[i]?.type !== "user_message") continue;
    let end = i + 1;
    while (end < items.length && items[end]?.type === "branch_nav") {
      end++;
    }
    return items.slice(0, end);
  }
  return [...items];
}
