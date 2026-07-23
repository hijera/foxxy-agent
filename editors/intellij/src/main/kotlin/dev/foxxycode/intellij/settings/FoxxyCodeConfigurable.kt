package dev.foxxycode.intellij.settings

import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.fileChooser.FileChooserDescriptorFactory
import com.intellij.openapi.options.Configurable
import com.intellij.openapi.progress.ProgressManager
import com.intellij.openapi.ui.TextFieldWithBrowseButton
import com.intellij.ui.components.JBCheckBox
import com.intellij.ui.components.JBLabel
import com.intellij.ui.components.JBTextField
import com.intellij.util.ui.FormBuilder
import dev.foxxycode.intellij.FoxxyCodeBundle
import dev.foxxycode.intellij.binary.FoxxyCodeBinaryResolver
import java.io.File
import javax.swing.JButton
import javax.swing.JComponent
import javax.swing.JPanel

/**
 * Settings | Tools | FoxxyCode — configure the `foxxycode http` launch parameters and an optional
 * binary path override. The foxxycode binary is bundled with the plugin; leave "Binary path"
 * empty to use the bundled one.
 */
class FoxxyCodeConfigurable : Configurable {
    private val pathField = TextFieldWithBrowseButton()
    private val hostField = JBTextField()
    private val portField = JBTextField()
    private val homeField = TextFieldWithBrowseButton()
    private val extraArgsField = JBTextField()
    private val followThemeCheckBox = JBCheckBox()
    private val nativeDiffsCheckBox = JBCheckBox()
    private val autoApproveCheckBox = JBCheckBox()
    private val trackOpenFilesCheckBox = JBCheckBox()
    private val trackTerminalsCheckBox = JBCheckBox()
    private val planNoSelfRunCheckBox = JBCheckBox()
    private val statusLabel = JBLabel(" ")

    private val settings get() = FoxxyCodeSettings.getInstance().state

    override fun getDisplayName(): String = FoxxyCodeBundle.message("settings.displayName")

    override fun createComponent(): JComponent {
        pathField.addBrowseFolderListener(
            FoxxyCodeBundle.message("settings.browse.binaryTitle"),
            FoxxyCodeBundle.message("settings.browse.binaryDescription"),
            null,
            FileChooserDescriptorFactory.createSingleFileDescriptor()
        )
        homeField.addBrowseFolderListener(
            FoxxyCodeBundle.message("settings.browse.homeTitle"),
            FoxxyCodeBundle.message("settings.browse.homeDescription"),
            null,
            FileChooserDescriptorFactory.createSingleFolderDescriptor()
        )

        val actions = JPanel().apply {
            add(JButton(FoxxyCodeBundle.message("settings.button.verify")).apply { addActionListener { verify() } })
        }

        val panel = FormBuilder.createFormBuilder()
            .addLabeledComponent(
                FoxxyCodeBundle.message("settings.label.binaryPath"),
                pathField
            )
            .addComponent(
                JBLabel(FoxxyCodeBundle.message("settings.hint.binaryPath"))
            )
            .addComponent(actions)
            .addComponent(statusLabel)
            .addSeparator()
            .addLabeledComponent(FoxxyCodeBundle.message("settings.label.host"), hostField)
            .addLabeledComponent(FoxxyCodeBundle.message("settings.label.port"), portField)
            .addLabeledComponent(FoxxyCodeBundle.message("settings.label.home"), homeField)
            .addLabeledComponent(FoxxyCodeBundle.message("settings.label.extraArgs"), extraArgsField)
            .addSeparator()
            .addComponent(followThemeCheckBox)
            .addSeparator()
            .addComponent(nativeDiffsCheckBox)
            .addComponent(autoApproveCheckBox)
            .addSeparator()
            .addComponent(trackOpenFilesCheckBox)
            .addComponent(trackTerminalsCheckBox)
            .addSeparator()
            .addComponent(planNoSelfRunCheckBox)
            .addComponentFillVertically(JPanel(), 0)
            .panel
        reset()
        return panel
    }

    private fun currentBinary(): File? {
        val p = pathField.text.trim()
        return if (p.isBlank()) FoxxyCodeBinaryResolver.resolveExisting() else File(p).takeIf { it.isFile }
    }

    private fun verify() {
        val bin = currentBinary()
        if (bin == null) {
            statusLabel.text = FoxxyCodeBundle.message("settings.status.noBinary")
            return
        }
        ProgressManager.getInstance().runProcessWithProgressSynchronously({
            val v = FoxxyCodeBinaryResolver.validate(bin)
            ApplicationManager.getApplication().invokeLater { statusLabel.text = v.message }
        }, FoxxyCodeBundle.message("settings.status.verifying"), true, null)
    }

    override fun isModified(): Boolean {
        val s = settings
        return pathField.text.trim() != s.binaryPath ||
            hostField.text.trim() != s.host ||
            (portField.text.trim().toIntOrNull() ?: 0) != s.fixedPort ||
            homeField.text.trim() != s.foxxycodeHome ||
            extraArgsField.text.trim() != s.extraArgs ||
            followThemeCheckBox.isSelected != s.followIdeTheme ||
            nativeDiffsCheckBox.isSelected != s.nativeDiffs ||
            autoApproveCheckBox.isSelected != s.autoApproveEdits ||
            trackOpenFilesCheckBox.isSelected != s.trackOpenFiles ||
            trackTerminalsCheckBox.isSelected != s.trackTerminals ||
            planNoSelfRunCheckBox.isSelected != s.planNoSelfRun
    }

    override fun apply() {
        val s = settings
        s.binaryPath = pathField.text.trim()
        s.host = hostField.text.trim().ifBlank { "127.0.0.1" }
        s.fixedPort = (portField.text.trim().toIntOrNull() ?: 0).coerceIn(0, 65535)
        s.foxxycodeHome = homeField.text.trim()
        s.extraArgs = extraArgsField.text.trim()
        s.followIdeTheme = followThemeCheckBox.isSelected
        s.nativeDiffs = nativeDiffsCheckBox.isSelected
        s.autoApproveEdits = autoApproveCheckBox.isSelected
        s.trackOpenFiles = trackOpenFilesCheckBox.isSelected
        s.trackTerminals = trackTerminalsCheckBox.isSelected
        s.planNoSelfRun = planNoSelfRunCheckBox.isSelected
        s.firstRunCompleted = true
        reset()
    }

    override fun reset() {
        val s = settings
        pathField.text = s.binaryPath
        hostField.text = s.host
        portField.text = s.fixedPort.toString()
        homeField.text = s.foxxycodeHome
        extraArgsField.text = s.extraArgs
        followThemeCheckBox.text = FoxxyCodeBundle.message("settings.checkbox.followTheme")
        nativeDiffsCheckBox.text = FoxxyCodeBundle.message("settings.checkbox.nativeDiffs")
        autoApproveCheckBox.text = FoxxyCodeBundle.message("settings.checkbox.autoApprove")
        trackOpenFilesCheckBox.text = FoxxyCodeBundle.message("settings.checkbox.trackOpenFiles")
        trackTerminalsCheckBox.text = FoxxyCodeBundle.message("settings.checkbox.trackTerminals")
        planNoSelfRunCheckBox.text = FoxxyCodeBundle.message("settings.checkbox.planNoSelfRun")
        followThemeCheckBox.isSelected = s.followIdeTheme
        nativeDiffsCheckBox.isSelected = s.nativeDiffs
        autoApproveCheckBox.isSelected = s.autoApproveEdits
        trackOpenFilesCheckBox.isSelected = s.trackOpenFiles
        trackTerminalsCheckBox.isSelected = s.trackTerminals
        planNoSelfRunCheckBox.isSelected = s.planNoSelfRun
        statusLabel.text = " "
    }
}
