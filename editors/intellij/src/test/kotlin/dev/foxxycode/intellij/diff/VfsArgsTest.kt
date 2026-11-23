package dev.foxxycode.intellij.diff

import com.google.gson.JsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test

class VfsArgsTest {
    @Test
    fun `required path cannot silently become the workspace root`() {
        val args = JsonObject().apply { addProperty("path", " ") }
        assertThrows(IllegalArgumentException::class.java) { VfsArgs.required(args, "path", "rm") }
        args.addProperty("path", "src/file.txt")
        assertEquals("src/file.txt", VfsArgs.required(args, "path", "rm"))
    }
}
