package dev.foxxycode.intellij.editor

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** Unit tests for [SelectionPayload] — the pure half of selection reporting. */
class SelectionPayloadTest {

    @Test
    fun `converts 0-based lines to a 1-based inclusive range`() {
        val json = SelectionPayload.build("/ws/a.go", 20, 30, endAtLineStart = false, text = "x := 1")!!
        assertEquals(21, json.get("startLine").asInt)
        assertEquals(31, json.get("endLine").asInt)
        assertEquals("/ws/a.go", json.get("file").asString)
        assertEquals("x := 1", json.get("text").asString)
    }

    @Test
    fun `excludes a final line the selection only touches at column 0`() {
        val ln = SelectionPayload.lines(20, 31, endAtLineStart = true)!!
        assertEquals(21, ln.startLine)
        assertEquals(31, ln.endLine)
    }

    @Test
    fun `drops blank selections and blank files`() {
        assertNull(SelectionPayload.build("/ws/a.go", 1, 1, false, ""))
        assertNull(SelectionPayload.build("/ws/a.go", 1, 1, false, "  \n "))
        assertNull(SelectionPayload.build(" ", 1, 1, false, "text"))
        // A caret at column 0 of the same line selects nothing.
        assertNull(SelectionPayload.build("/ws/a.go", 3, 3, endAtLineStart = true, text = ""))
    }

    @Test
    fun `caps the text keeping the tail`() {
        val long = "y".repeat(SelectionPayload.MAX_SELECTION_BYTES + 500)
        val json = SelectionPayload.build("/ws/a.go", 0, 400, false, long)!!
        val text = json.get("text").asString
        assertEquals(SelectionPayload.MAX_SELECTION_BYTES, text.length)
        assertTrue(long.endsWith(text))
    }
}
