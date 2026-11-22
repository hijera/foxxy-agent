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

    @Test
    fun `install prunes the handlers of instances that died without a dispose`() {
        val home = createTempDir()
        install(home, "one")
        // A second window (or a restarted IDE) installs; "one" no longer serves.
        install(home, "two") { url -> !url.endsWith("/one") }
        val text = File(home, "hooks.json").readText()
        // The marker text is escaped by the pretty printer ("=" is \u003d), so the
        // assertions key on the endpoint url, which is what the prune probes too.
        assertFalse(text.contains(":1/one"))
        assertTrue(text, text.contains(":1/two"))
    }

    @Test
    fun `install keeps the handlers of a live sibling instance`() {
        val home = createTempDir()
        install(home, "one")
        install(home, "two") { _ -> true }
        val text = File(home, "hooks.json").readText()
        assertTrue(text, text.contains(":1/one"))
        assertTrue(text, text.contains(":1/two"))
    }

    private fun install(home: File, id: String, isAlive: (String) -> Boolean = { true }) =
        HookDefinitionInstaller.install(home, id, File("java"), File("plugin.jar"), "http://127.0.0.1:1/$id", "token-$id", isAlive)

    private fun createTempDir(): File = Files.createTempDirectory("foxxycode-hooks-test").toFile()
}
