package dev.foxxycode.intellij.diff

import com.google.gson.JsonParser
import com.intellij.diff.DiffContentFactory
import com.intellij.diff.DiffDialogHints
import com.intellij.diff.DiffManager
import com.intellij.diff.chains.SimpleDiffRequestChain
import com.intellij.diff.requests.SimpleDiffRequest
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.logger
import com.intellij.openapi.fileTypes.FileTypeManager
import com.intellij.openapi.progress.ProgressIndicator
import com.intellij.openapi.progress.ProgressManager
import com.intellij.openapi.progress.Task
import com.intellij.openapi.project.Project
import dev.foxxycode.intellij.FoxxyCodeBundle
import dev.foxxycode.intellij.FoxxyCodeNotifications
import dev.foxxycode.intellij.process.FoxxyCodeProcessManager
import java.net.HttpURLConnection
import java.net.URI
import java.net.URLEncoder
import java.nio.charset.StandardCharsets

/**
 * Opens every file change of a FoxxyCode session in the IDE's own diff viewer.
 *
 * The panel already shows a changed-files card, but reading a diff belongs in
 * the tool the user reads diffs in, so the card and the toolbar button both end
 * up here. A [SimpleDiffRequestChain] gives the native window its file selector,
 * which is the multi-file review UI the platform ships without pulling in the
 * VCS APIs the plugin does not depend on.
 */
class FoxxyCodeSessionChangesService(private val project: Project) {
    private val log = logger<FoxxyCodeSessionChangesService>()

    /** One changed file as reported by GET /foxxycode/sessions/{id}/changes. */
    private data class ChangedFile(
        val path: String,
        val status: String,
        val additions: Int,
        val deletions: Int,
        val binary: Boolean,
        val before: String,
        val after: String,
    )

    /**
     * Shows the changes of [sessionId], or of the session the panel last had
     * open when it is null. [focusPath] selects that file in the viewer when the
     * user clicked a row rather than the summary. Runs the fetch on a background
     * task so a slow or dead backend never freezes the IDE.
     */
    fun showChanges(sessionId: String? = null, focusPath: String? = null) {
        val base = FoxxyCodeProcessManager.getInstance(project).baseUrl
        if (base == null) {
            FoxxyCodeNotifications.warn(
                project,
                FoxxyCodeBundle.message("changes.title"),
                FoxxyCodeBundle.message("changes.notRunning"),
            )
            return
        }
        ProgressManager.getInstance().run(
            object : Task.Backgroundable(project, FoxxyCodeBundle.message("changes.loading"), true) {
                override fun run(indicator: ProgressIndicator) {
                    val id = sessionId?.takeIf { it.isNotBlank() } ?: lastSessionId(base)
                    if (id == null) {
                        notify(FoxxyCodeBundle.message("changes.noSession"))
                        return
                    }
                    val files = try {
                        fetchChanges(base, id)
                    } catch (e: Exception) {
                        log.warn("session changes fetch failed: ${e.message}")
                        notify(FoxxyCodeBundle.message("changes.failed"))
                        return
                    }
                    if (files.isEmpty()) {
                        notify(FoxxyCodeBundle.message("changes.none"))
                        return
                    }
                    ApplicationManager.getApplication().invokeLater {
                        if (!project.isDisposed) showChain(files, focusPath)
                    }
                }
            },
        )
    }

    private fun notify(message: String) {
        ApplicationManager.getApplication().invokeLater {
            if (!project.isDisposed) {
                FoxxyCodeNotifications.info(project, FoxxyCodeBundle.message("changes.title"), message)
            }
        }
    }

    private fun showChain(files: List<ChangedFile>, focusPath: String? = null) {
        val factory = DiffContentFactory.getInstance()
        val requests = files.map { file ->
            val type = FileTypeManager.getInstance().getFileTypeByFileName(fileName(file.path))
            // A binary file has no text sides; say so rather than showing two
            // empty panes that read as "nothing changed".
            val before = if (file.binary) FoxxyCodeBundle.message("changes.binaryBody") else file.before
            val after = if (file.binary) FoxxyCodeBundle.message("changes.binaryBody") else file.after
            SimpleDiffRequest(
                diffTitle(file),
                factory.create(project, before, type),
                factory.create(project, after, type),
                FoxxyCodeBundle.message("diff.window.before"),
                FoxxyCodeBundle.message("diff.window.after"),
            )
        }
        // The clicked row decides where the chain opens; the window's own
        // prev/next-file arrows and file list still cover the rest of the set.
        val index = focusPath?.takeIf { it.isNotBlank() }
            ?.let { p -> files.indexOfFirst { it.path == p } }
            ?.takeIf { it >= 0 } ?: 0
        val chain = SimpleDiffRequestChain(requests, index)
        DiffManager.getInstance().showDiff(project, chain, DiffDialogHints.DEFAULT)
    }

    /** The selector entry: path plus the counts, so the list reads like the card. */
    private fun diffTitle(file: ChangedFile): String =
        if (file.binary) {
            "${file.path}  (${file.status})"
        } else {
            "${file.path}  +${file.additions} −${file.deletions}"
        }

    // ---- http -------------------------------------------------------------------------

    /**
     * The session the SPA last opened in this project. The panel records it on
     * every session switch, so the toolbar button follows whatever chat the user
     * is looking at without needing a bridge into the webview.
     */
    private fun lastSessionId(base: String): String? {
        val body = httpGet(base, "foxxycode/project/last-session") ?: return null
        val obj = JsonParser.parseString(body).asJsonObject
        val id = obj.get("session_id")?.takeIf { !it.isJsonNull }?.asString
        return id?.takeIf { it.isNotBlank() }
    }

    private fun fetchChanges(base: String, sessionId: String): List<ChangedFile> {
        val encoded = URLEncoder.encode(sessionId, StandardCharsets.UTF_8.name())
        // include=content in one request: the native viewer needs both sides,
        // and a request per file would be a round trip per row.
        val body = httpGet(base, "foxxycode/sessions/$encoded/changes?include=content")
            ?: return emptyList()
        val files = JsonParser.parseString(body).asJsonObject.getAsJsonArray("files")
            ?: return emptyList()
        return files.map { element ->
            val obj = element.asJsonObject
            fun str(name: String) = obj.get(name)?.takeIf { !it.isJsonNull }?.asString ?: ""
            fun int(name: String) = obj.get(name)?.takeIf { !it.isJsonNull }?.asInt ?: 0
            ChangedFile(
                path = str("path"),
                status = str("status"),
                additions = int("additions"),
                deletions = int("deletions"),
                binary = obj.get("binary")?.takeIf { !it.isJsonNull }?.asBoolean ?: false,
                before = str("before"),
                after = str("after"),
            )
        }
    }

    private fun httpGet(base: String, path: String): String? {
        val url = (if (base.endsWith("/")) base else "$base/") + path
        val conn = URI.create(url).toURL().openConnection() as HttpURLConnection
        return try {
            conn.requestMethod = "GET"
            conn.connectTimeout = 3000
            conn.readTimeout = 15000
            if (conn.responseCode !in 200..299) {
                log.debug("GET $path returned ${conn.responseCode}")
                null
            } else {
                conn.inputStream.readBytes().toString(StandardCharsets.UTF_8)
            }
        } finally {
            conn.disconnect()
        }
    }

    private fun fileName(path: String): String = path.replace('\\', '/').substringAfterLast('/')

    companion object {
        fun getInstance(project: Project): FoxxyCodeSessionChangesService =
            project.getService(FoxxyCodeSessionChangesService::class.java)
    }
}
