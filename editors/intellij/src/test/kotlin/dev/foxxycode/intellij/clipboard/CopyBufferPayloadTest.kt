package dev.foxxycode.intellij.clipboard

import com.google.gson.JsonParser
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** Unit tests for [CopyBufferPayload] — the pure half of copy interception. */
class CopyBufferPayloadTest {

    @Test
    fun `selection matching is CRLF-insensitive and exact`() {
        assertTrue(CopyBufferPayload.matchesSelection("a\r\nb\r\n", "a\nb"))
        assertFalse(CopyBufferPayload.matchesSelection("a", "a\nb"))
        assertFalse(CopyBufferPayload.matchesSelection("a\nb", null))
        assertFalse(CopyBufferPayload.matchesSelection("a\nb", "  "))
    }

    @Test
    fun `reportable gates blank and oversize copies`() {
        assertTrue(CopyBufferPayload.reportable("x := 1"))
        assertFalse(CopyBufferPayload.reportable(null))
        assertFalse(CopyBufferPayload.reportable("   \n "))
        assertFalse(CopyBufferPayload.reportable("y".repeat(CopyBufferPayload.MAX_TEXT_BYTES + 1)))
    }

    @Test
    fun `terminal chain detection is by class name`() {
        assertTrue(
            CopyBufferPayload.looksLikeTerminalChain(
                listOf("com.jediterm.terminal.ui.TerminalPanel", "javax.swing.JPanel"),
            ),
        )
        assertTrue(
            CopyBufferPayload.looksLikeTerminalChain(
                listOf("org.jetbrains.plugins.terminal.ShellTerminalWidget"),
            ),
        )
        assertFalse(
            CopyBufferPayload.looksLikeTerminalChain(
                listOf("com.intellij.openapi.editor.impl.EditorComponentImpl", "javax.swing.JPanel"),
            ),
        )
    }

    @Test
    fun `bodies serialize to the copy-buffer request shape`() {
        val file = JsonParser.parseString(
            CopyBufferPayload.fileBody("C:\\ws\\Dockerfile", 21, 31, "FROM x"),
        ).asJsonObject
        assertEquals("file", file.get("kind").asString)
        assertEquals("C:\\ws\\Dockerfile", file.get("path").asString)
        assertEquals(21, file.get("startLine").asInt)
        assertEquals(31, file.get("endLine").asInt)

        val term = JsonParser.parseString(CopyBufferPayload.terminalBody("out")).asJsonObject
        assertEquals("terminal", term.get("kind").asString)
        assertEquals("out", term.get("text").asString)
    }
}
