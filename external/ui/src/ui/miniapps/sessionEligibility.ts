import type { TranscriptItem } from "../chat/types";

/**
 * Header creation is available only for a completed, tool-driven session on a
 * server that actually linked the Mini Apps HTTP surface. Keep this predicate
 * pure so capability probing cannot regress into an inert action in lean
 * builds (and so the null, not-yet-probed state remains hidden).
 */
export function isMiniAppSessionEligible(args: {
  miniAppsHttpLinked: boolean | null;
  editorEmbed: boolean;
  sessionId: string;
  generating: boolean;
  items: TranscriptItem[];
}): boolean {
  if (
    args.miniAppsHttpLinked !== true ||
    args.editorEmbed ||
    !args.sessionId.trim() ||
    args.generating
  ) {
    return false;
  }
  const hasAssistant = args.items.some(
    (item) => item.type === "assistant_message" && item.content.trim() !== "",
  );
  const hasSuccessfulTool = args.items.some(
    (item) => item.type === "tool_call" && item.status === "completed",
  );
  return hasAssistant && hasSuccessfulTool;
}
