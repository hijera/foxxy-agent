package dev.foxxycode.intellij.hooks

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class HookScopeTest {
    @Test
    fun `accepts the project and its nested worktree`() {
        assertTrue(HookScope.owns("/work/project", "/work/project"))
        assertTrue(HookScope.owns("/work/project", "/work/project/.coddy/worktrees/feature"))
    }

    @Test
    fun `refuses siblings and unknown cwd`() {
        assertFalse(HookScope.owns("/work/project", "/work/project-other"))
        assertFalse(HookScope.owns("/work/project", "/elsewhere"))
        assertFalse(HookScope.owns("/work/project", ""))
        assertFalse(HookScope.owns("", "/work/project"))
    }
}
