package dev.foxxycode.intellij.ui

import com.intellij.openapi.project.DumbAware
import com.intellij.openapi.project.Project
import com.intellij.openapi.wm.ToolWindow
import com.intellij.openapi.wm.ToolWindowFactory
import dev.foxxycode.intellij.settings.FoxxyCodeSettings

class FoxxyCodeToolWindowFactory : ToolWindowFactory, DumbAware {
    override fun createToolWindowContent(project: Project, toolWindow: ToolWindow) {
        if (!FoxxyCodeSettings.getInstance().state.firstRunCompleted) {
            FirstRunDialog(project).showAndConfigure()
        }
        val panel = FoxxyCodeBrowserPanel(project)
        // contentManager.factory works across 2021.2..latest (avoids the ContentFactory API split).
        val content = toolWindow.contentManager.factory.createContent(panel, "", false)
        content.setDisposer(panel)
        toolWindow.contentManager.addContent(content)
    }
}
