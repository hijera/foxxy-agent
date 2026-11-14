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

/**
 * Plain-text excerpt of an assistant reply for notification bodies: toasts and
 * balloons render literal markdown, so syntax is stripped and the cut lands on
 * a word boundary inside `max` characters.
 */
export function plainTurnExcerpt(content: string, max = 300): string {
  const kept: string[] = [];
  let inFence = false;
  for (const line of content.split(/\r?\n/)) {
    if (/^\s*(```|~~~)/.test(line)) {
      inFence = !inFence;
      continue;
    }
    if (inFence) {
      kept.push(line);
      continue;
    }
    kept.push(
      line
        .replace(/^\s{0,3}(#{1,6}|>|[-*+]|\d+[.)])\s+/, "")
        .replace(/!\[([^\]]*)\]\([^)]*\)/g, "$1")
        .replace(/\[([^\]]*)\]\([^)]*\)/g, "$1")
        .replace(/(\*\*|__|\*|_|~~|`)/g, ""),
    );
  }
  const text = kept
    .join("\n")
    .replace(/\n{3,}/g, "\n\n")
    .trim();
  if (text.length <= max) return text;
  const cut = text.slice(0, max);
  const boundary = cut.lastIndexOf(" ");
  return (boundary > max * 0.6 ? cut.slice(0, boundary) : cut).trimEnd() + "…";
}
