package dev.foxxycode.intellij.clipboard

import com.google.gson.JsonObject

/**
 * Pure decision logic behind the copy-buffer reporter (unit-testable without
 * the platform): clipboard-vs-selection matching, terminal-focus classification
 * by component class names, and the POST bodies for /foxxycode/ide/copy-buffer.
 */
object CopyBufferPayload {
    /** Oversize copies are not reported: truncating breaks exact paste matching. */
    const val MAX_TEXT_BYTES = 64 * 1024

    /** CRLF-insensitive comparison form (mirrors the server's idecopy.Normalize). */
    fun normalized(s: String): String = s.replace("\r\n", "\n").trimEnd('\n')

    /** True when the clipboard text is exactly the editor's current selection. */
    fun matchesSelection(clipboardText: String, selectedText: String?): Boolean {
        if (selectedText.isNullOrBlank()) return false
        return normalized(clipboardText) == normalized(selectedText)
    }

    /** True when the copy is worth reporting at all. */
    fun reportable(text: String?): Boolean =
        !text.isNullOrBlank() && text.length <= MAX_TEXT_BYTES

    /**
     * True when a focus component's class-name chain looks like the IDE
     * terminal (JediTerm). String matching keeps the plugin free of a
     * dependency on org.jetbrains.plugins.terminal, mirroring the reflection in
     * FoxxyCodeTerminalContextService.
     */
    fun looksLikeTerminalChain(classNames: List<String>): Boolean =
        classNames.any { it.contains("JediTerm") || it.contains("terminal.", ignoreCase = true) || it.contains("Terminal") }

    fun fileBody(path: String, startLine: Int, endLine: Int, text: String): String =
        JsonObject().apply {
            addProperty("kind", "file")
            addProperty("path", path)
            addProperty("startLine", startLine)
            addProperty("endLine", endLine)
            addProperty("text", text)
        }.toString()

    fun terminalBody(text: String): String =
        JsonObject().apply {
            addProperty("kind", "terminal")
            addProperty("text", text)
        }.toString()
}
