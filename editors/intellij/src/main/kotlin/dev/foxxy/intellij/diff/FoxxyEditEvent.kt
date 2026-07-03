package dev.foxxy.intellij.diff

/**
 * One `edit_proposed` / `edit_applied` event from the foxxy `GET /coddy/ide/events` SSE stream.
 * Mirrors the Go `ideEvent` struct in external/httpserver/ideevents.go.
 */
data class FoxxyEditEvent(
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
