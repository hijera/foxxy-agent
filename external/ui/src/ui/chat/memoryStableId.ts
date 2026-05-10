/** Stable React list key for memory_copilot rows so reload from API does not remount the row and drop UI state (e.g. <details> open). */
export function stableMemoryCopilotItemId(
  memoryRowId: string,
  userTurnIndex: number,
): string {
  const row = memoryRowId.trim();
  if (row) {
    return `mc_${row.replace(/[^a-zA-Z0-9_-]/g, "_")}`;
  }
  return `mc_turn_${userTurnIndex}`;
}
