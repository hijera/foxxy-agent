package dev.foxxy.intellij.settings

import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.fileChooser.FileChooserDescriptorFactory
import com.intellij.openapi.options.Configurable
import com.intellij.openapi.progress.ProgressManager
import com.intellij.openapi.ui.TextFieldWithBrowseButton
import com.intellij.ui.components.JBCheckBox
import com.intellij.ui.components.JBLabel
import com.intellij.ui.components.JBTextField
import com.intellij.util.ui.FormBuilder
import dev.foxxy.intellij.binary.FoxxyBinaryResolver
import java.io.File
import javax.swing.JButton
import javax.swing.JComponent
import javax.swing.JPanel

/**
 * Settings | Tools | Foxxy — configure the `foxxy http` launch parameters and an optional
 * binary path override. The foxxy binary is bundled with the plugin; leave "Binary path"
 * empty to use the bundled one.
 */
class FoxxyConfigurable : Configurable {
    private val pathField = TextFieldWithBrowseButton()
    private val hostField = JBTextField()
    private val portField = JBTextField()
    private val homeField = TextFieldWithBrowseButton()
    private val extraArgsField = JBTextField()
    private val followThemeCheckBox = JBCheckBox("Match Foxxy UI theme to the IDE theme")
    private val nativeDiffsCheckBox = JBCheckBox("Show native inline diffs in the editor when the agent edits files")
    private val autoApproveCheckBox = JBCheckBox("Auto-apply edits without asking (still shows the diff, with Revert)")
    private val statusLabel = JBLabel(" ")

    private val settings get() = FoxxySettings.getInstance().state

    override fun getDisplayName(): String = "Foxxy"

    override fun createComponent(): JComponent {
        pathField.addBrowseFolderListener(
            "Foxxy Binary (optional)", "Override the bundled foxxy executable", null,
            FileChooserDescriptorFactory.createSingleFileDescriptor()
        )
        homeField.addBrowseFolderListener(
            "Foxxy Home", "State directory for sessions/config (default ~/.coddy)", null,
            FileChooserDescriptorFactory.createSingleFolderDescriptor()
        )

        val actions = JPanel().apply {
            add(JButton("Verify binary").apply { addActionListener { verify() } })
        }

        val panel = FormBuilder.createFormBuilder()
            .addLabeledComponent(
                "Binary path (optional):",
                pathField
            )
            .addComponent(
                JBLabel("Leave empty to use the foxxy binary bundled with the plugin.")
            )
            .addComponent(actions)
            .addComponent(statusLabel)
            .addSeparator()
            .addLabeledComponent("Host:", hostField)
            .addLabeledComponent("Port (0 = auto):", portField)
            .addLabeledComponent("Foxxy home (optional):", homeField)
            .addLabeledComponent("Extra args:", extraArgsField)
            .addSeparator()
            .addComponent(followThemeCheckBox)
            .addSeparator()
            .addComponent(nativeDiffsCheckBox)
            .addComponent(autoApproveCheckBox)
            .addComponentFillVertically(JPanel(), 0)
            .panel
        reset()
        return panel
    }

    private fun currentBinary(): File? {
        val p = pathField.text.trim()
        return if (p.isBlank()) FoxxyBinaryResolver.resolveExisting() else File(p).takeIf { it.isFile }
    }

    private fun verify() {
        val bin = currentBinary()
        if (bin == null) {
            statusLabel.text = "No binary available. Install the plugin or set a valid path."
            return
        }
        ProgressManager.getInstance().runProcessWithProgressSynchronously({
            val v = FoxxyBinaryResolver.validate(bin)
            ApplicationManager.getApplication().invokeLater { statusLabel.text = v.message }
        }, "Verifying Foxxy Binary", true, null)
    }

    override fun isModified(): Boolean {
        val s = settings
        return pathField.text.trim() != s.binaryPath ||
            hostField.text.trim() != s.host ||
            (portField.text.trim().toIntOrNull() ?: 0) != s.fixedPort ||
            homeField.text.trim() != s.foxxyHome ||
            extraArgsField.text.trim() != s.extraArgs ||
            followThemeCheckBox.isSelected != s.followIdeTheme ||
            nativeDiffsCheckBox.isSelected != s.nativeDiffs ||
            autoApproveCheckBox.isSelected != s.autoApproveEdits
    }

    override fun apply() {
        val s = settings
        s.binaryPath = pathField.text.trim()
        s.host = hostField.text.trim().ifBlank { "127.0.0.1" }
        s.fixedPort = (portField.text.trim().toIntOrNull() ?: 0).coerceIn(0, 65535)
        s.foxxyHome = homeField.text.trim()
        s.extraArgs = extraArgsField.text.trim()
        s.followIdeTheme = followThemeCheckBox.isSelected
        s.nativeDiffs = nativeDiffsCheckBox.isSelected
        s.autoApproveEdits = autoApproveCheckBox.isSelected
        s.firstRunCompleted = true
    }

    override fun reset() {
        val s = settings
        pathField.text = s.binaryPath
        hostField.text = s.host
        portField.text = s.fixedPort.toString()
        homeField.text = s.foxxyHome
        extraArgsField.text = s.extraArgs
        followThemeCheckBox.isSelected = s.followIdeTheme
        nativeDiffsCheckBox.isSelected = s.nativeDiffs
        autoApproveCheckBox.isSelected = s.autoApproveEdits
        statusLabel.text = " "
    }
}
