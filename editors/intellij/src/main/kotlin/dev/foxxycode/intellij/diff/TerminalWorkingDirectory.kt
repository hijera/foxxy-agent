package dev.foxxycode.intellij.diff

import com.google.gson.JsonObject
import java.nio.file.Files
import java.nio.file.Path

/** Resolves the per-command cwd with the same rules as the native shell tool. */
object TerminalWorkingDirectory {
    fun resolve(args: JsonObject, workspace: String, projectPath: String?): String {
        val base = Path.of(workspace.ifBlank { projectPath ?: "." })
        val requested = args.get("cwd")?.takeUnless { it.isJsonNull }?.asString.orEmpty()
        val resolved = if (requested.isEmpty()) base else base.resolve(requested).normalize()
        require(Files.isDirectory(resolved)) { "command cwd is not a directory: $resolved" }
        return resolved.toString()
    }
}
