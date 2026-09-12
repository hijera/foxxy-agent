package dev.foxxycode.intellij.clipboard

import com.intellij.openapi.Disposable
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.logger
import com.intellij.openapi.fileEditor.FileDocumentManager
import com.intellij.openapi.fileEditor.FileEditorManager
import com.intellij.openapi.ide.CopyPasteManager
import com.intellij.openapi.project.Project
import com.intellij.openapi.vfs.LocalFileSystem
import com.intellij.openapi.wm.IdeFocusManager
import dev.foxxycode.intellij.editor.SelectionPayload
import dev.foxxycode.intellij.process.FoxxyCodeProcessManager
import dev.foxxycode.intellij.settings.FoxxyCodeSettings
import java.awt.Component
import java.awt.datatransfer.DataFlavor
import java.awt.datatransfer.Transferable
import java.io.OutputStreamWriter
import java.net.HttpURLConnection
import java.net.URI
import java.nio.charset.StandardCharsets

/**
 * Reports fragments the user copies inside the IDE to the foxxycode backend
 * (POST /foxxycode/ide/copy-buffer), feeding the composer's paste-to-chip flow:
 * pasting the copied text 1:1 into the chat becomes an `@file:start-end` or
 * `@terminal` mention instead of raw text.
 *
 * Listens to [CopyPasteManager] clipboard changes (fires for every in-IDE copy).
 * The source is decided at event time: a copy whose text equals the focused
 * editor's selection is a file fragment; a copy made while an IDE terminal has
 * focus is a terminal fragment; anything else is not reported. File entries are
 * gated by [FoxxyCodeSettings.State.trackOpenFiles], terminal entries by
 * [FoxxyCodeSettings.State.trackTerminals]. Registered as a projectService.
 */
class FoxxyCodeCopyBufferService(private val project: Project) : Disposable {
    private val log = logger<FoxxyCodeCopyBufferService>()

    private var connected = false

    /** Subscribes to clipboard changes (idempotent). */
    fun startIfNeeded() {
        if (connected) return
        connected = true
        CopyPasteManager.getInstance().addContentChangedListener(
            { _, newTransferable -> onClipboardChanged(newTransferable) },
            this,
        )
    }

    override fun dispose() {
        // The listener is tied to this Disposable; nothing else to release.
    }

    /** Runs on the EDT; capture the focus owner and clipboard text synchronously. */
    private fun onClipboardChanged(contents: Transferable?) {
        if (project.isDisposed) return
        val text = stringContents(contents) ?: return
        if (!CopyBufferPayload.reportable(text)) return
        val focus = IdeFocusManager.getGlobalInstance().focusOwner

        val settings = FoxxyCodeSettings.getInstance().state
        if (settings.trackOpenFiles) {
            val body = editorCopyBody(text)
            if (body != null) {
                postAsync(body)
                return
            }
        }
        if (settings.trackTerminals && focus != null && CopyBufferPayload.looksLikeTerminalChain(classChain(focus))) {
            postAsync(CopyBufferPayload.terminalBody(text))
        }
    }

    /** A file body when the clipboard text equals the focused editor's selection. */
    private fun editorCopyBody(text: String): String? =
        ApplicationManager.getApplication().runReadAction<String?> {
            if (project.isDisposed) return@runReadAction null
            val editor = FileEditorManager.getInstance(project).selectedTextEditor
                ?: return@runReadAction null
            val sel = editor.selectionModel
            if (!CopyBufferPayload.matchesSelection(text, sel.selectedText)) return@runReadAction null
            val doc = editor.document
            val vf = FileDocumentManager.getInstance().getFile(doc) ?: return@runReadAction null
            if (vf.fileSystem !is LocalFileSystem) return@runReadAction null
            val startLine0 = doc.getLineNumber(sel.selectionStart)
            val endLine0 = doc.getLineNumber(sel.selectionEnd)
            val endAtLineStart = sel.selectionEnd == doc.getLineStartOffset(endLine0)
            val lines = SelectionPayload.lines(startLine0, endLine0, endAtLineStart)
                ?: return@runReadAction null
            CopyBufferPayload.fileBody(vf.path, lines.startLine, lines.endLine, text)
        }

    private fun stringContents(contents: Transferable?): String? = try {
        if (contents != null && contents.isDataFlavorSupported(DataFlavor.stringFlavor)) {
            contents.getTransferData(DataFlavor.stringFlavor) as? String
        } else {
            null
        }
    } catch (e: Exception) {
        log.debug("clipboard read failed: ${e.message}")
        null
    }

    private fun classChain(c: Component): List<String> =
        generateSequence<Component>(c) { it.parent }.map { it.javaClass.name }.toList()

    private fun postAsync(body: String) {
        val base = FoxxyCodeProcessManager.getInstance(project).baseUrl ?: return
        val url = (if (base.endsWith("/")) base else "$base/") + "foxxycode/ide/copy-buffer"
        ApplicationManager.getApplication().executeOnPooledThread { post(url, body) }
    }

    private fun post(url: String, body: String) {
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
            log.warn("copy-buffer POST failed: ${e.message}")
        }
    }

    companion object {
        fun getInstance(project: Project): FoxxyCodeCopyBufferService =
            project.getService(FoxxyCodeCopyBufferService::class.java)
    }
}
