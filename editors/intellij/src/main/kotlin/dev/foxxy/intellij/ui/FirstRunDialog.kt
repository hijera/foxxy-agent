package dev.foxxy.intellij.ui

import com.intellij.openapi.project.Project
import com.intellij.openapi.ui.DialogWrapper
import com.intellij.ui.components.JBLabel
import com.intellij.util.ui.FormBuilder
import dev.foxxy.intellij.FoxxyBundle
import dev.foxxy.intellij.settings.FoxxySettings
import javax.swing.JComponent

/**
 * First-run wizard: informs the user that the foxxy binary is bundled with the plugin
 * and lets them start immediately. Re-runnable from Settings | Tools | Foxxy.
 */
class FirstRunDialog(project: Project) : DialogWrapper(project) {
    init {
        title = FoxxyBundle.message("firstrun.title")
        init()
    }

    override fun createCenterPanel(): JComponent =
        FormBuilder.createFormBuilder()
            .addComponent(
                JBLabel(FoxxyBundle.message("firstrun.body"))
            )
            .panel

    /** Shows the dialog (modal) and marks the first-run wizard as completed. */
    fun showAndConfigure() {
        showAndGet()
        FoxxySettings.getInstance().state.firstRunCompleted = true
    }
}
