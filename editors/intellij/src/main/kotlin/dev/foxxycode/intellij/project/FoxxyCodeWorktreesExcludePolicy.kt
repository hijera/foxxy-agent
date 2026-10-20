package dev.foxxycode.intellij.project

import com.intellij.openapi.project.Project
import com.intellij.openapi.roots.impl.DirectoryIndexExcludePolicy
import com.intellij.openapi.vfs.VfsUtilCore

/**
 * Keeps the git worktrees FoxxyCode creates out of the project index.
 *
 * A branch opened in its own worktree lands in `<main checkout>/.foxxycode/worktrees/<branch>`
 * (`gitws.WorktreesRoot` in the backend), which is inside the project folder. Each entry is a
 * full checkout of another branch, so without this every class, symbol and search result of
 * the project would show up once more per worktree, and indexing would rerun whenever the
 * agent switched a branch. The folder carries its own `.gitignore`, which keeps it out of
 * `git status` but not out of the IDE index.
 */
class FoxxyCodeWorktreesExcludePolicy(private val project: Project) : DirectoryIndexExcludePolicy {

    override fun getExcludeUrlsForProject(): Array<String> {
        val base = project.basePath ?: return emptyArray()
        return arrayOf(VfsUtilCore.pathToUrl(worktreesPath(base)))
    }
}

/** The worktrees folder of the checkout at [basePath], in IntelliJ's forward-slash form. */
internal fun worktreesPath(basePath: String): String =
    basePath.replace('\\', '/').trimEnd('/') + "/.foxxycode/worktrees"
