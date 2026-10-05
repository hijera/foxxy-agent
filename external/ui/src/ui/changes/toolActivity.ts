import type { TranscriptItem } from "../chat/types";

/**
 * Tool calls in the transcript that have finished, however they ended. Every
 * one can have touched the workspace - a shell command as much as an edit - so
 * the changed-files card, open mid-turn, re-reads each time this moves.
 */
export function finishedToolCalls(items: readonly TranscriptItem[]): number {
  let n = 0;
  for (const it of items) {
    if (
      it.type === "tool_call" &&
      (it.status === "completed" || it.status === "failed" || it.status === "cancelled")
    ) {
      n += 1;
    }
  }
  return n;
}
