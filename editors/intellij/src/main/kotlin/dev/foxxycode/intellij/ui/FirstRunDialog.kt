package dev.foxxycode.intellij.ui

import com.intellij.openapi.project.Project

/**
 * First-run entry point: opens the multi-step welcome wizard and marks onboarding complete.
 * Re-runnable from **Tools → Show FoxxyCode Welcome** ([FoxxyCodeWelcomeAction]).
 */
class FirstRunDialog(private val project: Project) {
    fun showAndConfigure() {
        WelcomeWizardDialog(project, markFirstRunComplete = true).showAndConfigure()
    }
}
