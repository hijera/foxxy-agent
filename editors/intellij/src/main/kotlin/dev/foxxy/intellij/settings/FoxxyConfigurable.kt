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
import dev.foxxy.intellij.FoxxyBundle
import dev.foxxy.intellij.binary.FoxxyBinaryResolver
import java.io.File
import javax.swing.JButton
import javax.swing.JComboBox
import javax.swing.JComponent
import javax.swing.JPanel

/**
 * Settings | Tools | Foxxy — configure the `foxxy http` launch parameters and an optional
 * binary path override. The foxxy binary is bundled with the plugin; leave "Binary path"
 * empty to use the bundled one.
 */
class FoxxyConfigurable : Configurable {
    private val languageBox = JComboBox<String>()
    private val pathField = TextFieldWithBrowseButton()
    private val hostField = JBTextField()
    private val portField = JBTextField()
    private val homeField = TextFieldWithBrowseButton()
    private val extraArgsField = JBTextField()
    private val followThemeCheckBox = JBCheckBox()
    private val nativeDiffsCheckBox = JBCheckBox()
    private val autoApproveCheckBox = JBCheckBox()
    private val statusLabel = JBLabel(" ")

    private val settings get() = FoxxySettings.getInstance().state

    override fun getDisplayName(): String = FoxxyBundle.message("settings.displayName")

    override fun createComponent(): JComponent {
        pathField.addBrowseFolderListener(
            FoxxyBundle.message("settings.browse.binaryTitle"),
            FoxxyBundle.message("settings.browse.binaryDescription"),
            null,
            FileChooserDescriptorFactory.createSingleFileDescriptor()
        )
        homeField.addBrowseFolderListener(
            FoxxyBundle.message("settings.browse.homeTitle"),
            FoxxyBundle.message("settings.browse.homeDescription"),
            null,
            FileChooserDescriptorFactory.createSingleFolderDescriptor()
        )

        val actions = JPanel().apply {
            add(JButton(FoxxyBundle.message("settings.button.verify")).apply { addActionListener { verify() } })
        }

        val panel = FormBuilder.createFormBuilder()
            .addLabeledComponent(FoxxyBundle.message("settings.label.language"), languageBox)
            .addSeparator()
            .addLabeledComponent(
                FoxxyBundle.message("settings.label.binaryPath"),
                pathField
            )
            .addComponent(
                JBLabel(FoxxyBundle.message("settings.hint.binaryPath"))
            )
            .addComponent(actions)
            .addComponent(statusLabel)
            .addSeparator()
            .addLabeledComponent(FoxxyBundle.message("settings.label.host"), hostField)
            .addLabeledComponent(FoxxyBundle.message("settings.label.port"), portField)
            .addLabeledComponent(FoxxyBundle.message("settings.label.home"), homeField)
            .addLabeledComponent(FoxxyBundle.message("settings.label.extraArgs"), extraArgsField)
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
            statusLabel.text = FoxxyBundle.message("settings.status.noBinary")
            return
        }
        ProgressManager.getInstance().runProcessWithProgressSynchronously({
            val v = FoxxyBinaryResolver.validate(bin)
            ApplicationManager.getApplication().invokeLater { statusLabel.text = v.message }
        }, FoxxyBundle.message("settings.status.verifying"), true, null)
    }

    override fun isModified(): Boolean {
        val s = settings
        return languageCode(languageBox.selectedIndex) != s.language ||
            pathField.text.trim() != s.binaryPath ||
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
        val prevLanguage = s.language
        s.language = languageCode(languageBox.selectedIndex)
        s.binaryPath = pathField.text.trim()
        s.host = hostField.text.trim().ifBlank { "127.0.0.1" }
        s.fixedPort = (portField.text.trim().toIntOrNull() ?: 0).coerceIn(0, 65535)
        s.foxxyHome = homeField.text.trim()
        s.extraArgs = extraArgsField.text.trim()
        s.followIdeTheme = followThemeCheckBox.isSelected
        s.nativeDiffs = nativeDiffsCheckBox.isSelected
        s.autoApproveEdits = autoApproveCheckBox.isSelected
        s.firstRunCompleted = true
        reset()
        if (s.language != prevLanguage) {
            ApplicationManager.getApplication().messageBus
                .syncPublisher(dev.foxxy.intellij.ui.FoxxyLanguageListener.TOPIC)
                .languageChanged()
        }
    }

    override fun reset() {
        val s = settings
        refreshLanguageBox(s.language)
        pathField.text = s.binaryPath
        hostField.text = s.host
        portField.text = s.fixedPort.toString()
        homeField.text = s.foxxyHome
        extraArgsField.text = s.extraArgs
        followThemeCheckBox.text = FoxxyBundle.message("settings.checkbox.followTheme")
        nativeDiffsCheckBox.text = FoxxyBundle.message("settings.checkbox.nativeDiffs")
        autoApproveCheckBox.text = FoxxyBundle.message("settings.checkbox.autoApprove")
        followThemeCheckBox.isSelected = s.followIdeTheme
        nativeDiffsCheckBox.isSelected = s.nativeDiffs
        autoApproveCheckBox.isSelected = s.autoApproveEdits
        statusLabel.text = " "
    }

    private fun refreshLanguageBox(selectedCode: String) {
        languageBox.removeAllItems()
        for ((label, code) in languageOptions()) {
            languageBox.addItem(label)
            if (code == selectedCode) {
                languageBox.selectedIndex = languageBox.itemCount - 1
            }
        }
        if (languageBox.selectedIndex < 0 && languageBox.itemCount > 0) {
            languageBox.selectedIndex = 0
        }
    }

    companion object {
        private val LANGUAGE_CODES = listOf("system", "en", "ru")

        private fun languageOptions(): List<Pair<String, String>> = listOf(
            FoxxyBundle.message("settings.language.system") to "system",
            FoxxyBundle.message("settings.language.en") to "en",
            FoxxyBundle.message("settings.language.ru") to "ru",
        )

        private fun languageCode(index: Int): String =
            LANGUAGE_CODES.getOrElse(index) { "system" }
    }
}
