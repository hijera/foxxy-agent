package dev.foxxycode.intellij.hooks

import java.nio.file.Path

/** Limits a user-scope hook to sessions owned by this IDE project. */
object HookScope {
    fun owns(projectBase: String?, cwd: String?): Boolean {
        if (projectBase.isNullOrBlank() || cwd.isNullOrBlank()) return false
        return try {
            val base = Path.of(projectBase).toAbsolutePath().normalize()
            val session = Path.of(cwd).toAbsolutePath().normalize()
            session.startsWith(base)
        } catch (_: Exception) {
            false
        }
    }
}
