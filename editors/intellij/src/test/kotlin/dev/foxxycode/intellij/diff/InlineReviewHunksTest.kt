package dev.foxxycode.intellij.diff

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class InlineReviewHunksTest {
    @Test
    fun `rejecting one block preserves the other and unrelated edits`() {
        val after = "new one\nkeep\nnew two\n"
        val (first, second) = reviewHunks("old one\nkeep\nold two\n", after)
        assertEquals("old one\nkeep\nnew two\n", after.replaceRange(first.start, first.end, first.before))
        val prefix = "// mine\n"
        val moved = second.move(0, 0, prefix.length)!!
        assertEquals(second.start + prefix.length, moved.start)
        assertTrue(moved.matches(prefix + after))
        assertFalse(first.matches("NEW one\nkeep\nnew two\n"))
        assertEquals(null, first.move(1, 3, 1))
    }

    @Test
    fun `deletions and a final newline have reviewable offsets`() {
        val deletion = reviewHunks("a\nb\n", "a\n").single()
        assertEquals(2, deletion.start)
        assertEquals(2, deletion.end)
        assertEquals("b\n", deletion.before)
        assertEquals(null, deletion.move(2, 2, 4))
        assertEquals("a\n", reviewHunks("a", "a\n").single().after)
    }
}
