package dev.foxxycode.intellij.autocomplete

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Unit tests for [SuggestionText] — the pure half of inline completion (prefix cache, inlay split,
 * typing filter). Platform-free, so it runs in the plain JUnit4 `test` task.
 */
class SuggestionTextTest {

    @Test
    fun `typing what was suggested re-renders the remainder without asking again`() {
        assertEquals(" + b", SuggestionText.advance("a + b", "a"))
        assertEquals("+ b", SuggestionText.advance("a + b", "a "))
    }

    @Test
    fun `typing the suggestion out in full leaves nothing to draw`() {
        assertEquals("", SuggestionText.advance("a + b", "a + b"))
    }

    @Test
    fun `typing something else invalidates the suggestion`() {
        assertNull(SuggestionText.advance("a + b", "x"))
        assertNull(SuggestionText.advance("a + b", "a - "))
    }

    @Test
    fun `an empty edit keeps the suggestion as it is`() {
        assertEquals("a + b", SuggestionText.advance("a + b", ""))
    }

    @Test
    fun `a single-line suggestion needs no block inlay`() {
        val (head, tail) = SuggestionText.split("return a + b")
        assertEquals("return a + b", head)
        assertTrue(tail.isEmpty())
    }

    @Test
    fun `a multi-line suggestion splits into the caret line and the lines below`() {
        val (head, tail) = SuggestionText.split("if err != nil {\n\treturn err\n}")
        assertEquals("if err != nil {", head)
        assertEquals(listOf("\treturn err", "}"), tail)
    }

    @Test
    fun `CRLF does not leak into the rendered lines`() {
        val (head, tail) = SuggestionText.split("a\r\nb")
        assertEquals("a", head)
        assertEquals(listOf("b"), tail)
    }

    @Test
    fun `a suggestion that starts on the next line has an empty caret-line part`() {
        val (head, tail) = SuggestionText.split("\n\treturn err")
        assertEquals("", head)
        assertEquals(listOf("\treturn err"), tail)
    }

    @Test
    fun `a request is skipped inside a word, after a closer, and while indenting`() {
        assertFalse(SuggestionText.shouldRequest(lineBefore = "fo", typed = "o", charAfter = 'B'))
        assertFalse(SuggestionText.shouldRequest(lineBefore = "fo", typed = "o", charAfter = '_'))
        assertFalse(SuggestionText.shouldRequest(lineBefore = "foo()", typed = ")", charAfter = null))
        assertFalse(SuggestionText.shouldRequest(lineBefore = "x := 1;", typed = ";", charAfter = '\n'))
        assertFalse(SuggestionText.shouldRequest(lineBefore = "    ", typed = "    ", charAfter = null))
        assertFalse(SuggestionText.shouldRequest(lineBefore = "\t\t", typed = "\t", charAfter = '\n'))
    }

    @Test
    fun `a request is made for ordinary typing, at line ends, and after Enter`() {
        assertTrue(SuggestionText.shouldRequest(lineBefore = "a", typed = "a", charAfter = null))
        assertTrue(SuggestionText.shouldRequest(lineBefore = "\treturn ", typed = " ", charAfter = null))
        assertTrue(SuggestionText.shouldRequest(lineBefore = "foo(", typed = "(", charAfter = ')'))
        assertTrue(SuggestionText.shouldRequest(lineBefore = "", typed = "\n", charAfter = '}'))
        assertTrue(SuggestionText.shouldRequest(lineBefore = "func f() {", typed = "{", charAfter = '\n'))
    }

    @Test
    fun `a 429 pause follows Retry-After within sane bounds`() {
        assertEquals(6L, SuggestionText.retryAfterSeconds("6"))
        assertEquals(6L, SuggestionText.retryAfterSeconds(" 6 "))
        assertEquals(SuggestionText.DEFAULT_PAUSE_SECONDS, SuggestionText.retryAfterSeconds(null))
        assertEquals(SuggestionText.DEFAULT_PAUSE_SECONDS, SuggestionText.retryAfterSeconds("soon"))
        assertEquals(SuggestionText.DEFAULT_PAUSE_SECONDS, SuggestionText.retryAfterSeconds("0"))
        assertEquals(SuggestionText.MAX_PAUSE_SECONDS, SuggestionText.retryAfterSeconds("86400"))
    }

    @Test
    fun `only forward typing triggers a request`() {
        assertTrue(SuggestionText.isTypingChange(newFragmentLength = 1, oldFragmentLength = 0))
        // An IDE-inserted bracket pair is still typing.
        assertTrue(SuggestionText.isTypingChange(newFragmentLength = 2, oldFragmentLength = 0))
        // Deleting, replacing a selection, and pasting a file are not.
        assertFalse(SuggestionText.isTypingChange(newFragmentLength = 0, oldFragmentLength = 1))
        assertFalse(SuggestionText.isTypingChange(newFragmentLength = 3, oldFragmentLength = 5))
        assertFalse(SuggestionText.isTypingChange(newFragmentLength = 4000, oldFragmentLength = 0))
    }
}
