package dev.foxxycode.intellij.terminal

import com.google.gson.JsonArray
import com.google.gson.JsonObject
import com.intellij.openapi.Disposable
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.logger
import com.intellij.openapi.project.Project
import com.intellij.openapi.wm.ToolWindowManager
import com.intellij.util.Alarm
import dev.foxxycode.intellij.process.FoxxyCodeProcessManager
import dev.foxxycode.intellij.settings.FoxxyCodeSettings
import java.awt.Component
import java.awt.Container
import java.io.OutputStreamWriter
import java.net.HttpURLConnection
import java.net.URI
import java.nio.charset.StandardCharsets

/**
 * Reports the open IDE terminals and their recent output to the foxxycode backend
 * (POST /foxxycode/ide/terminal-state) so the model can see the terminals the user is
 * working in — the same "@terminal" context cline provides. The backend injects the
 * latest snapshot into each agent turn and resolves @terminal mentions.
 *
 * Unlike the editor-context service there is no message-bus event for terminal output, so
 * this polls the Terminal tool window on a timer. To stay compatible across the JetBrains
 * terminal reworks (223 → new terminal) it uses only stable platform APIs (tool window +
 * content manager for tab names / focus) and reflects into the JediTerm text buffer for the
 * output — every reflective step is wrapped so an unknown terminal API simply yields no
 * output rather than breaking the plugin.
 *
 * Gated by [FoxxyCodeSettings.State.trackTerminals] (default true). Registered as a
 * projectService; started from [dev.foxxycode.intellij.ui.FoxxyCodeBrowserPanel].
 */
class FoxxyCodeTerminalContextService(private val project: Project) : Disposable {
    private val log = logger<FoxxyCodeTerminalContextService>()
    private val alarm = Alarm(Alarm.ThreadToUse.SWING_THREAD, this)

    private var started = false
    private var lastPayload: String? = null

    /** Begins polling (idempotent). */
    fun startIfNeeded() {
        if (!started) {
            started = true
            schedule()
        }
    }

    override fun dispose() {
        alarm.cancelAllRequests()
    }

    private fun schedule() {
        if (project.isDisposed) return
        alarm.cancelAllRequests()
        alarm.addRequest({ poll() }, POLL_MS)
    }

    /** Runs on the EDT (SWING_THREAD alarm): safe to touch Swing / the terminal widget. */
    private fun poll() {
        if (project.isDisposed) return
        try {
            if (FoxxyCodeSettings.getInstance().state.trackTerminals) {
                val base = FoxxyCodeProcessManager.getInstance(project).baseUrl
                if (base != null) {
                    val body = buildBody(collect())
                    if (body != lastPayload) {
                        lastPayload = body
                        val url = (if (base.endsWith("/")) base else "$base/") + "foxxycode/ide/terminal-state"
                        // Never block the EDT on network — POST from a pooled thread.
                        ApplicationManager.getApplication().executeOnPooledThread { post(url, body) }
                    }
                }
            }
        } catch (t: Throwable) {
            log.debug("terminal poll failed", t)
        } finally {
            schedule()
        }
    }

    private data class Term(val id: String, val name: String, val output: String, val active: Boolean)

    private fun collect(): List<Term> {
        val tw = ToolWindowManager.getInstance(project).getToolWindow(TERMINAL_TOOL_WINDOW_ID) ?: return emptyList()
        val cm = tw.contentManager
        val selected = cm.selectedContent
        val out = ArrayList<Term>()
        cm.contents.forEachIndexed { i, content ->
            val name = content.displayName?.takeIf { it.isNotBlank() } ?: "Terminal ${i + 1}"
            val widget = findTerminalWidget(content.component)
            val output = if (widget != null) readScreenText(widget) else ""
            out.add(Term(id = (i + 1).toString(), name = name, output = output, active = content === selected))
        }
        return out
    }

    /** Depth-first search of the content's Swing tree for a JediTerm widget. */
    private fun findTerminalWidget(root: Component?): Component? {
        if (root == null) return null
        if (hasMethod(root.javaClass, "getTerminalTextBuffer")) return root
        if (root is Container) {
            for (child in root.components) {
                val found = findTerminalWidget(child)
                if (found != null) return found
            }
        }
        return null
    }

    /** Reads the visible screen text from a JediTerm widget via reflection. */
    private fun readScreenText(widget: Component): String {
        return try {
            val buffer = widget.javaClass.getMethod("getTerminalTextBuffer").invoke(widget) ?: return ""
            val screen = buffer.javaClass.getMethod("getScreenLines").invoke(buffer) as? String ?: return ""
            capTail(screen.trimEnd(), MAX_OUTPUT_CHARS)
        } catch (t: Throwable) {
            ""
        }
    }

    private fun buildBody(terms: List<Term>): String {
        val arr = JsonArray()
        for (t in terms) {
            arr.add(
                JsonObject().apply {
                    addProperty("id", t.id)
                    addProperty("name", t.name)
                    if (t.output.isNotEmpty()) addProperty("output", t.output)
                    if (t.active) addProperty("active", true)
                },
            )
        }
        return JsonObject().apply { add("terminals", arr) }.toString()
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
            log.debug("terminal-state POST failed: ${e.message}")
        }
    }

    companion object {
        private const val POLL_MS = 2000
        private const val MAX_OUTPUT_CHARS = 8000
        private const val TERMINAL_TOOL_WINDOW_ID = "Terminal"

        fun getInstance(project: Project): FoxxyCodeTerminalContextService =
            project.getService(FoxxyCodeTerminalContextService::class.java)

        private fun hasMethod(cls: Class<*>, name: String): Boolean =
            try {
                cls.getMethod(name)
                true
            } catch (t: Throwable) {
                false
            }

        private fun capTail(s: String, maxChars: Int): String =
            if (s.length <= maxChars) s else s.substring(s.length - maxChars)
    }
}
