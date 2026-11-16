package dev.foxxycode.intellij.diff

import dev.foxxycode.intellij.terminal.TerminalCommandOutput
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Unit tests for scraping a delegated terminal run out of the screen text.
 * Platform-free, so it runs in the plain JUnit4 `test` task.
 */
class TerminalCommandOutputTest {

    private val screen = listOf(
        "user@box:~/proj$ echo hello from the IDE terminal",
        "hello from the IDE terminal",
        "user@box:~/proj$ echo FOXXYCODE_EXIT_\$?",
        "FOXXYCODE_EXIT_0",
        "user@box:~/proj$ echo FOXXYCODE_DONE",
        "FOXXYCODE_DONE",
    ).joinToString("\n")

    @Test
    fun `output and exit code are picked up`() {
        val out = TerminalCommandOutput.extract(screen, "echo hello from the IDE terminal")
        assertEquals("hello from the IDE terminal\n(exit code 0)", out)
    }

    @Test
    fun `powershell boolean exit maps to a code`() {
        val ps = "PS C:\\proj> echo hi\nhi\nFOXXYCODE_EXIT_False\nFOXXYCODE_DONE"
        val out = TerminalCommandOutput.extract(ps, "echo hi")
        assertEquals("hi\n(exit code 1)", out)
    }

    @Test
    fun `cmd reports the exit code unknown`() {
        val cmd = "C:\\proj>echo hi\nhi\nFOXXYCODE_EXIT_\$?\nFOXXYCODE_DONE"
        val out = TerminalCommandOutput.extract(cmd, "echo hi")
        assertEquals("hi\n(exit code unknown (shell did not expand it))", out)
    }

    @Test
    fun `a screen without the marker keeps its tail`() {
        val out = TerminalCommandOutput.extract("still running", "echo hi")
        assertEquals("still running", out)
    }

    @Test
    fun `an empty output still reports the exit code`() {
        val out = TerminalCommandOutput.extract("FOXXYCODE_EXIT_3\nFOXXYCODE_DONE", "true")
        assertTrue(out.contains("exit code 3"))
    }
}
