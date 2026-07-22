// Preambles that the backend prepends to a compaction summary message so the
// LLM knows earlier history was replaced. The SPA strips them for display since
// the CompactionMessage foldout already labels the row.
const COMPACTION_PREAMBLES = [
  // coddy engine (internal/session/compaction.go)
  "The earlier conversation was compacted. Summary of the compacted part:\n\n",
  // opencode engine (internal/agent/compaction.go)
  "Summary of the earlier conversation (older turns were compacted to save context):\n\n",
];

/** Removes a known compaction preamble prefix; returns the trimmed summary body. */
export function stripCompactionPreamble(content: string): string {
  const text = content ?? "";
  for (const p of COMPACTION_PREAMBLES) {
    if (text.startsWith(p)) {
      return text.slice(p.length).trim();
    }
  }
  return text.trim();
}
