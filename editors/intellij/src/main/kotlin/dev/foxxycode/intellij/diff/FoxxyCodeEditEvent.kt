package dev.foxxycode.intellij.diff

/**
 * One `edit_proposed` / `edit_applied` / `open_file` event from the foxxycode
 * `GET /foxxycode/ide/events` SSE stream.
 * Mirrors the Go `ideEvent` struct in external/httpserver/ideevents.go.
 */
data class FoxxyCodeEditEvent(
    val type: String,        // "edit_proposed" | "edit_applied" | "open_file"
    val toolCallId: String,
    val sessionId: String,
    val path: String,        // absolute path
    val before: String,
    val after: String,
) {
    val isProposed: Boolean get() = type == "edit_proposed"
    val isApplied: Boolean get() = type == "edit_applied"

    /**
     * User asked to open a file in the editor ("Show in IDE" on a plan card).
     * Unlike the edit events this is user-initiated and points at the session
     * bundle, i.e. outside the project — handlers must not apply the
     * in-project / native-diff filters to it.
     */
    val isOpenFile: Boolean get() = type == "open_file"
}
