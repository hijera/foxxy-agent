package dev.foxxycode.intellij.diff

import com.google.gson.JsonObject
import com.google.gson.JsonParser
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.command.WriteCommandAction
import com.intellij.openapi.diagnostic.logger
import com.intellij.openapi.editor.Document
import com.intellij.openapi.fileEditor.FileDocumentManager
import com.intellij.openapi.project.Project
import com.intellij.openapi.vfs.LocalFileSystem
import com.intellij.openapi.vfs.VfsUtil
import dev.foxxycode.intellij.FoxxyCodeBundle
import dev.foxxycode.intellij.process.FoxxyCodeProcessManager
import dev.foxxycode.intellij.settings.FoxxyCodeSettings
import dev.foxxycode.intellij.terminal.FoxxyCodeTerminalCommandRunner
import java.io.OutputStreamWriter
import java.net.HttpURLConnection
import java.net.URI
import java.nio.charset.StandardCharsets
import java.nio.file.Path

/**
 * What a delegated filesystem write needs, given the editor's current view of the file.
 * Pure decision logic so it is unit-testable without the platform.
 */
object ToolExecPlan {
    enum class Action {
        /** Replace the document text with `after` and save (the document was clean and matched `before`). */
        APPLY,

        /** Create the file with `after` as its content (`before` is empty and the file does not exist). */
        CREATE,

        /** The editor's buffer no longer matches what the agent read - answer with an error so it re-reads. */
        CONFLICT,

        /** This client cannot execute the call - the agent runs it locally. */
        NOT_HANDLED,
    }

    data class Decision(val action: Action, val message: String = "")

    /**
     * Line endings are not content: the agent's edit transforms treat LF, CRLF
     * and CR as equivalent (internal/tools/fs/edit.go), and an IDE save can
     * rewrite a document's endings to the project's separator policy without
     * touching its text. Compare through that same equivalence.
     */
    fun normalizeEol(s: String): String = s.replace("\r\n", "\n").replace("\r", "\n")

    /**
     * @param currentText the document text, or null when the file does not exist
     * @param documentDirty whether the user has unsaved changes in that document
     */
    fun decide(
        path: String,
        before: String,
        after: String,
        currentText: String?,
        documentDirty: Boolean,
    ): Decision {
        if (path.isBlank()) return Decision(Action.NOT_HANDLED)
        if (currentText == null) {
            return if (before.isEmpty()) {
                Decision(Action.CREATE)
            } else {
                Decision(Action.CONFLICT, "file changed on disk since it was read; re-read and retry")
            }
        }
        if (documentDirty) {
            return Decision(Action.CONFLICT, "unsaved changes in the editor; save or revert them and retry")
        }
        if (normalizeEol(currentText) != normalizeEol(before)) {
            return Decision(Action.CONFLICT, "file changed in the editor since it was read; re-read and retry")
        }
        return Decision(Action.APPLY)
    }
}

/** Pure line slicing of a delegated read, matching the agent's read tool. */
object ReadSlice {
    /** Mirrors the agent's sliceLines: 1-based offset, limit lines, trailing newline kept. */
    fun apply(content: String, offset: Int, limit: Int): String {
        if (content.isEmpty()) return ""
        val lines = content.split("\n").toMutableList()
        var hasTrailingNewline = false
        if (lines.last() == "") {
            lines.removeAt(lines.size - 1)
            hasTrailingNewline = true
        }
        val start = if (offset < 1) 1 else offset
        if (start > lines.size) return ""
        var end = if (limit > 0) start + limit - 1 else lines.size
        if (end < 1 || end > lines.size) end = lines.size
        val result = lines.subList(start - 1, end).joinToString("\n")
        return if (hasTrailingNewline && end == lines.size) result + "\n" else result
    }
}

/**
 * Executes delegated tool calls (`tool_execute` events on the IDE event stream) through the
 * editor's document model: `write` / `edit` / `apply_patch` land in the user's buffers and the
 * undo stack instead of the agent writing the file behind the editor. The answer travels back
 * as `POST /foxxycode/ide/tool-result`; until it arrives the agent writes nothing, and a
 * `not_handled` answer (setting off, non-file tool, disposed project) makes it run the call
 * locally. See docs/features/ide-tool-delegation.md for the contract.
 */
class FoxxyCodeToolExecutor(private val project: Project) {
    private val log = logger<FoxxyCodeToolExecutor>()

    private companion object {
        /** The agent's delegate answer window is 60 s; a longer command falls back. */
        const val MAX_DELEGATED_COMMAND_SECONDS = 50
    }

    private data class Outcome(val status: String, val output: String? = null, val error: String? = null)

    private fun handled(output: String) = Outcome("handled", output = output)
    private fun failed(error: String) = Outcome("handled", error = error)
    private fun notHandled() = Outcome("not_handled")

    /** Runs on the SSE reader thread; the document work hops to the EDT. */
    fun execute(ev: FoxxyCodeEditEvent) {
        ApplicationManager.getApplication().invokeLater {
            if (project.isDisposed) {
                respond(ev, notHandled())
                return@invokeLater
            }
            try {
                run(ev)
            } catch (e: Throwable) {
                // Throwable, not Exception: an AssertionError from the platform
                // (DocumentImpl rejects mixed line separators) must still answer
                // the agent, or the call waits out its timeout before falling back.
                log.warn("tool_execute failed", e)
                respond(ev, failed(e.message ?: "IDE execution failed"))
            }
        }
    }

    private fun run(ev: FoxxyCodeEditEvent) {
        val settings = FoxxyCodeSettings.getInstance().state
        if (!settings.ideToolExecution) {
            respond(ev, notHandled())
            return
        }
        // Per-tool switches: an off tool (or one this client does not own) runs
        // in the agent itself. plan_write falls through to the document path -
        // its event carries the path/before/after preview like a plain write.
        when (ev.tool) {
            "run_command" -> {
                if (settings.ideExecTerminal) runInTerminal(ev) else respond(ev, notHandled())
                return
            }
            "read" -> {
                if (settings.ideExecRead) readFromDocument(ev) else respond(ev, notHandled())
                return
            }
            "mkdir" -> {
                if (settings.ideExecMkdir) vfsOp(ev) else respond(ev, notHandled())
                return
            }
            "rm" -> {
                if (settings.ideExecRm) vfsOp(ev) else respond(ev, notHandled())
                return
            }
            "mv" -> {
                if (settings.ideExecMv) vfsOp(ev) else respond(ev, notHandled())
                return
            }
            "touch" -> {
                if (settings.ideExecTouch) vfsOp(ev) else respond(ev, notHandled())
                return
            }
            "plan_write" -> {
                if (!settings.ideExecPlanWrite) {
                    respond(ev, notHandled())
                    return
                }
            }
        }
        val path = ev.path.trim()
        // No file preview in the event: a non-fs tool, or an fs call whose
        // transform failed before the broadcast. This client executes nothing
        // here, so the agent must run the call itself.
        if (path.isBlank()) {
            respond(ev, notHandled())
            return
        }
        val vf = try {
            LocalFileSystem.getInstance().refreshAndFindFileByNioFile(Path.of(path))
        } catch (e: Exception) {
            log.debug("tool_execute rejected path $path: ${e.message}")
            null
        }
        if (vf == null || vf.isDirectory) {
            val d = ToolExecPlan.decide(path, ev.before, ev.after, null, false)
            when (d.action) {
                ToolExecPlan.Action.CREATE -> createFile(ev)
                else -> respond(ev, failed(d.message))
            }
            return
        }
        val doc = FileDocumentManager.getInstance().getDocument(vf)
        if (doc == null) {
            log.warn("tool_execute: no document for $path (vf=${vf.path}, valid=${vf.isValid}, fileType=${vf.fileType.name})")
            respond(ev, failed("no text document for the file path=$path vf=${vf.path} valid=${vf.isValid} type=${vf.fileType.name}"))
            return
        }
        val dirty = FileDocumentManager.getInstance().isDocumentUnsaved(doc)
        val d = ToolExecPlan.decide(
            path, ev.before, ev.after,
            doc.immutableCharSequence.toString(),
            dirty,
        )
        when (d.action) {
            ToolExecPlan.Action.APPLY -> apply(ev, doc)
            ToolExecPlan.Action.CONFLICT -> respond(ev, failed(d.message))
            else -> respond(ev, notHandled())
        }
    }

    /**
     * Foreground `run_command` runs in an IDE terminal (see
     * [dev.foxxycode.intellij.terminal.FoxxyCodeTerminalCommandRunner]); anything the
     * terminal cannot own - a background command, or one whose timeout outruns this
     * client's answer window - falls back to the agent's own runner.
     */
    private fun runInTerminal(ev: FoxxyCodeEditEvent) {
        val args = try {
            JsonParser.parseString(ev.args).asJsonObject
        } catch (e: Exception) {
            respond(ev, notHandled())
            return
        }
        val command = args.get("command")?.takeIf { it.isJsonPrimitive }?.asString.orEmpty()
        val background = args.get("background")?.takeIf { it.isJsonPrimitive }?.asBoolean ?: false
        val timeoutSeconds = args.get("timeout_seconds")?.takeIf { it.isJsonPrimitive }?.asInt ?: 0
        if (command.isBlank() || background || timeoutSeconds > MAX_DELEGATED_COMMAND_SECONDS) {
            respond(ev, notHandled())
            return
        }
        val waitMs = (if (timeoutSeconds > 0) timeoutSeconds else MAX_DELEGATED_COMMAND_SECONDS) * 1000L
        ApplicationManager.getApplication().executeOnPooledThread {
            val r = try {
                FoxxyCodeTerminalCommandRunner.run(project, command, project.basePath, waitMs)
            } catch (t: Throwable) {
                log.warn("run_command terminal failed", t)
                FoxxyCodeTerminalCommandRunner.Result(handled = false)
            }
            when {
                !r.handled -> respond(ev, notHandled())
                r.error.isNotEmpty() -> respond(ev, failed(r.error))
                else -> respond(ev, handled(r.output))
            }
        }
    }

    /**
     * `read` from the editor buffer: the document may hold unsaved changes the disk does
     * not. Directories, pictures and files no editor document backs are left to the
     * agent's own reader, which knows how to list and sniff them.
     */
    private fun readFromDocument(ev: FoxxyCodeEditEvent) {
        val args = parseArgs(ev) ?: return respond(ev, notHandled())
        val path = args.get("path")?.takeIf { it.isJsonPrimitive }?.asString.orEmpty()
        if (path.isBlank()) {
            respond(ev, notHandled())
            return
        }
        // The event carries no file preview for read, so the path is the tool's
        // own - relative ones resolve against the session workspace, which the
        // backend was started with as its cwd.
        val vf = try {
            LocalFileSystem.getInstance().refreshAndFindFileByNioFile(resolveInProject(path))
        } catch (e: Exception) {
            null
        }
        val doc = vf?.takeIf { it.isValid && !it.isDirectory }?.let { FileDocumentManager.getInstance().getDocument(it) }
        if (doc == null) {
            respond(ev, notHandled())
            return
        }
        val offset = args.get("offset")?.takeIf { it.isJsonPrimitive }?.asInt ?: 0
        val limit = args.get("limit")?.takeIf { it.isJsonPrimitive }?.asInt ?: 0
        respond(ev, handled(ReadSlice.apply(doc.immutableCharSequence.toString(), offset, limit)))
    }

    /**
     * mkdir / rm / mv / touch through the IDE's own filesystem: the project tree updates
     * at once and Local History keeps what the agent changed. The result strings mirror
     * the agent's tools.
     */
    private fun vfsOp(ev: FoxxyCodeEditEvent) {
        val args = parseArgs(ev) ?: return respond(ev, notHandled())
        try {
            var outcome: String? = null
            var failure: String? = null
            WriteCommandAction.runWriteCommandAction(project, FoxxyCodeBundle.message("toolexec.command.vfs"), null, {
                try {
                    outcome = vfsOpLocked(ev, args)
                } catch (t: Throwable) {
                    failure = t.message ?: "filesystem operation failed"
                }
            })
            failure?.let { respond(ev, failed(it)) } ?: respond(ev, handled(outcome ?: ""))
        } catch (t: Throwable) {
            log.warn("vfs op failed", t)
            respond(ev, failed(t.message ?: "filesystem operation failed"))
        }
    }

    private fun vfsOpLocked(ev: FoxxyCodeEditEvent, args: com.google.gson.JsonObject): String {
        val lfs = LocalFileSystem.getInstance()
        fun str(key: String) = args.get(key)?.takeIf { it.isJsonPrimitive }?.asString.orEmpty()
        fun bool(key: String) = args.get(key)?.takeIf { it.isJsonPrimitive }?.asBoolean ?: false
        fun resolve(p: String) = resolveInProject(p)
        return when (ev.tool) {
            "mkdir" -> {
                val target = resolve(str("path"))
                val created = VfsUtil.createDirectoryIfMissing(target.toString())
                    ?: throw IllegalStateException("mkdir: cannot create $target")
                "created directory $target"
            }
            "touch" -> {
                val target = resolve(str("path"))
                val vf = lfs.refreshAndFindFileByNioFile(target)
                if (vf != null && !vf.isDirectory) {
                    vf.setBinaryContent(vf.contentsToByteArray())
                    "updated timestamps for $target"
                } else {
                    if (vf == null && bool("create_parents")) {
                        VfsUtil.createDirectoryIfMissing(target.parent.toString())
                            ?: throw IllegalStateException("touch: cannot create ${target.parent}")
                    }
                    val parent = lfs.refreshAndFindFileByNioFile(target.parent)
                        ?: throw IllegalStateException("touch: parent directory not found")
                    val file = if (vf != null) vf else parent.createChildData(this, target.fileName.toString())
                    if (vf == null) file.setBinaryContent(ByteArray(0))
                    "created file $target"
                }
            }
            "rm" -> {
                val target = resolve(str("path"))
                val vf = lfs.refreshAndFindFileByNioFile(target)
                    ?: throw IllegalStateException("rm: $target not found")
                val recursive = bool("recursive")
                if (vf.isDirectory && !recursive) {
                    throw IllegalStateException("rm: $target is a directory; use recursive")
                }
                val tree = vf.isDirectory && recursive
                deleteRecursively(vf)
                if (tree) "removed tree $target" else "removed $target"
            }
            "mv" -> {
                val src = resolve(str("src"))
                val dst = resolve(str("dst"))
                val vf = lfs.refreshAndFindFileByNioFile(src)
                    ?: throw IllegalStateException("mv: $src not found")
                VfsUtil.createDirectoryIfMissing(dst.parent.toString())
                    ?: throw IllegalStateException("mv: cannot create ${dst.parent}")
                val parent = lfs.refreshAndFindFileByNioFile(dst.parent)
                    ?: throw IllegalStateException("mv: parent directory not found")
                val newName = dst.fileName.toString()
                if (vf.parent != parent) vf.move(this, parent)
                if (vf.name != newName) vf.rename(this, newName)
                "moved $src -> $dst"
            }
            else -> throw IllegalStateException("not a filesystem tool: ${ev.tool}")
        }
    }

    private fun parseArgs(ev: FoxxyCodeEditEvent): com.google.gson.JsonObject? {
        return try {
            JsonParser.parseString(ev.args).asJsonObject
        } catch (e: Exception) {
            null
        }
    }

    /** A tool's own path is relative to the session workspace (the backend's cwd). */
    private fun resolveInProject(p: String): Path =
        Path.of(project.basePath ?: ".").resolve(p).normalize()

    /** VfsUtil.deleteRecursively is not on the 2022.2 floor; plain VirtualFile recursion is. */
    private fun deleteRecursively(vf: com.intellij.openapi.vfs.VirtualFile) {
        if (vf.isDirectory) {
            for (child in vf.children) deleteRecursively(child)
        }
        vf.delete(this)
    }

    private fun apply(ev: FoxxyCodeEditEvent, doc: Document) {
        try {
            WriteCommandAction.runWriteCommandAction(project, FoxxyCodeBundle.message("toolexec.command.apply"), null, {
                // The document stores \n internally and keeps the file's own line
                // separators aside (re-applied on save), so the incoming text goes
                // in normalized: DocumentImpl.setText rejects mixed separators.
                doc.setText(ToolExecPlan.normalizeEol(ev.after))
                // The document matched `before` and was clean, so saving brings the disk in
                // line with what the agent expects its next read to see.
                FileDocumentManager.getInstance().saveDocument(doc)
            })
            respond(ev, handled("applied through the IDE document"))
        } catch (e: Throwable) {
            log.warn("apply failed", e)
            respond(ev, failed(e.message ?: "apply failed"))
        }
    }

    private fun createFile(ev: FoxxyCodeEditEvent) {
        try {
            WriteCommandAction.runWriteCommandAction(project, FoxxyCodeBundle.message("toolexec.command.create"), null, {
                val p = Path.of(ev.path)
                val parent = VfsUtil.createDirectoryIfMissing(p.parent.toString())
                    ?: throw IllegalStateException("cannot create the parent directory")
                val file = parent.createChildData(this, p.fileName.toString())
                file.setBinaryContent(ev.after.toByteArray(StandardCharsets.UTF_8))
            })
            respond(ev, handled("created " + ev.path))
        } catch (e: Throwable) {
            log.warn("create failed", e)
            respond(ev, failed(e.message ?: "create failed"))
        }
    }

    private fun respond(ev: FoxxyCodeEditEvent, outcome: Outcome) {
        if (ev.nonce.isBlank()) return
        val base = FoxxyCodeProcessManager.getInstance(project).baseUrl ?: return
        val url = (if (base.endsWith("/")) base else "$base/") + "foxxycode/ide/tool-result"
        val body = JsonObject().apply {
            addProperty("toolCallId", ev.toolCallId)
            addProperty("nonce", ev.nonce)
            addProperty("status", outcome.status)
            outcome.output?.let { addProperty("output", it) }
            outcome.error?.let { addProperty("error", it) }
        }.toString()
        // Fire-and-forget on a background thread; the response is a 204.
        ApplicationManager.getApplication().executeOnPooledThread {
            try {
                val conn = URI.create(url).toURL().openConnection() as HttpURLConnection
                conn.requestMethod = "POST"
                conn.connectTimeout = 3000
                conn.readTimeout = 5000
                conn.doOutput = true
                conn.setRequestProperty("Content-Type", "application/json")
                OutputStreamWriter(conn.outputStream, StandardCharsets.UTF_8).use { it.write(body) }
                conn.responseCode // trigger the request
                conn.disconnect()
            } catch (e: Exception) {
                log.warn("tool-result POST failed: ${e.message}")
            }
        }
    }
}
