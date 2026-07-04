package dev.foxxycode.intellij.diff

/**
 * One `edit_proposed` / `edit_applied` event from the foxxycode `GET /foxxycode/ide/events` SSE stream.
 * Mirrors the Go `ideEvent` struct in external/httpserver/ideevents.go.
 */
data class FoxxyCodeEditEvent(
    val type: String,        // "edit_proposed" | "edit_applied"
    val toolCallId: String,
    val sessionId: String,
    val path: String,        // absolute path
    val before: String,
    val after: String,
) {
    val isProposed: Boolean get() = type == "edit_proposed"
    val isApplied: Boolean get() = type == "edit_applied"
}
