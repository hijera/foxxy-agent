import type { TranscriptItem } from "../chat/types";

/** Alerts belong to the newest turn, including one woken by a background task. */
export function finishedTurnContent(rows: readonly TranscriptItem[]) {
  let start = -1;
  for (let i = rows.length - 1; i >= 0; i--) {
    if (
      rows[i]?.type === "user_message" ||
      rows[i]?.type === "background_wake"
    ) {
      start = i;
      break;
    }
  }
  const turn = rows.slice(start + 1);
  const reply = turn.filter((it) => it.type === "assistant_message").at(-1);
  const plans = turn.filter(
    (it) => it.type === "plan_document" && !it.discarded,
  );
  return { reply, plans };
}
