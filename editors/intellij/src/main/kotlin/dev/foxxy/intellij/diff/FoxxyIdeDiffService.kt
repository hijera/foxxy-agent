package dev.foxxy.intellij.diff

import com.google.gson.JsonObject
import com.intellij.diff.DiffContentFactory
import com.intellij.diff.DiffManager
import com.intellij.diff.comparison.ComparisonManager
import com.intellij.diff.comparison.ComparisonPolicy
import com.intellij.diff.requests.SimpleDiffRequest
import com.intellij.notification.NotificationAction
import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.Disposable
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.command.WriteCommandAction
import com.intellij.openapi.diagnostic.logger
import com.intellij.openapi.editor.Editor
import com.intellij.openapi.editor.markup.HighlighterLayer
import com.intellij.openapi.editor.markup.HighlighterTargetArea
import com.intellij.openapi.editor.markup.MarkupModel
import com.intellij.openapi.editor.markup.RangeHighlighter
import com.intellij.openapi.editor.markup.TextAttributes
import com.intellij.openapi.fileEditor.FileEditorManager
import com.intellij.openapi.fileEditor.OpenFileDescriptor
import com.intellij.openapi.fileTypes.FileTypeManager
import com.intellij.openapi.progress.DumbProgressIndicator
import com.intellij.openapi.project.Project
import com.intellij.openapi.util.SystemInfo
import com.intellij.openapi.vfs.LocalFileSystem
import com.intellij.ui.JBColor
import dev.foxxy.intellij.FoxxyBundle
import dev.foxxy.intellij.process.FoxxyProcessManager
import dev.foxxy.intellij.settings.FoxxySettings
import java.awt.Color
import java.io.OutputStreamWriter
import java.net.HttpURLConnection
import java.net.URI
import java.nio.charset.StandardCharsets
import java.nio.file.Path

/**
 * Native inline-diff bridge: subscribes to the foxxy IDE event stream and renders each agent
 * file edit in the real editor with green/red line highlights and an Accept/Reject (or Revert)
 * decision, driven by IntelliJ's own diff APIs. Registered as a projectService.
 */
class FoxxyIdeDiffService(private val project: Project) : Disposable {
    private val log = logger<FoxxyIdeDiffService>()

    private var client: FoxxyIdeEventClient? = null
    private var clientBase: String? = null

    private class HighlightSet(val model: MarkupModel, val markers: List<RangeHighlighter>)

    // Active highlighters per absolute (normalized) path, cleared before re-rendering a file.
    private val highlighters = HashMap<String, HighlightSet>()

    /**
     * Starts (or, after a server restart on a new port, re-points) the event stream. Safe to
     * call repeatedly; a no-op when already connected to the current base URL.
     */
    fun startIfNeeded() {
        val base = FoxxyProcessManager.getInstance(project).baseUrl ?: return
        if (client != null && clientBase == base) return
        client?.stop()
        val c = FoxxyIdeEventClient(base) { ev -> onEvent(ev) }
        client = c
        clientBase = base
        c.start()
    }

    private fun onEvent(ev: FoxxyEditEvent) {
        if (!FoxxySettings.getInstance().state.nativeDiffs) return
        if (!isInProject(ev.path)) return
        ApplicationManager.getApplication().invokeLater {
            if (project.isDisposed) return@invokeLater
            handle(ev)
        }
    }

    private fun handle(ev: FoxxyEditEvent) {
        when {
            ev.isProposed -> {
                if (FoxxySettings.getInstance().state.autoApproveEdits) {
                    respondPermission(ev.sessionId, ev.toolCallId, "allow")
                    return
                }
                openAndHighlight(ev, useAfterRanges = false)
                notifyProposed(ev)
            }
            ev.isApplied -> {
                openAndHighlight(ev, useAfterRanges = true)
                notifyApplied(ev)
            }
        }
    }

    // ---- inline rendering -------------------------------------------------------------

    /** Opens the file and highlights changed line ranges; returns the editor (or null). */
    private fun openAndHighlight(ev: FoxxyEditEvent, useAfterRanges: Boolean): Editor? {
        val vf = LocalFileSystem.getInstance().refreshAndFindFileByNioFile(Path.of(ev.path)) ?: return null
        val editor = FileEditorManager.getInstance(project)
            .openTextEditor(OpenFileDescriptor(project, vf), true) ?: return null

        clearHighlights(ev.path)
        val model = editor.markupModel
        val list = ArrayList<RangeHighlighter>()
        val doc = editor.document
        val fragments = try {
            ComparisonManager.getInstance()
                .compareLines(ev.before, ev.after, ComparisonPolicy.DEFAULT, DumbProgressIndicator.INSTANCE)
        } catch (e: Exception) {
            log.debug("compareLines failed: ${e.message}")
            emptyList()
        }
        for (f in fragments) {
            val startLine = if (useAfterRanges) f.startLine2 else f.startLine1
            val endLine = if (useAfterRanges) f.endLine2 else f.endLine1
            if (endLine <= startLine) continue // pure deletion has no line in this side
            val last = doc.lineCount
            if (startLine >= last) continue
            val from = doc.getLineStartOffset(startLine.coerceIn(0, last - 1))
            val to = doc.getLineEndOffset((endLine - 1).coerceIn(0, last - 1))
            val hl = model.addRangeHighlighter(
                from, to, HighlighterLayer.ADDITIONAL_SYNTAX, changedLineAttributes(),
                HighlighterTargetArea.LINES_IN_RANGE,
            )
            list.add(hl)
        }
        if (list.isNotEmpty()) highlighters[normalize(ev.path)] = HighlightSet(model, list)
        return editor
    }

    private fun clearHighlights(path: String) {
        val set = highlighters.remove(normalize(path)) ?: return
        for (hl in set.markers) {
            try {
                set.model.removeHighlighter(hl)
            } catch (_: Exception) {
            }
        }
    }

    private fun changedLineAttributes(): TextAttributes {
        val bg = JBColor(Color(0xE6, 0xFF, 0xE6), Color(0x2A, 0x3A, 0x2A)) // subtle green, light/dark
        return TextAttributes().apply { backgroundColor = bg }
    }

    // ---- notifications / decisions ----------------------------------------------------

    private fun group() =
        NotificationGroupManager.getInstance().getNotificationGroup("Foxxy")

    private fun notifyProposed(ev: FoxxyEditEvent) {
        val n = group().createNotification(
            FoxxyBundle.message("diff.notify.proposed.title", fileName(ev.path)),
            FoxxyBundle.message("diff.notify.proposed.content"),
            NotificationType.INFORMATION,
        )
        n.addAction(NotificationAction.createSimple(FoxxyBundle.message("diff.action.accept")) {
            respondPermission(ev.sessionId, ev.toolCallId, "allow")
            n.expire()
        })
        n.addAction(NotificationAction.createSimple(FoxxyBundle.message("diff.action.reject")) {
            respondPermission(ev.sessionId, ev.toolCallId, "reject")
            clearHighlights(ev.path)
            n.expire()
        })
        n.addAction(NotificationAction.createSimple(FoxxyBundle.message("diff.action.showDiff")) { showDiff(ev) })
        n.notify(project)
    }

    private fun notifyApplied(ev: FoxxyEditEvent) {
        val n = group().createNotification(
            FoxxyBundle.message("diff.notify.applied.title", fileName(ev.path)),
            FoxxyBundle.message("diff.notify.applied.content"),
            NotificationType.INFORMATION,
        )
        n.addAction(NotificationAction.createSimple(FoxxyBundle.message("diff.action.revert")) {
            revert(ev)
            clearHighlights(ev.path)
            n.expire()
        })
        n.addAction(NotificationAction.createSimple(FoxxyBundle.message("diff.action.showDiff")) { showDiff(ev) })
        n.notify(project)
    }

    private fun showDiff(ev: FoxxyEditEvent) {
        val factory = DiffContentFactory.getInstance()
        val type = FileTypeManager.getInstance().getFileTypeByFileName(fileName(ev.path))
        val c1 = factory.create(project, ev.before, type)
        val c2 = factory.create(project, ev.after, type)
        val req = SimpleDiffRequest(
            fileName(ev.path), c1, c2,
            FoxxyBundle.message("diff.window.before"),
            FoxxyBundle.message("diff.window.after"),
        )
        DiffManager.getInstance().showDiff(project, req)
    }

    /** Native per-edit rollback: restore the file's pre-edit content with undo support. */
    private fun revert(ev: FoxxyEditEvent) {
        val vf = LocalFileSystem.getInstance().refreshAndFindFileByNioFile(Path.of(ev.path)) ?: return
        try {
            WriteCommandAction.runWriteCommandAction(project, FoxxyBundle.message("diff.command.revert"), null, {
                vf.setBinaryContent(ev.before.toByteArray(StandardCharsets.UTF_8))
            })
        } catch (e: Exception) {
            log.warn("revert failed", e)
        }
    }

    // ---- http -------------------------------------------------------------------------

    private fun respondPermission(sessionId: String, toolCallId: String, optionId: String) {
        if (sessionId.isBlank() || toolCallId.isBlank()) return
        val base = FoxxyProcessManager.getInstance(project).baseUrl ?: return
        val url = (if (base.endsWith("/")) base else "$base/") + "coddy/sessions/$sessionId/permission"
        val body = JsonObject().apply {
            addProperty("toolCallId", toolCallId)
            addProperty("optionId", optionId)
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
                log.warn("permission POST failed: ${e.message}")
            }
        }
    }

    // ---- helpers ----------------------------------------------------------------------

    private fun isInProject(path: String): Boolean {
        val base = project.basePath ?: return false
        return normalize(path).startsWith(normalize(base))
    }

    private fun normalize(p: String): String {
        val s = p.replace('\\', '/')
        return if (SystemInfo.isFileSystemCaseSensitive) s else s.lowercase()
    }

    private fun fileName(path: String): String = path.replace('\\', '/').substringAfterLast('/')

    override fun dispose() {
        client?.stop()
        client = null
        for (set in highlighters.values) {
            for (hl in set.markers) {
                try {
                    set.model.removeHighlighter(hl)
                } catch (_: Exception) {
                }
            }
        }
        highlighters.clear()
    }

    companion object {
        fun getInstance(project: Project): FoxxyIdeDiffService =
            project.getService(FoxxyIdeDiffService::class.java)
    }
}
