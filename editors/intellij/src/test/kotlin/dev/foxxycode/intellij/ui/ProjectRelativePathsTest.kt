package dev.foxxycode.intellij.ui

import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File

/**
 * Unit tests for [ProjectRelativePaths.relativize] — the pure half of the panel's
 * drag-and-drop handling (Project view items and editor tabs both end up here).
 * Platform-free, so it runs in the plain JUnit4 `test` task.
 */
class ProjectRelativePathsTest {

    @get:Rule
    val tmp = TemporaryFolder()

    private fun file(vararg segments: String): File {
        val f = File(tmp.root, segments.joinToString(File.separator))
        f.parentFile.mkdirs()
        f.writeText("x")
        return f
    }

    @Test
    fun `relativizes files under the project root with posix separators`() {
        val a = file("src", "main", "App.kt")
        val b = file("README.md")
        assertEquals(
            listOf("src/main/App.kt", "README.md"),
            ProjectRelativePaths.relativize(tmp.root.path, listOf(a, b)),
        )
    }

    @Test
    fun `drops directories`() {
        val dir = File(tmp.root, "src").also { it.mkdirs() }
        val f = file("src", "App.kt")
        assertEquals(
            listOf("src/App.kt"),
            ProjectRelativePaths.relativize(tmp.root.path, listOf(dir, f)),
        )
    }

    @Test
    fun `drops files outside the project root`() {
        val outside = File(tmp.newFolder("outside"), "Other.kt").also { it.writeText("x") }
        val inside = file("App.kt")
        val root = File(tmp.root, "project").also { it.mkdirs() }
        val insideProject = File(root, "In.kt").also { it.writeText("x") }
        assertEquals(
            listOf("In.kt"),
            ProjectRelativePaths.relativize(root.path, listOf(outside, inside, insideProject)),
        )
    }

    @Test
    fun `de-duplicates repeated files while keeping input order`() {
        val a = file("a.kt")
        val b = file("b.kt")
        assertEquals(
            listOf("a.kt", "b.kt"),
            ProjectRelativePaths.relativize(tmp.root.path, listOf(a, b, a)),
        )
    }

    @Test
    fun `an unknown project root yields nothing`() {
        val a = file("a.kt")
        assertEquals(emptyList<String>(), ProjectRelativePaths.relativize(null, listOf(a)))
        assertEquals(emptyList<String>(), ProjectRelativePaths.relativize("  ", listOf(a)))
    }

    @Test
    fun `an empty drag yields nothing`() {
        assertEquals(emptyList<String>(), ProjectRelativePaths.relativize(tmp.root.path, emptyList()))
    }
}
