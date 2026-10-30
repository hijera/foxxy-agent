package dev.foxxycode.intellij.terminal

import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.logger
import com.intellij.openapi.project.Project
import java.awt.Component
import java.awt.Container

/**
 * Runs a delegated `run_command` in an IDE terminal tab: the JetBrains terminal plugin
 * opens a local shell widget and the command is typed into it, so the user sees it run
 * in the Terminal tool window instead of a hidden process of the agent.
 *
 * The terminal plugin is an optional dependency (a host without it keeps working), so
 * every step reflects into `org.jetbrains.plugins.terminal.TerminalView` /
 * `ShellTerminalWidget` the way [FoxxyCodeTerminalContextService] reads the screen, and
 * an unknown terminal API means "not handled" - the agent runs the command itself.
 *
 * Completion and output are read from the terminal screen: a `FOXXYCODE_DONE` marker is
 * queued after the command (plus a `FOXXYCODE_EXIT_$?` line that reports the exit code
 * in a shell family-dependent way). The visible screen is what the terminal holds, so
 * very long output is captured as its tail.
 */
object FoxxyCodeTerminalCommandRunner {
    private val log = logger<FoxxyCodeTerminalCommandRunner>()

    data class Result(val handled: Boolean, val output: String = "", val error: String = "")

    private const val DONE_MARKER = "FOXXYCODE_DONE"
    private const val EXIT_MARKER = "FOXXYCODE_EXIT_"
    private const val TAB_PREFIX = "FoxxyCode: "
    private const val POLL_MS = 200L
    private const val SHELL_STARTUP_MS = 800L

    /**
     * @param timeoutMs how long to wait for the command to finish on screen
     */
    fun run(project: Project, command: String, workDir: String?, timeoutMs: Long): Result {
        val widget: Component = try {
            createWidget(project, command, workDir)
        } catch (t: Throwable) {
            log.debug("terminal widget unavailable: $t")
            return Result(handled = false)
        }
        try {
            Thread.sleep(SHELL_STARTUP_MS)
            typeInto(widget, command)
            typeInto(widget, "echo $EXIT_MARKER" + "$?")
            typeInto(widget, "echo $DONE_MARKER")
        } catch (t: Throwable) {
            log.debug("terminal command send failed: $t")
            return Result(handled = false)
        }
        val deadline = System.currentTimeMillis() + timeoutMs
        while (System.currentTimeMillis() < deadline) {
            val screen = readScreen(widget)
            if (screen.contains(DONE_MARKER)) {
                return Result(handled = true, output = TerminalCommandOutput.extract(screen, command))
            }
            Thread.sleep(POLL_MS)
        }
        return Result(
            handled = true,
            error = "command still running in the IDE terminal after ${timeoutMs / 1000}s; " +
                "check the Terminal tool window for its output",
        )
    }

    /** Creates the shell tab on the EDT and types the command into it. */
    private fun createWidget(project: Project, command: String, workDir: String?): Component {
        var widget: Component? = null
        ApplicationManager.getApplication().invokeAndWait {
            val cls = Class.forName("org.jetbrains.plugins.terminal.TerminalView")
            val view = cls.getMethod("getInstance", Project::class.java).invoke(null, project)
            val tabName = (TAB_PREFIX + command).take(60)
            widget = cls.getMethod("createLocalShellWidget", String::class.java, String::class.java)
                .invoke(view, tabName, workDir ?: project.basePath) as? Component
        }
        return widget ?: throw IllegalStateException("no terminal widget")
    }

    /** Types one line into the shell (a queued line runs after the previous one ends). */
    private fun typeInto(widget: Component, line: String) {
        widget.javaClass.getMethod("executeCommand", String::class.java).invoke(widget, line)
    }

    private fun readScreen(widget: Component): String {
        return try {
            val buffer = widget.javaClass.getMethod("getTerminalTextBuffer").invoke(widget) ?: return ""
            buffer.javaClass.getMethod("getScreenLines").invoke(buffer) as? String ?: ""
        } catch (t: Throwable) {
            ""
        }
    }
}

/** Pure screen-scraping of a delegated terminal run; unit-tested without the platform. */
object TerminalCommandOutput {
    /**
     * Picks the command's own output out of the terminal screen: the text between the
     * echoed command line and the `FOXXYCODE_DONE` marker. The exit code line is
     * interpreted per shell family - `FOXXYCODE_EXIT_<n>` in a POSIX shell,
     * `FOXXYCODE_EXIT_True/False` in PowerShell, and the literal `$?` where the shell
     * did not expand it (cmd), where the exit code is reported as unknown.
     */
    fun extract(screen: String, command: String): String {
        val lines = screen.replace("\r\n", "\n").split('\n')
        val doneAt = lines.indexOfLast { it.contains("FOXXYCODE_DONE") }
        val cmdAt = lines.indexOfLast { it.contains(command) && !it.contains("FOXXYCODE_") }
        if (doneAt < 0) return screen.trimEnd()
        val from = if (cmdAt in 0 until doneAt) cmdAt + 1 else 0
        val body = lines.subList(from, doneAt)
            .filterNot { it.contains("FOXXYCODE_") }
            .joinToString("\n")
            .trimEnd()
        val exitLine = lines.take(doneAt).lastOrNull { it.contains("FOXXYCODE_EXIT_") }.orEmpty()
        val exit = when {
            Regex("""FOXXYCODE_EXIT_(\d+)\s*$""").find(exitLine)?.groupValues?.get(1) != null ->
                Regex("""FOXXYCODE_EXIT_(\d+)\s*$""").find(exitLine)!!.groupValues[1]
            exitLine.contains("FOXXYCODE_EXIT_True") -> "0"
            exitLine.contains("FOXXYCODE_EXIT_False") -> "1"
            else -> ""
        }
        val exitNote = if (exit.isEmpty()) "exit code unknown (shell did not expand it)" else "exit code $exit"
        return if (body.isEmpty()) "($exitNote)" else body + "\n($exitNote)"
    }
}
