import { transcriptItemsLooselyEqual } from "./transcriptServerSnapshot";
import type { TranscriptItem } from "./types";

/** Stable id for a tool_call row. */
export function stableToolCallItemId(toolCallId: string): string {
  return `tc_${toolCallId.trim()}`;
}

export function stablePermissionPromptItemId(toolCallId: string): string {
  return `pp_${toolCallId.trim()}`;
}

export function stableQuestionPromptItemId(requestId: string): string {
  return `qp_${requestId.trim()}`;
}

export function stableThinkingItemId(
  userTurnIndex: number,
  indexInTurn: number,
): string {
  return `th_${userTurnIndex}_${indexInTurn}`;
}

export function stableAssistantItemId(
  userTurnIndex: number,
  indexInTurn = 0,
): string {
  return indexInTurn === 0
    ? `as_${userTurnIndex}`
    : `as_${userTurnIndex}_${indexInTurn}`;
}

export function stableUserItemId(userTurnIndex: number): string {
  return `u_${userTurnIndex}`;
}

/**
 * After rebuilding transcript from the server, reuse React keys (and plan expanded)
 * from the previous in-memory list when rows describe the same step. A tool call
 * also keeps the todo plan snapshot it already showed when the server row carries
 * none: the snapshot reaches the client once over SSE, and a reload that finds it
 * missing on disk (write race, branch, failed enrichment) must not blank the card.
 */
export function preserveTranscriptItemIds(
  merged: TranscriptItem[],
  previous: TranscriptItem[] | undefined,
): TranscriptItem[] {
  if (!previous?.length) {
    return merged;
  }
  let pi = 0;
  const out: TranscriptItem[] = [];
  for (const row of merged) {
    let matched = false;
    for (let j = pi; j < previous.length; j++) {
      const prev = previous[j];
      if (!prev || !transcriptItemsLooselyEqual(row, prev)) {
        continue;
      }
      pi = j + 1;
      matched = true;
      if (row.type === "plan_document" && prev.type === "plan_document") {
        out.push({ ...row, id: prev.id, expanded: prev.expanded });
      } else if (
        row.type === "tool_call" &&
        prev.type === "tool_call" &&
        !(row.todoPlan && row.todoPlan.length > 0) &&
        prev.todoPlan &&
        prev.todoPlan.length > 0
      ) {
        out.push({ ...row, id: prev.id, todoPlan: prev.todoPlan });
      } else {
        out.push({ ...row, id: prev.id });
      }
      break;
    }
    if (!matched) {
      out.push(row);
    }
  }
  return out;
}
