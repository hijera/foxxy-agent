package dev.foxxycode.intellij.ui

import java.io.File
import java.nio.file.Path
import java.nio.file.Paths

/**
 * Turns files and directories carried by a drag-and-drop onto the FoxxyCode panel into short,
 * project-relative POSIX paths for the composer `@`-mention.
 *
 * Kept free of IntelliJ APIs so it is unit-testable without the platform
 * (see ProjectRelativePathsTest); [FoxxyCodeBrowserPanel] owns the DnD plumbing.
 */
object ProjectRelativePaths {

    /**
     * Project-relative POSIX paths for [files], in input order and de-duplicated.
     * Directories keep a trailing slash so the mention resolver can distinguish them
     * from files. Paths outside [basePath] and anything that cannot be relativized
     * are dropped, so an unrelated drag simply yields nothing.
     */
    fun relativize(basePath: String?, files: List<File>): List<String> {
        val base = basePath?.trim().orEmpty()
        if (base.isEmpty() || files.isEmpty()) return emptyList()
        val root: Path = try {
            Paths.get(base)
        } catch (e: Exception) {
            return emptyList()
        }
        val out = LinkedHashSet<String>()
        for (f in files) {
            val rel = try {
                root.relativize(f.toPath()).toString().replace('\\', '/')
            } catch (e: Exception) {
                continue
            }
            if (rel.isEmpty() || rel.startsWith("..")) continue
            out.add(if (f.isDirectory) "$rel/" else rel)
        }
        return out.toList()
    }
}
