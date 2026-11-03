package dev.foxxycode.intellij.diff

/**
 * One `edit_proposed` / `edit_applied` / `open_file` / `reveal_file` / `tool_execute` event from
 * the foxxycode `GET /foxxycode/ide/events` SSE stream.
 * Mirrors the Go `ideEvent` struct in external/httpserver/ideevents.go.
 */
data class FoxxyCodeEditEvent(
    val type: String,        // "edit_proposed" | "edit_applied" | "open_file" | "reveal_file" | "tool_execute"
    val toolCallId: String,
    val sessionId: String,
    val path: String,        // absolute path
    val before: String,
    val after: String,
    // Set on tool_execute only: the raw tool call the client is asked to execute,
    // and the token its POST /foxxycode/ide/tool-result answer must echo.
    val tool: String = "",
    val args: String = "",
    val nonce: String = "",
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
     * The agent offers this tool call for execution through the IDE. The
     * answer travels back as POST /foxxycode/ide/tool-result (see
     * FoxxyCodeToolExecutor); until it arrives the agent writes nothing.
     */
    val isToolExecute: Boolean get() = type == "tool_execute"
}
