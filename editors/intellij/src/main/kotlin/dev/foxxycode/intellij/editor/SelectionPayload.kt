package dev.foxxycode.intellij.editor

import com.google.gson.JsonObject

/**
 * Pure line/cap math behind the editor-state selection payload — platform-free
 * so it runs in the plain JUnit4 `test` task (mirrors [dev.foxxycode.intellij.ui.ProjectRelativePaths]).
 */
object SelectionPayload {
    /** Cap on the reported selection text; the tail is kept (the server re-caps too). */
    const val MAX_SELECTION_BYTES = 16 * 1024

    data class Lines(val startLine: Int, val endLine: Int)

    /**
     * Converts 0-based selection lines to a 1-based inclusive range. A selection
     * whose end sits at column 0 of a later line does not include that line (the
     * usual shift-down-to-line-start gesture). Null when nothing remains.
     */
    fun lines(startLine0: Int, endLine0: Int, endAtLineStart: Boolean): Lines? {
        var end0 = endLine0
        if (endAtLineStart && end0 > startLine0) end0 -= 1
        if (end0 < startLine0) return null
        return Lines(startLine0 + 1, end0 + 1)
    }

    fun capTail(text: String, maxBytes: Int = MAX_SELECTION_BYTES): String =
        if (text.length <= maxBytes) text else text.substring(text.length - maxBytes)

    /** The JSON `selection` object for editor-state, or null when it should be omitted. */
    fun build(
        file: String,
        startLine0: Int,
        endLine0: Int,
        endAtLineStart: Boolean,
        text: String,
    ): JsonObject? {
        if (file.isBlank() || text.isBlank()) return null
        val ln = lines(startLine0, endLine0, endAtLineStart) ?: return null
        return JsonObject().apply {
            addProperty("file", file)
            addProperty("startLine", ln.startLine)
            addProperty("endLine", ln.endLine)
            addProperty("text", capTail(text))
        }
    }
}
