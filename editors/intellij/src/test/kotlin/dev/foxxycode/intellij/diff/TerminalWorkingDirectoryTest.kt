package dev.foxxycode.intellij.diff

import com.google.gson.JsonParser
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test
import java.nio.file.Files

class TerminalWorkingDirectoryTest {
    @Test
    fun `command cwd overrides workspace and is validated before execution`() {
        val workspace = Files.createTempDirectory("terminal-cwd")
        try {
            val child = Files.createDirectory(workspace.resolve("ui"))
            fun resolve(json: String) = TerminalWorkingDirectory.resolve(
                JsonParser.parseString(json).asJsonObject, workspace.toString(), null,
            )
            assertEquals(workspace.toString(), resolve("{}"))
            assertEquals(child.toString(), resolve("""{"cwd":"ui"}"""))
            assertEquals(workspace.toString(), resolve("""{"cwd":"ui/.."}"""))
            val absolute = com.google.gson.JsonObject().apply { addProperty("cwd", child.toString()) }
            assertEquals(child.toString(), resolve(absolute.toString()))
            assertThrows(IllegalArgumentException::class.java) { resolve("""{"cwd":"missing"}""") }
            val file = Files.createFile(workspace.resolve("file"))
            assertThrows(IllegalArgumentException::class.java) { resolve("""{"cwd":"file"}""") }
            Files.delete(file)
            Files.delete(child)
        } finally {
            workspace.toFile().deleteRecursively()
        }
    }
}
