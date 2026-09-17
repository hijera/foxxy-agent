package dev.foxxycode.intellij.project

import org.junit.Assert.assertEquals
import org.junit.Test

class FoxxyCodeWorktreesExcludePolicyTest {

    @Test
    fun `the worktrees folder sits under the project's own foxxycode folder`() {
        assertEquals("/home/u/project/.foxxycode/worktrees", worktreesPath("/home/u/project"))
    }

    // IntelliJ hands a Windows project path with forward slashes, but a path built elsewhere
    // may still carry backslashes or a trailing separator; either way the URL names one folder.
    @Test
    fun `a trailing separator or a Windows path gives the same folder`() {
        assertEquals("/home/u/project/.foxxycode/worktrees", worktreesPath("/home/u/project/"))
        assertEquals("C:/work/project/.foxxycode/worktrees", worktreesPath("C:\\work\\project\\"))
    }
}
