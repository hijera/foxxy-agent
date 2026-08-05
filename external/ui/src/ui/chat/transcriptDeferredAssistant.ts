import { stableAssistantItemId } from "./transcriptItemIds";
import type { TranscriptItem } from "./types";

export type DeferredAssistant = {
  content: string;
  createdAtUtc?: string;
};

export function emptyDeferredAssistant(): DeferredAssistant {
  return { content: "" };
}

export function appendDeferredAssistant(
  current: DeferredAssistant,
  content: string,
  createdAtUtc?: string,
): DeferredAssistant {
  if (!content) return current;
  const timestamp = createdAtUtc?.trim();
  return {
    content: current.content + content,
    ...(timestamp
      ? { createdAtUtc: timestamp }
      : current.createdAtUtc
        ? { createdAtUtc: current.createdAtUtc }
        : {}),
  };
}

/**
 * One assistant bubble out of the buffered text, or null when nothing accumulated.
 *
 * `indexInTurn` distinguishes several bubbles inside one turn — a turn that speaks, calls a
 * tool, then speaks again produces more than one, and they must not share a React key.
 */
export function deferredAssistantItem(
  pending: DeferredAssistant,
  userTurnIndex: number,
  indexInTurn = 0,
): Extract<TranscriptItem, { type: "assistant_message" }> | null {
  if (!pending.content) return null;
  return {
    id: stableAssistantItemId(userTurnIndex, indexInTurn),
    type: "assistant_message",
    content: pending.content,
    ...(pending.createdAtUtc ? { createdAtUtc: pending.createdAtUtc } : {}),
  };
}
