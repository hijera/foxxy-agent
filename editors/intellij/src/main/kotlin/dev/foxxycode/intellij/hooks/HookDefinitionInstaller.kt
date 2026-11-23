package dev.foxxycode.intellij.hooks

import com.google.gson.GsonBuilder
import com.google.gson.JsonArray
import com.google.gson.JsonElement
import com.google.gson.JsonObject
import com.google.gson.JsonParser
import java.io.File
import java.io.RandomAccessFile
import java.net.HttpURLConnection
import java.net.URI
import java.nio.charset.StandardCharsets
import java.nio.file.Files
import java.nio.file.StandardCopyOption
import java.nio.file.attribute.PosixFilePermission

/** Adds and removes only this IDE instance's handlers in Coddy's user-scope hooks.json. */
object HookDefinitionInstaller {
    private const val MARKER = "--foxxycode-hook="
    private val gson = GsonBuilder().setPrettyPrinting().create()

    fun install(
        home: File,
        id: String,
        java: File,
        classPath: File,
        url: String,
        token: String,
        isAlive: (String) -> Boolean = ::alive,
    ) {
        withLock(home) { file ->
            val root = read(file)
            // This instance's own stale group goes first, then the groups of
            // instances that died without a dispose (a crashed IDE): a hook
            // process pointed at a dead port would spawn and fail on every
            // matching tool call. Groups of other live instances answer their
            // probe and survive.
            removeManaged(root, id)
            pruneStaleInstances(root, isAlive)
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
        removeManagedMatching(root) { markerId, _ -> markerId == id }
    }

    /**
     * Drops the managed handlers of instances that are not serving their endpoint
     * any more. Runs under the same lock as the install, once per panel start.
     */
    private fun pruneStaleInstances(root: JsonObject, isAlive: (String) -> Boolean) {
        removeManagedMatching(root) { _, url -> url.isBlank() || !isAlive(url) }
    }

    /**
     * Removes managed handlers the [drop] predicate calls for. A handler is
     * managed when its args carry the `--foxxycode-hook=<id>` marker; its
     * endpoint url sits two args before the marker (`-cp, classpath, class,
     * url, token, marker`). Nothing without the marker is ever touched.
     */
    private fun removeManagedMatching(root: JsonObject, drop: (markerId: String, url: String) -> Boolean) {
        val hooks = root.getAsJsonObject("hooks") ?: return
        for (event in hooks.entrySet().toList()) {
            val groups = event.value?.takeIf { it.isJsonArray }?.asJsonArray ?: continue
            for (group in groups.toList()) {
                if (!group.isJsonObject) continue
                val handlers = group.asJsonObject.getAsJsonArray("hooks") ?: continue
                val managed = handlers.filter { handler ->
                    handler.isJsonObject && handler.dropPredicate(drop)
                }
                managed.forEach { handlers.remove(it) }
                if (handlers.size() == 0) groups.remove(group)
            }
            if (groups.size() == 0) hooks.remove(event.key)
        }
        if (hooks.size() == 0) root.remove("hooks")
    }

    private fun JsonElement.dropPredicate(drop: (markerId: String, url: String) -> Boolean): Boolean {
        val args = asJsonObject.getAsJsonArray("args") ?: return false
        val markerIdx = args.indexOfFirst { it.isJsonPrimitive && it.asString.startsWith(MARKER) }
        if (markerIdx < 2) return false
        val markerId = args[markerIdx].asString.removePrefix(MARKER)
        val url = args.get(markerIdx - 2)?.takeIf { it.isJsonPrimitive }?.asString.orEmpty()
        return drop(markerId, url)
    }

    /** Any HTTP answer - even a 401 for the probe's missing token - means the instance serves. */
    private fun alive(url: String): Boolean = try {
        val conn = URI.create(url).toURL().openConnection() as HttpURLConnection
        conn.connectTimeout = 500
        conn.readTimeout = 500
        conn.requestMethod = "GET"
        conn.responseCode > 0
    } catch (_: Exception) {
        false
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
