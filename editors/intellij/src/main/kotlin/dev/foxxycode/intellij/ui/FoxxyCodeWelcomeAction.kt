package dev.foxxycode.intellij.ui

import com.intellij.icons.AllIcons
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.ui.Messages
import dev.foxxycode.intellij.FoxxyCodeBundle

/** Re-opens the FoxxyCode onboarding wizard without changing the first-run flag. */
class FoxxyCodeWelcomeAction : AnAction(
    FoxxyCodeBundle.message("action.welcome.text"),
    FoxxyCodeBundle.message("action.welcome.description"),
    AllIcons.General.ContextHelp,
) {
    override fun actionPerformed(e: AnActionEvent) {
        val project = e.project ?: return
        // The wizard is built on JCEF; where the plugin cannot load it (IDE 2026.2+ with the
        // Web Browser (JCEF) plugin disabled) show the same text the wizard falls back to.
        try {
            WelcomeWizardDialog(project, markFirstRunComplete = false).showAndGet()
        } catch (_: LinkageError) {
            Messages.showInfoMessage(
                project,
                FoxxyCodeBundle.message("welcome.fallback.body"),
                FoxxyCodeBundle.message("welcome.title"),
            )
        }
    }

    override fun update(e: AnActionEvent) {
        e.presentation.isEnabledAndVisible = e.project != null
    }
}
