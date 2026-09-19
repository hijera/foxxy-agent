package dev.foxxycode.intellij.diff

import com.google.gson.JsonObject
import com.intellij.diff.DiffContentFactory
import com.intellij.diff.DiffManager
import com.intellij.diff.requests.SimpleDiffRequest
import com.intellij.ide.actions.RevealFileAction
import com.intellij.notification.NotificationAction
import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.Disposable
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.command.WriteCommandAction
import com.intellij.openapi.diagnostic.logger
import com.intellij.openapi.diff.DiffColors
import com.intellij.openapi.editor.Editor
import com.intellij.openapi.editor.colors.TextAttributesKey
import com.intellij.openapi.editor.markup.HighlighterLayer
import com.intellij.openapi.editor.markup.HighlighterTargetArea
import com.intellij.openapi.editor.markup.MarkupModel
import com.intellij.openapi.editor.markup.RangeHighlighter
import com.intellij.openapi.fileEditor.FileEditorManager
import com.intellij.openapi.fileEditor.OpenFileDescriptor
import com.intellij.openapi.fileTypes.FileTypeManager
import com.intellij.openapi.project.Project
import com.intellij.openapi.util.SystemInfo
import com.intellij.openapi.vfs.LocalFileSystem
import com.intellij.openapi.vfs.VfsUtil
import dev.foxxycode.intellij.FoxxyCodeBundle
import dev.foxxycode.intellij.process.FoxxyCodeProcessManager
import dev.foxxycode.intellij.settings.FoxxyCodeSettings
import java.io.File
import java.io.OutputStreamWriter
import java.net.HttpURLConnection
import java.net.URI
import java.nio.charset.StandardCharsets
import java.nio.file.Path

/**
 * Native inline-diff bridge: subscribes to the foxxycode IDE event stream and renders each agent
 * file edit in the real editor with green/red line highlights and an Accept/Reject (or Revert)
 * decision, driven by IntelliJ's own diff APIs. Registered as a projectService.
 */
class FoxxyCodeIdeDiffService(private val project: Project) : Disposable {
    private val log = logger<FoxxyCodeIdeDiffService>()

    private var client: FoxxyCodeIdeEventClient? = null
    private var clientBase: String? = null

    private class HighlightSet(val model: MarkupModel, val markers: List<RangeHighlighter>)

    // Active highlighters per absolute (normalized) path, cleared before re-rendering a file.
    private val highlighters = HashMap<String, HighlightSet>()

    /**
     * Starts (or, after a server restart on a new port, re-points) the event stream. Safe to
     * call repeatedly; a no-op when already connected to the current base URL.
     */
    fun startIfNeeded() {
        val base = FoxxyCodeProcessManager.getInstance(project).baseUrl ?: return
        if (client != null && clientBase == base) return
        client?.stop()
        val c = FoxxyCodeIdeEventClient(base) { ev -> onEvent(ev) }
        client = c
        clientBase = base
        c.start()
    }

    private fun onEvent(ev: FoxxyCodeEditEvent) {
        // User-initiated open ("Show in IDE"): the plan file lives in the session bundle
        // outside the project, and it is not a diff — so it runs before the nativeDiffs
        // and in-project guards below.
        if (ev.isOpenFile) {
            ApplicationManager.getApplication().invokeLater {
                if (project.isDisposed) return@invokeLater
                openFile(ev.path)
            }
            return
        }
        // Exported transcript written to the OS temp dir. Same reasoning as
        // open_file — user-initiated and outside the project — but the document
        // is a download, so it goes to the file manager, not an editor tab.
        if (ev.isRevealFile) {
            ApplicationManager.getApplication().invokeLater {
                if (project.isDisposed) return@invokeLater
                revealFile(ev.path)
            }
            return
        }
        if (!FoxxyCodeSettings.getInstance().state.nativeDiffs) return
        if (!isInProject(ev.path)) return
        ApplicationManager.getApplication().invokeLater {
            if (project.isDisposed) return@invokeLater
            handle(ev)
        }
    }

    /** Opens a file in the editor area and focuses it (no highlights, no diff). */
    private fun openFile(path: String) {
        val target = path.trim()
        if (target.isEmpty()) return
        val vf = try {
            LocalFileSystem.getInstance().refreshAndFindFileByNioFile(Path.of(target))
        } catch (e: Exception) {
            log.debug("open_file rejected path $target: ${e.message}")
            null
        } ?: return
        FileEditorManager.getInstance(project).openTextEditor(OpenFileDescriptor(project, vf), true)
    }

    /**
     * Selects an exported document in the OS file manager. The IDE cannot show a
     * PDF or a DOCX usefully, and the panel's own webview cannot save one at all
     * (JCEF drops downloads no handler claims), so the server writes the file and
     * this puts the user in front of it.
     */
    private fun revealFile(path: String) {
        val target = path.trim()
        if (target.isEmpty()) return
        val file = File(target)
        if (!file.isFile) {
            log.debug("reveal_file skipped missing path $target")
            return
        }
        try {
            RevealFileAction.openFile(file)
        } catch (e: Exception) {
            log.debug("reveal_file failed for $target: ${e.message}")
        }
    }

    private fun handle(ev: FoxxyCodeEditEvent) {
        when {
            ev.isProposed -> {
                if (FoxxyCodeSettings.getInstance().state.autoApproveEdits) {
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
    private fun openAndHighlight(ev: FoxxyCodeEditEvent, useAfterRanges: Boolean): Editor? {
        val vf = LocalFileSystem.getInstance().refreshAndFindFileByNioFile(Path.of(ev.path)) ?: return null
        // An applied edit is already on disk, but a file that is open (or merely cached by the
        // VFS) keeps its old document until a refresh notices the change. Refresh now, so the
        // document is reloaded before it is highlighted rather than after.
        if (useAfterRanges) VfsUtil.markDirtyAndRefresh(false, false, false, vf)
        val editor = FileEditorManager.getInstance(project)
            .openTextEditor(OpenFileDescriptor(project, vf), true) ?: return null

        clearHighlights(ev.path)
        val model = editor.markupModel
        val list = ArrayList<RangeHighlighter>()
        val doc = editor.document
        val side = if (useAfterRanges) ChangedLineRanges.Side.AFTER else ChangedLineRanges.Side.BEFORE
        // Ranges are computed against the document's actual text, not taken from the event's
        // line numbers: if the reload has still not happened (unsaved changes in the editor),
        // lines the document does not contain stay unhighlighted instead of landing elsewhere.
        for (r in ChangedLineRanges.inDocument(ev.before, ev.after, doc.immutableCharSequence, side)) {
            if (r.start >= doc.lineCount) continue
            val from = doc.getLineStartOffset(r.start)
            val to = doc.getLineEndOffset(minOf(r.end, doc.lineCount) - 1)
            val hl = model.addRangeHighlighter(
                attributesKey(r.kind), from, to, HighlighterLayer.ADDITIONAL_SYNTAX,
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

    /**
     * The diff viewer's own color keys. A key is resolved against the editor's color scheme on
     * every paint, so the highlight suits whichever scheme is active and follows a theme switch
     * while it is on screen. The fixed JBColor used before did neither: it picks its variant from
     * the look-and-feel flag, not the editor scheme, and could paint dark green behind the dark
     * text of a light scheme.
     */
    private fun attributesKey(kind: ChangedLineRanges.Kind): TextAttributesKey = when (kind) {
        ChangedLineRanges.Kind.INSERTED -> DiffColors.DIFF_INSERTED
        ChangedLineRanges.Kind.DELETED -> DiffColors.DIFF_DELETED
        ChangedLineRanges.Kind.MODIFIED -> DiffColors.DIFF_MODIFIED
    }

    // ---- notifications / decisions ----------------------------------------------------

    private fun group() =
        NotificationGroupManager.getInstance().getNotificationGroup("FoxxyCode")

    private fun notifyProposed(ev: FoxxyCodeEditEvent) {
        val n = group().createNotification(
            FoxxyCodeBundle.message("diff.notify.proposed.title", fileName(ev.path)),
            FoxxyCodeBundle.message("diff.notify.proposed.content"),
            NotificationType.INFORMATION,
        )
        n.addAction(NotificationAction.createSimple(FoxxyCodeBundle.message("diff.action.accept")) {
            respondPermission(ev.sessionId, ev.toolCallId, "allow")
            n.expire()
        })
        n.addAction(NotificationAction.createSimple(FoxxyCodeBundle.message("diff.action.reject")) {
            respondPermission(ev.sessionId, ev.toolCallId, "reject")
            clearHighlights(ev.path)
            n.expire()
        })
        n.addAction(NotificationAction.createSimple(FoxxyCodeBundle.message("diff.action.showDiff")) { showDiff(ev) })
        n.notify(project)
    }

    private fun notifyApplied(ev: FoxxyCodeEditEvent) {
        val n = group().createNotification(
            FoxxyCodeBundle.message("diff.notify.applied.title", fileName(ev.path)),
            FoxxyCodeBundle.message("diff.notify.applied.content"),
            NotificationType.INFORMATION,
        )
        n.addAction(NotificationAction.createSimple(FoxxyCodeBundle.message("diff.action.revert")) {
            revert(ev)
            clearHighlights(ev.path)
            n.expire()
        })
        n.addAction(NotificationAction.createSimple(FoxxyCodeBundle.message("diff.action.showDiff")) { showDiff(ev) })
        n.notify(project)
    }

    private fun showDiff(ev: FoxxyCodeEditEvent) {
        val factory = DiffContentFactory.getInstance()
        val type = FileTypeManager.getInstance().getFileTypeByFileName(fileName(ev.path))
        val c1 = factory.create(project, ev.before, type)
        val c2 = factory.create(project, ev.after, type)
        val req = SimpleDiffRequest(
            fileName(ev.path), c1, c2,
            FoxxyCodeBundle.message("diff.window.before"),
            FoxxyCodeBundle.message("diff.window.after"),
        )
        DiffManager.getInstance().showDiff(project, req)
    }

    /** Native per-edit rollback: restore the file's pre-edit content with undo support. */
    private fun revert(ev: FoxxyCodeEditEvent) {
        val vf = LocalFileSystem.getInstance().refreshAndFindFileByNioFile(Path.of(ev.path)) ?: return
        try {
            WriteCommandAction.runWriteCommandAction(project, FoxxyCodeBundle.message("diff.command.revert"), null, {
                vf.setBinaryContent(ev.before.toByteArray(StandardCharsets.UTF_8))
            })
        } catch (e: Exception) {
            log.warn("revert failed", e)
        }
    }

    // ---- http -------------------------------------------------------------------------

    private fun respondPermission(sessionId: String, toolCallId: String, optionId: String) {
        if (sessionId.isBlank() || toolCallId.isBlank()) return
        val base = FoxxyCodeProcessManager.getInstance(project).baseUrl ?: return
        val url = (if (base.endsWith("/")) base else "$base/") + "foxxycode/sessions/$sessionId/permission"
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
        fun getInstance(project: Project): FoxxyCodeIdeDiffService =
            project.getService(FoxxyCodeIdeDiffService::class.java)
    }
}
