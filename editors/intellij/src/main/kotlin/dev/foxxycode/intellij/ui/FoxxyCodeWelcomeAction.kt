package dev.foxxycode.intellij.ui

import com.intellij.icons.AllIcons
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import dev.foxxycode.intellij.FoxxyCodeBundle

/** Re-opens the FoxxyCode onboarding wizard without changing the first-run flag. */
class FoxxyCodeWelcomeAction : AnAction(
    FoxxyCodeBundle.message("action.welcome.text"),
    FoxxyCodeBundle.message("action.welcome.description"),
    AllIcons.General.ContextHelp,
) {
    override fun actionPerformed(e: AnActionEvent) {
        val project = e.project ?: return
        WelcomeWizardDialog(project, markFirstRunComplete = false).showAndGet()
    }

    override fun update(e: AnActionEvent) {
        e.presentation.isEnabledAndVisible = e.project != null
    }
}
