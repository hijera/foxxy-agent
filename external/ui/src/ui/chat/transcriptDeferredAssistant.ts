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

export function deferredAssistantItem(
  pending: DeferredAssistant,
  userTurnIndex: number,
): Extract<TranscriptItem, { type: "assistant_message" }> | null {
  if (!pending.content) return null;
  return {
    id: stableAssistantItemId(userTurnIndex),
    type: "assistant_message",
    content: pending.content,
    ...(pending.createdAtUtc ? { createdAtUtc: pending.createdAtUtc } : {}),
  };
}
