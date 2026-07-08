package dev.foxxycode.intellij.editor

import com.google.gson.JsonArray
import com.google.gson.JsonObject
import com.intellij.openapi.Disposable
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.logger
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
    private val alarm = Alarm(Alarm.ThreadToUse.POOLED_THREAD, this)

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

    private fun report() {
        if (project.isDisposed) return
        if (!FoxxyCodeSettings.getInstance().state.trackOpenFiles) return
        val base = FoxxyCodeProcessManager.getInstance(project).baseUrl ?: return

        val snapshot = ApplicationManager.getApplication().runReadAction<Snapshot> {
            if (project.isDisposed) return@runReadAction Snapshot(emptyList(), "")
            val fem = FileEditorManager.getInstance(project)
            val open = fem.openFiles.filter { it.fileSystem is LocalFileSystem }.map { it.path }
            val active = fem.selectedFiles.firstOrNull { it.fileSystem is LocalFileSystem }?.path ?: ""
            Snapshot(open, active)
        }

        // De-duplicate, keeping the active file first (mirrors the VSCode reporter).
        val ordered = LinkedHashSet<String>()
        if (snapshot.activeFile.isNotBlank()) ordered.add(snapshot.activeFile)
        ordered.addAll(snapshot.openFiles)

        val body = JsonObject().apply {
            add("openFiles", JsonArray().apply { ordered.forEach { add(it) } })
            addProperty("activeFile", snapshot.activeFile)
        }.toString()
        if (body == lastPayload) return
        lastPayload = body

        val url = (if (base.endsWith("/")) base else "$base/") + "foxxycode/ide/editor-state"
        post(url, body)
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

    private data class Snapshot(val openFiles: List<String>, val activeFile: String)

    companion object {
        private const val DEBOUNCE_MS = 300

        fun getInstance(project: Project): FoxxyCodeEditorContextService =
            project.getService(FoxxyCodeEditorContextService::class.java)
    }
}
