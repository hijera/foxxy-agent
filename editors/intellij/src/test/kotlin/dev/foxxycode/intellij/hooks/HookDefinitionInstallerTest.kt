package dev.foxxycode.intellij.hooks

import com.google.gson.JsonParser
import java.io.File
import java.nio.file.Files
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class HookDefinitionInstallerTest {
    @Test
    fun `install preserves user hooks and is idempotent`() {
        val home = createTempDir()
        File(home, "hooks.json").writeText("""{"hooks":{"PreToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"mine"}]}]},"other":true}""")
        install(home, "one")
        install(home, "one")
        val root = JsonParser.parseString(File(home, "hooks.json").readText()).asJsonObject
        assertTrue(root.get("other").asBoolean)
        assertEquals(1, root.getAsJsonObject("hooks").getAsJsonArray("PreToolUse").size())
        assertEquals(1, root.getAsJsonObject("hooks").getAsJsonArray("ToolExecute").size())
        assertEquals(1, root.getAsJsonObject("hooks").getAsJsonArray("FirstPrompt").size())
    }

    @Test
    fun `invalid user json is left untouched`() {
        val home = createTempDir()
        val file = File(home, "hooks.json")
        file.writeText("{broken")
        org.junit.Assert.assertThrows(Exception::class.java) { install(home, "one") }
        assertEquals("{broken", file.readText())
    }

    @Test
    fun `remove deletes only the calling IDE instance`() {
        val home = createTempDir()
        install(home, "one")
        install(home, "two")
        HookDefinitionInstaller.remove(home, "one")
        val text = File(home, "hooks.json").readText()
        assertFalse(text.contains("--foxxycode-hook=one"))
        assertTrue(text, text.contains("token-two"))
        HookDefinitionInstaller.remove(home, "two")
        assertFalse(File(home, "hooks.json").readText().contains("ToolExecute"))
    }

    private fun install(home: File, id: String) =
        HookDefinitionInstaller.install(home, id, File("java"), File("plugin.jar"), "http://127.0.0.1:1/$id", "token-$id")

    private fun createTempDir(): File = Files.createTempDirectory("foxxycode-hooks-test").toFile()
}
