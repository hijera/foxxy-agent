package dev.foxxy.intellij.ui

import com.intellij.openapi.project.Project
import com.intellij.openapi.ui.DialogWrapper
import com.intellij.ui.components.JBLabel
import com.intellij.util.ui.FormBuilder
import dev.foxxy.intellij.settings.FoxxySettings
import javax.swing.JComponent

/**
 * First-run wizard: informs the user that the foxxy binary is bundled with the plugin
 * and lets them start immediately. Re-runnable from Settings | Tools | Foxxy.
 */
class FirstRunDialog(project: Project) : DialogWrapper(project) {
    init {
        title = "Set Up Foxxy"
        init()
    }

    override fun createCenterPanel(): JComponent =
        FormBuilder.createFormBuilder()
            .addComponent(
                JBLabel(
                    "<html>The foxxy agent binary is bundled with the Foxxy plugin and ready to use.<br/>" +
                        "Just close this dialog and the Foxxy tool window will start it automatically.<br/><br/>" +
                        "Optional: configure host, port, Foxxy home, or override the binary path in<br/>" +
                        "Settings | Tools | Foxxy.</html>"
                )
            )
            .panel

    /** Shows the dialog (modal) and marks the first-run wizard as completed. */
    fun showAndConfigure() {
        showAndGet()
        FoxxySettings.getInstance().state.firstRunCompleted = true
    }
}
