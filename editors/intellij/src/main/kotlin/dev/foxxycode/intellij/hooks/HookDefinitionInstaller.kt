package dev.foxxycode.intellij.hooks

import com.google.gson.GsonBuilder
import com.google.gson.JsonArray
import com.google.gson.JsonObject
import com.google.gson.JsonParser
import java.io.File
import java.io.RandomAccessFile
import java.nio.charset.StandardCharsets
import java.nio.file.Files
import java.nio.file.StandardCopyOption
import java.nio.file.attribute.PosixFilePermission

/** Adds and removes only this IDE instance's handlers in Coddy's user-scope hooks.json. */
object HookDefinitionInstaller {
    private const val MARKER = "--foxxycode-hook="
    private val gson = GsonBuilder().setPrettyPrinting().create()

    fun install(home: File, id: String, java: File, classPath: File, url: String, token: String) {
        withLock(home) { file ->
            val root = read(file)
            removeManaged(root, id)
            val hooks = objectAt(root, "hooks")
            addExecutor(hooks, "ToolExecute", "write|edit|apply_patch|run_command|read|mkdir|rm|mv|touch", java, classPath, url, token, id)
            addExecutor(hooks, "FirstPrompt", "*", java, classPath, url, token, id)
            write(file, root)
        }
    }

    fun remove(home: File, id: String) {
        val file = File(home, "hooks.json")
        if (!file.isFile) return
        withLock(home) {
            if (!file.isFile) return@withLock
            val root = read(file)
            removeManaged(root, id)
            write(file, root)
        }
    }

    private fun addExecutor(
        hooks: JsonObject,
        event: String,
        matcher: String,
        java: File,
        classPath: File,
        url: String,
        token: String,
        id: String,
    ) {
        val handler = JsonObject().apply {
            addProperty("type", "command")
            addProperty("command", java.absolutePath)
            add("args", JsonArray().apply {
                add("-cp")
                add(classPath.absolutePath)
                add(FoxxyCodeHookProcess::class.java.name)
                add(url)
                add(token)
                add(MARKER + id)
            })
        }
        val group = JsonObject().apply {
            addProperty("matcher", matcher)
            add("hooks", JsonArray().apply { add(handler) })
        }
        val groups = hooks.getAsJsonArray(event) ?: JsonArray().also { hooks.add(event, it) }
        groups.add(group)
    }

    private fun removeManaged(root: JsonObject, id: String) {
        val hooks = root.getAsJsonObject("hooks") ?: return
        val marker = MARKER + id
        for (event in hooks.entrySet().toList()) {
            val groups = event.value?.takeIf { it.isJsonArray }?.asJsonArray ?: continue
            for (group in groups.toList()) {
                if (!group.isJsonObject) continue
                val handlers = group.asJsonObject.getAsJsonArray("hooks") ?: continue
                val managed = handlers.filter { handler ->
                    handler.isJsonObject && handler.asJsonObject.getAsJsonArray("args")?.any {
                        it.isJsonPrimitive && it.asString == marker
                    } == true
                }
                managed.forEach { handlers.remove(it) }
                if (handlers.size() == 0) groups.remove(group)
            }
            if (groups.size() == 0) hooks.remove(event.key)
        }
        if (hooks.size() == 0) root.remove("hooks")
    }

    private fun objectAt(root: JsonObject, key: String): JsonObject {
        val existing = root.get(key)
        if (existing == null || existing.isJsonNull) return JsonObject().also { root.add(key, it) }
        if (!existing.isJsonObject) throw IllegalArgumentException("$key in ${FoxxyCodeHookService.HOOKS_FILE} must be an object")
        return existing.asJsonObject
    }

    private fun read(file: File): JsonObject {
        if (!file.exists()) return JsonObject()
        val parsed = JsonParser.parseString(file.readText(StandardCharsets.UTF_8))
        if (!parsed.isJsonObject) throw IllegalArgumentException("${file.name} must contain a JSON object")
        return parsed.asJsonObject
    }

    private fun write(file: File, root: JsonObject) {
        file.parentFile.mkdirs()
        val temp = Files.createTempFile(file.parentFile.toPath(), ".hooks-", ".tmp")
        try {
            Files.writeString(temp, gson.toJson(root) + "\n", StandardCharsets.UTF_8)
            try {
                Files.setPosixFilePermissions(temp, setOf(PosixFilePermission.OWNER_READ, PosixFilePermission.OWNER_WRITE))
            } catch (_: UnsupportedOperationException) {
            }
            try {
                Files.move(temp, file.toPath(), StandardCopyOption.ATOMIC_MOVE, StandardCopyOption.REPLACE_EXISTING)
            } catch (_: java.nio.file.AtomicMoveNotSupportedException) {
                Files.move(temp, file.toPath(), StandardCopyOption.REPLACE_EXISTING)
            }
        } finally {
            Files.deleteIfExists(temp)
        }
    }

    private fun withLock(home: File, block: (File) -> Unit) {
        home.mkdirs()
        RandomAccessFile(File(home, ".foxxycode-hooks.lock"), "rw").use { lockFile ->
            lockFile.channel.use { channel ->
                channel.lock().use {
                    block(File(home, FoxxyCodeHookService.HOOKS_FILE))
                }
            }
        }
    }
}
