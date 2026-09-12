package dev.foxxycode.intellij.autocomplete

/**
 * Pure text rules shared by the document listener and the renderers. Kept free of IntelliJ types so
 * they can be unit-tested without an IDE fixture, which is the only kind of test this plugin runs.
 */
object SuggestionText {

    /**
     * Re-uses an already displayed suggestion for the text the user has just typed.
     *
     * This is what makes an LLM-backed completion feel usable at all: typing the characters the
     * suggestion already predicted must not cost a round trip. When [typed] is a prefix of
     * [shown], the remainder is what is still ahead of the caret; otherwise the suggestion is
     * stale and the caller must ask the backend again.
     *
     * Returns null when the suggestion no longer applies, and an empty string when the user has
     * typed it out exactly (nothing left to draw).
     */
    fun advance(shown: String, typed: String): String? {
        if (typed.isEmpty()) return shown
        if (!shown.startsWith(typed)) return null
        return shown.substring(typed.length)
    }

    /**
     * Splits a suggestion into the part drawn on the caret's own line and the lines drawn beneath
     * it. IntelliJ needs those as two different inlays (inline and block), so the split happens
     * once here.
     */
    fun split(suggestion: String): Pair<String, List<String>> {
        val normalized = suggestion.replace("\r\n", "\n")
        val idx = normalized.indexOf('\n')
        if (idx < 0) return normalized to emptyList()
        return normalized.substring(0, idx) to normalized.substring(idx + 1).split("\n")
    }

    /**
     * True when a document change should never trigger a suggestion: the completion is only useful
     * for ordinary forward typing, and asking after every programmatic edit (reformat, paste of a
     * whole file, refactoring) burns tokens for a suggestion nobody is waiting for.
     */
    fun isTypingChange(newFragmentLength: Int, oldFragmentLength: Int): Boolean =
        newFragmentLength in 1..MAX_TYPED_CHARS && oldFragmentLength == 0

    /**
     * Whether a keystroke is worth a request at all. The prefix cache still runs for every
     * keystroke; this only gates the network call, so a rejected keystroke costs nothing.
     *
     * A caret followed by an identifier character sits inside a word the user is still typing -
     * any suggestion there guesses the rest of a name. A closing bracket or statement end is a
     * point where nothing is expected next. Whitespace typed on an otherwise blank line is
     * indentation, not content yet (a space after `return`, by contrast, is a prime spot). A line
     * break is deliberately allowed: Enter after `{` is exactly where a block suggestion belongs.
     *
     * [lineBefore] is the caret's line up to the caret, [typed] the text the keystroke inserted,
     * [charAfter] the character right after the caret or null at the end of the document.
     */
    fun shouldRequest(lineBefore: String, typed: String, charAfter: Char?): Boolean {
        if (charAfter != null && (charAfter.isLetterOrDigit() || charAfter == '_')) return false
        if (typed.isNotEmpty() && typed.last() in CLOSERS) return false
        val whitespaceOnly = typed.isNotEmpty() && typed.all { it == ' ' || it == '\t' }
        if (whitespaceOnly && lineBefore.isBlank()) return false
        return true
    }

    /**
     * How long to pause automatic requests after the backend answered 429: the provider's
     * Retry-After in seconds when it is a sane number, [DEFAULT_PAUSE_SECONDS] otherwise, never
     * more than [MAX_PAUSE_SECONDS] so a bad header cannot switch the feature off for good.
     */
    fun retryAfterSeconds(header: String?): Long {
        val parsed = header?.trim()?.toLongOrNull() ?: return DEFAULT_PAUSE_SECONDS
        if (parsed <= 0) return DEFAULT_PAUSE_SECONDS
        return minOf(parsed, MAX_PAUSE_SECONDS)
    }

    /**
     * A paste is still a single change event, so cap what counts as typing. Completion-triggering
     * input is a keystroke or an IDE-inserted pair such as "()".
     */
    const val MAX_TYPED_CHARS = 8

    const val DEFAULT_PAUSE_SECONDS = 10L
    const val MAX_PAUSE_SECONDS = 60L

    private const val CLOSERS = ")]};,"
}
