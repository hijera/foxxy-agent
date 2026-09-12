/** The suggestion currently offered for one document position. */
export interface CachedSuggestion {
  uri: string;
  offset: number;
  text: string;
}

/**
 * Re-uses an already offered suggestion for the text the user has since typed.
 *
 * This is what makes an LLM-backed completion feel usable at all: typing the characters the
 * suggestion already predicted must not cost a round trip, because the round trip is hundreds of
 * milliseconds. When the text now sitting between the cached offset and the caret is exactly the
 * head of the cached suggestion, the tail is still valid and can be drawn immediately.
 *
 * Returns null when the cache does not apply, and an empty string when the user has typed the
 * suggestion out in full (nothing left to offer).
 *
 * Mirrors `SuggestionText.advance` in the IntelliJ plugin, but derives the typed text from the
 * document rather than from a change event, because that is what VS Code hands the provider.
 */
export function advanceCached(
  cached: CachedSuggestion | null,
  uri: string,
  offset: number,
  documentText: string,
): string | null {
  if (!cached || cached.uri !== uri) return null;
  if (offset < cached.offset) return null;
  const typedLength = offset - cached.offset;
  if (typedLength > cached.text.length) return null;
  const typed = documentText.slice(cached.offset, offset);
  if (!cached.text.startsWith(typed)) return null;
  return cached.text.slice(typedLength);
}
