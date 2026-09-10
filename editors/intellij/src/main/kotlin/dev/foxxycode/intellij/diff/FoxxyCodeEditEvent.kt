package dev.foxxycode.intellij.diff

/**
 * One `edit_proposed` / `edit_applied` / `open_file` / `reveal_file` event from
 * the foxxycode `GET /foxxycode/ide/events` SSE stream.
 * Mirrors the Go `ideEvent` struct in external/httpserver/ideevents.go.
 */
data class FoxxyCodeEditEvent(
    val type: String,        // "edit_proposed" | "edit_applied" | "open_file" | "reveal_file"
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

    /**
     * User exported a session transcript and the document is on disk. It is a
     * PDF / DOCX / HTML / JSON download, not source to edit, so it belongs in
     * the OS file manager rather than an editor tab.
     */
    val isRevealFile: Boolean get() = type == "reveal_file"

    /**
     * User asked to review everything the session changed (the Review button on
     * the changed-files card). Carries the session id, plus the clicked file's
     * path when the user picked one row rather than the summary; the plugin
     * reads the change set from the HTTP API itself, so this is user-initiated
     * and, like open_file, must skip the in-project / native-diff filters.
     */
    val isOpenChanges: Boolean get() = type == "open_changes"
}
