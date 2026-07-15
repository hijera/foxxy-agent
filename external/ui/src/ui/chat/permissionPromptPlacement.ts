import type { TranscriptItem } from "./types";

/**
 * Insertion index for a permission_prompt row.
 *
 * The live stream pins the composing assistant bubble at the bottom and splices
 * tool_call rows above it, so inserting the prompt right after its tool_call
 * would strand it ABOVE the text that introduced the tool ("I'll run it now:").
 * Instead insert after the later of the matching tool_call row and the live
 * streaming assistant bubble, so the Allow card always renders below that text.
 * Falls back to appending at the end when neither anchor exists.
 */
export function permissionPromptInsertIndex(
  items: TranscriptItem[],
  toolCallId: string,
): number {
  const tcid = toolCallId.trim();
  let anchor = -1;
  for (let i = 0; i < items.length; i++) {
    const it = items[i];
    if (!it) continue;
    if (it.type === "tool_call" && it.toolCallId === tcid && i > anchor) {
      anchor = i;
    }
    if (it.type === "assistant_message" && it.streaming === true && i > anchor) {
      anchor = i;
    }
  }
  return anchor >= 0 ? anchor + 1 : items.length;
}
