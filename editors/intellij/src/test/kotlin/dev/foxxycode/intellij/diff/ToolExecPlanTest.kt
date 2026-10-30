package dev.foxxycode.intellij.diff

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Unit tests for the delegated-write decision. Platform-free, so it runs in the plain
 * JUnit4 `test` task.
 */
class ToolExecPlanTest {

    @Test
    fun `clean document matching before applies after`() {
        val d = ToolExecPlan.decide("p/a.txt", "old", "new", "old", documentDirty = false)
        assertEquals(ToolExecPlan.Action.APPLY, d.action)
    }

    @Test
    fun `dirty document is a conflict`() {
        val d = ToolExecPlan.decide("p/a.txt", "old", "new", "old", documentDirty = true)
        assertEquals(ToolExecPlan.Action.CONFLICT, d.action)
        assertTrue(d.message.contains("unsaved"))
    }

    @Test
    fun `buffer that no longer matches before is a conflict`() {
        val d = ToolExecPlan.decide("p/a.txt", "old", "new", "something else", documentDirty = false)
        assertEquals(ToolExecPlan.Action.CONFLICT, d.action)
        assertTrue(d.message.contains("re-read"))
    }

    @Test
    fun `line endings are not a difference`() {
        val d = ToolExecPlan.decide("p/a.txt", "old\r\nline\r\n", "new", "old\nline\n", documentDirty = false)
        assertEquals(ToolExecPlan.Action.APPLY, d.action)
    }

    @Test
    fun `missing file with empty before creates it`() {
        val d = ToolExecPlan.decide("p/new.txt", "", "body", null, documentDirty = false)
        assertEquals(ToolExecPlan.Action.CREATE, d.action)
    }

    @Test
    fun `missing file with content in before is a conflict`() {
        val d = ToolExecPlan.decide("p/a.txt", "old", "new", null, documentDirty = false)
        assertEquals(ToolExecPlan.Action.CONFLICT, d.action)
    }

    @Test
    fun `blank path is not handled`() {
        val d = ToolExecPlan.decide("", "old", "new", "old", documentDirty = false)
        assertEquals(ToolExecPlan.Action.NOT_HANDLED, d.action)
    }
}
