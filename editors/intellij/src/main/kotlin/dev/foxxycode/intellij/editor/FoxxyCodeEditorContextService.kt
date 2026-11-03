package dev.foxxycode.intellij.editor

import com.google.gson.JsonArray
import com.google.gson.JsonObject
import com.intellij.openapi.Disposable
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.logger
import com.intellij.openapi.editor.EditorFactory
import com.intellij.openapi.editor.event.SelectionEvent
import com.intellij.openapi.editor.event.SelectionListener
import com.intellij.openapi.fileEditor.FileDocumentManager
import com.intellij.openapi.fileEditor.FileEditorManager
import com.intellij.openapi.fileEditor.FileEditorManagerEvent
import com.intellij.openapi.fileEditor.FileEditorManagerListener
import com.intellij.openapi.project.Project
import com.intellij.openapi.vfs.LocalFileSystem
import com.intellij.openapi.vfs.VirtualFile
import com.intellij.util.Alarm
import dev.foxxycode.intellij.process.FoxxyCodeProcessManager
import dev.foxxycode.intellij.settings.FoxxyCodeSettings
import java.io.OutputStreamWriter
import java.net.HttpURLConnection
import java.net.URI
import java.nio.charset.StandardCharsets

/**
 * Reports the set of open editor tabs and the focused file to the foxxycode backend
 * (POST /foxxycode/ide/editor-state) whenever the editor selection changes. The backend
 * injects the latest snapshot into each agent turn so the model knows which files the user
 * is actively viewing — the same "open tabs" context other coding agents provide.
 *
 * Mirrors the lifecycle of [dev.foxxycode.intellij.diff.FoxxyCodeIdeDiffService]:
 * [startIfNeeded] wires the message-bus subscription once the server base URL is known.
 * Gated by [FoxxyCodeSettings.State.trackOpenFiles] (default true). Registered as a projectService.
 */
class FoxxyCodeEditorContextService(private val project: Project) : Disposable {
    private val log = logger<FoxxyCodeEditorContextService>()
    // The snapshot must be taken on the EDT: FileEditorManager.getSelectedTextEditor asserts
    // the dispatch thread (a read action on a pooled thread is not enough), so the debounce
    // lands on the Swing thread and only the HTTP POST moves to the pool.
    private val alarm = Alarm(Alarm.ThreadToUse.SWING_THREAD, this)

    private var connected = false
    private var lastPayload: String? = null

    /** Subscribes to editor changes (idempotent) and pushes an initial snapshot. */
    fun startIfNeeded() {
        if (!connected) {
            connected = true
            val conn = project.messageBus.connect(this)
            conn.subscribe(
                FileEditorManagerListener.FILE_EDITOR_MANAGER,
                object : FileEditorManagerListener {
                    override fun fileOpened(source: FileEditorManager, file: VirtualFile) = schedule()
                    override fun fileClosed(source: FileEditorManager, file: VirtualFile) = schedule()
                    override fun selectionChanged(event: FileEditorManagerEvent) = schedule()
                },
            )
            // Text-selection tracking: the debounce plus the lastPayload dedupe keep
            // the POST rate bounded even though selection events fire constantly.
            EditorFactory.getInstance().eventMulticaster.addSelectionListener(
                object : SelectionListener {
                    override fun selectionChanged(e: SelectionEvent) {
                        if (e.editor.project === project) schedule()
                    }
                },
                this,
            )
        }
        schedule()
    }

    override fun dispose() {
        alarm.cancelAllRequests()
    }

    private fun schedule() {
        if (project.isDisposed) return
        alarm.cancelAllRequests()
        alarm.addRequest({ report() }, DEBOUNCE_MS)
    }

    /**
     * Pushes a snapshot that also carries the open buffers' document text - the
     * pre-send enrichment the SPA asks for through the browser panel's
     * `window.foxxycodeBeforeSend` bridge. Runs off the EDT (a JCEF query
     * thread), hops to the Swing thread for the editor reads, and POSTs
     * synchronously so the caller can send its message on a fresh snapshot.
     */
    fun pushWithBuffers() {
        if (project.isDisposed) return
        val base = FoxxyCodeProcessManager.getInstance(project).baseUrl ?: return
        var payload: String? = null
        ApplicationManager.getApplication().invokeAndWait {
            if (project.isDisposed) return@invokeAndWait
            val fem = FileEditorManager.getInstance(project)
            val open = fem.openFiles.filter { it.fileSystem is LocalFileSystem }.map { it.path }
            val active = fem.selectedFiles.firstOrNull { it.fileSystem is LocalFileSystem }?.path ?: ""
            val ordered = LinkedHashSet<String>()
            if (active.isNotBlank()) ordered.add(active)
            ordered.addAll(open)
            payload = JsonObject().apply {
                add("openFiles", JsonArray().apply { ordered.forEach { add(it) } })
                addProperty("activeFile", active)
                currentSelection(fem)?.let { add("selection", it) }
                add("buffers", openBuffers(fem))
            }.toString()
        }
        val body = payload ?: return
        val url = (if (base.endsWith("/")) base else "$base/") + "foxxycode/ide/editor-state"
        post(url, body)
    }

    /**
     * The document text of every open local buffer, the active one first; capped
     * like the backend (8 KiB per buffer, 8 buffers) so the POST stays small.
     */
    private fun openBuffers(fem: FileEditorManager): JsonArray {
        val arr = JsonArray()
        var total = 0
        val ordered = LinkedHashSet<com.intellij.openapi.vfs.VirtualFile>()
        fem.selectedFiles.firstOrNull { it.fileSystem is LocalFileSystem }?.let { ordered.add(it) }
        fem.openFiles.filterTo(ordered) { it.fileSystem is LocalFileSystem }
        for (vf in ordered) {
            if (arr.size() >= 8 || total > 32 * 1024) break
            val fdm = FileDocumentManager.getInstance()
            val doc = fdm.getDocument(vf) ?: continue
            var text = doc.immutableCharSequence.toString()
            if (text.length > 8 * 1024) text = text.takeLast(8 * 1024)
            total += text.length
            arr.add(
                JsonObject().apply {
                    addProperty("path", vf.path)
                    addProperty("text", text)
                    addProperty("dirty", fdm.isDocumentUnsaved(doc))
                },
            )
        }
        return arr
    }

    /** Runs on the EDT (see [alarm]); snapshots the editor state and posts it off-thread. */
    private fun report() {
        if (project.isDisposed) return
        if (!FoxxyCodeSettings.getInstance().state.trackOpenFiles) return
        val base = FoxxyCodeProcessManager.getInstance(project).baseUrl ?: return

        val fem = FileEditorManager.getInstance(project)
        val open = fem.openFiles.filter { it.fileSystem is LocalFileSystem }.map { it.path }
        val active = fem.selectedFiles.firstOrNull { it.fileSystem is LocalFileSystem }?.path ?: ""
        val snapshot = Snapshot(open, active, currentSelection(fem))

        // De-duplicate, keeping the active file first (mirrors the VSCode reporter).
        val ordered = LinkedHashSet<String>()
        if (snapshot.activeFile.isNotBlank()) ordered.add(snapshot.activeFile)
        ordered.addAll(snapshot.openFiles)

        val body = JsonObject().apply {
            add("openFiles", JsonArray().apply { ordered.forEach { add(it) } })
            addProperty("activeFile", snapshot.activeFile)
            snapshot.selection?.let { add("selection", it) }
        }.toString()
        if (body == lastPayload) return
        lastPayload = body

        val url = (if (base.endsWith("/")) base else "$base/") + "foxxycode/ide/editor-state"
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
            log.warn("editor-state POST failed: ${e.message}")
        }
    }

    /** The current text selection in the focused editor (EDT only: the getter asserts it). */
    private fun currentSelection(fem: FileEditorManager): JsonObject? {
        val editor = fem.selectedTextEditor ?: return null
        val sel = editor.selectionModel
        val text = sel.selectedText ?: return null
        if (text.isBlank()) return null
        val doc = editor.document
        val vf = FileDocumentManager.getInstance().getFile(doc) ?: return null
        if (vf.fileSystem !is LocalFileSystem) return null
        val startLine0 = doc.getLineNumber(sel.selectionStart)
        val endLine0 = doc.getLineNumber(sel.selectionEnd)
        val endAtLineStart = sel.selectionEnd == doc.getLineStartOffset(endLine0)
        return SelectionPayload.build(vf.path, startLine0, endLine0, endAtLineStart, text)
    }

    private data class Snapshot(
        val openFiles: List<String>,
        val activeFile: String,
        val selection: JsonObject?,
    )

    companion object {
        private const val DEBOUNCE_MS = 300

        fun getInstance(project: Project): FoxxyCodeEditorContextService =
            project.getService(FoxxyCodeEditorContextService::class.java)
    }
}
