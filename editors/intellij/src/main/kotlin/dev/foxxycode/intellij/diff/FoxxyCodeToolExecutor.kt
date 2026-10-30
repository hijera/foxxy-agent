package dev.foxxycode.intellij.diff

import com.google.gson.JsonObject
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
        if (currentText != before) {
            return Decision(Action.CONFLICT, "file changed in the editor since it was read; re-read and retry")
        }
        return Decision(Action.APPLY)
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
            } catch (e: Exception) {
                log.warn("tool_execute failed", e)
                respond(ev, failed(e.message ?: "IDE execution failed"))
            }
        }
    }

    private fun run(ev: FoxxyCodeEditEvent) {
        if (!FoxxyCodeSettings.getInstance().state.ideToolExecution) {
            respond(ev, notHandled())
            return
        }
        val path = ev.path.trim()
        val vf = try {
            LocalFileSystem.getInstance().refreshAndFindFileByNioFile(Path.of(path))
        } catch (e: Exception) {
            log.debug("tool_execute rejected path $path: ${e.message}")
            null
        }
        if (vf == null) {
            val d = ToolExecPlan.decide(path, ev.before, ev.after, null, false)
            when (d.action) {
                ToolExecPlan.Action.CREATE -> createFile(ev)
                else -> respond(ev, failed(d.message))
            }
            return
        }
        val doc = FileDocumentManager.getInstance().getDocument(vf)
        if (doc == null) {
            respond(ev, failed("no text document for the file"))
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

    private fun apply(ev: FoxxyCodeEditEvent, doc: Document) {
        try {
            WriteCommandAction.runWriteCommandAction(project, FoxxyCodeBundle.message("toolexec.command.apply"), null, {
                doc.setText(ev.after)
                // The document matched `before` and was clean, so saving brings the disk in
                // line with what the agent expects its next read to see.
                FileDocumentManager.getInstance().saveDocument(doc)
            })
            respond(ev, handled("applied through the IDE document"))
        } catch (e: Exception) {
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
        } catch (e: Exception) {
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
