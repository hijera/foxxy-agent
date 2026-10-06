package dev.foxxycode.intellij.ui

import com.intellij.openapi.Disposable
import com.intellij.openapi.diagnostic.logger
import com.intellij.openapi.project.DumbAware
import com.intellij.openapi.project.Project
import com.intellij.openapi.wm.ToolWindow
import com.intellij.openapi.wm.ToolWindowFactory
import com.intellij.ui.components.JBLabel
import com.intellij.util.ui.JBUI
import dev.foxxycode.intellij.FoxxyCodeBundle
import dev.foxxycode.intellij.settings.FoxxyCodeSettings
import java.awt.BorderLayout
import javax.swing.JComponent
import javax.swing.JPanel
import javax.swing.SwingConstants

class FoxxyCodeToolWindowFactory : ToolWindowFactory, DumbAware {
    override fun createToolWindowContent(project: Project, toolWindow: ToolWindow) {
        // FoxxyCodeBrowserPanel has JCEF types in its signatures: loading it without JCEF throws
        // NoClassDefFoundError and leaves the tool window empty. Check first, and still catch the
        // linkage error in case the classes resolve here but something they need does not.
        val content: JComponent = if (!JcefSupport.isAvailable()) {
            LOG.warn("JCEF classes are not visible to the FoxxyCode plugin; showing the JCEF notice instead of the panel")
            jcefMissingPanel(null)
        } else {
            try {
                if (!FoxxyCodeSettings.getInstance().state.firstRunCompleted) {
                    FirstRunDialog(project).showAndConfigure()
                }
                FoxxyCodeBrowserPanel(project)
            } catch (e: LinkageError) {
                LOG.warn("FoxxyCode browser panel could not be loaded", e)
                jcefMissingPanel(e)
            }
        }
        // contentManager.factory works across 2021.2..latest (avoids the ContentFactory API split).
        val tab = toolWindow.contentManager.factory.createContent(content, "", false)
        // Cast to the platform interface, not to FoxxyCodeBrowserPanel: naming that class here would
        // load it on the JCEF-less path too.
        (content as? Disposable)?.let { tab.setDisposer(it) }
        toolWindow.contentManager.addContent(tab)
    }

    private fun jcefMissingPanel(error: Throwable?): JComponent {
        val detail = error?.let { escapeHtml(it.toString()) }.orEmpty()
        val label = JBLabel(FoxxyCodeBundle.message("toolwindow.jcefMissing", detail), SwingConstants.CENTER)
        label.setAllowAutoWrapping(true)
        label.border = JBUI.Borders.empty(12)
        return JPanel(BorderLayout()).apply { add(label, BorderLayout.CENTER) }
    }

    private fun escapeHtml(text: String): String =
        text.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")

    private companion object {
        private val LOG = logger<FoxxyCodeToolWindowFactory>()
    }
}
