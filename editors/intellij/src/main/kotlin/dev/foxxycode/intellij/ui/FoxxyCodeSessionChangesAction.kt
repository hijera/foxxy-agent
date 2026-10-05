package dev.foxxycode.intellij.ui

import com.intellij.icons.AllIcons
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.project.DumbAware
import dev.foxxycode.intellij.FoxxyCodeBundle
import dev.foxxycode.intellij.diff.FoxxyCodeSessionChangesService

/**
 * Opens the IDE diff viewer on everything the current FoxxyCode session changed.
 *
 * It sits in the tool window title bar so the review is one click away from the
 * chat, without having to scroll the transcript down to the changed-files card.
 */
class FoxxyCodeSessionChangesAction : AnAction(
    FoxxyCodeBundle.message("action.changes.text"),
    FoxxyCodeBundle.message("action.changes.description"),
    AllIcons.Actions.Diff,
), DumbAware {
    override fun actionPerformed(e: AnActionEvent) {
        val project = e.project ?: return
        FoxxyCodeSessionChangesService.getInstance(project).showChanges()
    }

    override fun update(e: AnActionEvent) {
        e.presentation.isEnabledAndVisible = e.project != null
    }
}
