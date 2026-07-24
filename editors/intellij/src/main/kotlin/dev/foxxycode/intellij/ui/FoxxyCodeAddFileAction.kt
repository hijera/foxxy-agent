package dev.foxxycode.intellij.ui

import com.intellij.icons.AllIcons
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.actionSystem.CommonDataKeys
import com.intellij.openapi.project.Project
import com.intellij.openapi.vfs.VfsUtilCore
import com.intellij.openapi.vfs.VirtualFile
import com.intellij.openapi.wm.ToolWindowManager
import dev.foxxycode.intellij.FoxxyCodeBundle

/**
 * "Add to FoxxyCode": inserts the selected file(s) into the composer as short `@`-mentions.
 *
 * This is the reliable way to reference an open file, offered from the editor tab context menu,
 * the editor context menu, and the Project view — dragging onto the tool window does not work
 * because the composer lives in a heavyweight JCEF surface that swallows IntelliJ's intra-IDE
 * drag tracking. The action activates the FoxxyCode tool window (creating the panel if needed)
 * and hands the project-relative paths to [FoxxyCodeBrowserPanel.requestInsertFileMentions],
 * which queues them if the web page has not finished loading yet.
 */
class FoxxyCodeAddFileAction : AnAction(
    FoxxyCodeBundle.message("action.addFile.text"),
    FoxxyCodeBundle.message("action.addFile.description"),
    AllIcons.General.Add,
) {
    override fun actionPerformed(e: AnActionEvent) {
        val project = e.project ?: return
        val rels = relativePaths(project, selectedFiles(e))
        if (rels.isEmpty()) return

        val toolWindow = ToolWindowManager.getInstance(project).getToolWindow("FoxxyCode")
        if (toolWindow == null) {
            // No tool window (unexpected): fall back to any live panel for the project.
            FoxxyCodeBrowserPanel.forProject(project)?.requestInsertFileMentions(rels)
            return
        }
        // activate() creates the tool window content (and thus the panel) if needed, then runs
        // the callback on the EDT once it is shown.
        toolWindow.activate({
            FoxxyCodeBrowserPanel.forProject(project)?.requestInsertFileMentions(rels)
        }, true, true)
    }

    override fun update(e: AnActionEvent) {
        val project = e.project
        e.presentation.isEnabledAndVisible =
            project != null && relativePaths(project, selectedFiles(e)).isNotEmpty()
    }

    private fun selectedFiles(e: AnActionEvent): List<VirtualFile> {
        e.getData(CommonDataKeys.VIRTUAL_FILE_ARRAY)?.let { return it.toList() }
        return e.getData(CommonDataKeys.VIRTUAL_FILE)?.let { listOf(it) } ?: emptyList()
    }

    private fun relativePaths(project: Project, files: List<VirtualFile>): List<String> {
        val ioFiles = files
            .filter { it.isInLocalFileSystem && !it.isDirectory }
            .map { VfsUtilCore.virtualToIoFile(it) }
        return ProjectRelativePaths.relativize(project.basePath, ioFiles)
    }
}
