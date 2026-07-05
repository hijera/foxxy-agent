package dev.foxxycode.intellij.ui

import com.intellij.openapi.project.Project
import com.intellij.openapi.ui.DialogWrapper
import com.intellij.ui.components.JBScrollPane
import com.intellij.ui.jcef.JBCefApp
import com.intellij.ui.jcef.JBCefBrowser
import com.intellij.ui.components.JBLabel
import com.intellij.util.ui.JBUI
import dev.foxxycode.intellij.FoxxyCodeBundle
import dev.foxxycode.intellij.settings.FoxxyCodeSettings
import java.awt.BorderLayout
import java.awt.FlowLayout
import javax.swing.Action
import javax.swing.JButton
import javax.swing.JComponent
import javax.swing.JPanel

/**
 * Multi-step onboarding wizard rendered in JCEF (with a JBLabel fallback when JCEF is unavailable).
 * Re-open anytime via [FoxxyCodeWelcomeAction] or the first-run flow in [FirstRunDialog].
 */
class WelcomeWizardDialog(
    project: Project,
    private val markFirstRunComplete: Boolean = true,
) : DialogWrapper(project) {
    private val totalSteps = 5
    private var step = 0
    private var browser: JBCefBrowser? = null
    private val root = JPanel(BorderLayout())
    private val prevButton = JButton()
    private val nextButton = JButton()
    private val skipButton = JButton()

    init {
        title = FoxxyCodeBundle.message("welcome.title")
        init()
        prevButton.addActionListener { goStep(step - 1) }
        nextButton.addActionListener {
            if (step < totalSteps - 1) {
                goStep(step + 1)
            } else {
                persistFirstRunIfNeeded()
                close(OK_EXIT_CODE)
            }
        }
        skipButton.addActionListener {
            persistFirstRunIfNeeded()
            close(CANCEL_EXIT_CODE)
        }
        updateNavButtons()
    }

    override fun createCenterPanel(): JComponent {
        if (!JBCefApp.isSupported()) {
            val label = JBLabel(FoxxyCodeBundle.message("welcome.fallback.body"))
            label.setAllowAutoWrapping(true)
            return JBScrollPane(label)
        }
        val html = loadWelcomeHtml()
        browser = JBCefBrowser().also { it.loadHTML(html) }
        root.add(browser!!.component, BorderLayout.CENTER)
        val nav = JPanel(FlowLayout(FlowLayout.CENTER, JBUI.scale(8), 0))
        nav.add(prevButton)
        nav.add(nextButton)
        nav.add(skipButton)
        root.add(nav, BorderLayout.SOUTH)
        root.preferredSize = JBUI.size(520, 420)
        goStep(0)
        return root
    }

    /** Hide default OK/Cancel; navigation lives in the wizard footer. */
    override fun createActions(): Array<Action> = emptyArray()

    /** Shows the wizard (modal) and marks first-run completed when [markFirstRunComplete] is true. */
    fun showAndConfigure() {
        showAndGet()
        persistFirstRunIfNeeded()
    }

    private fun persistFirstRunIfNeeded() {
        if (markFirstRunComplete) {
            FoxxyCodeSettings.getInstance().state.firstRunCompleted = true
        }
    }

    private fun goStep(index: Int) {
        step = index.coerceIn(0, totalSteps - 1)
        browser?.cefBrowser?.executeJavaScript("window.setStep($step)", "about:blank", 0)
        updateNavButtons()
    }

    private fun updateNavButtons() {
        prevButton.text = FoxxyCodeBundle.message("welcome.button.prev")
        prevButton.isEnabled = step > 0
        nextButton.text = if (step == totalSteps - 1) {
            FoxxyCodeBundle.message("welcome.button.finish")
        } else {
            FoxxyCodeBundle.message("welcome.button.next")
        }
        skipButton.text = FoxxyCodeBundle.message("welcome.button.skip")
    }

    private fun loadWelcomeHtml(): String {
        val stream = WelcomeWizardDialog::class.java.classLoader.getResourceAsStream("welcome/welcome.html")
            ?: error("welcome/welcome.html missing from plugin resources")
        var html = stream.bufferedReader().readText()
        val keys = listOf(
            "welcome.step1.title", "welcome.step1.body",
            "welcome.step2.title", "welcome.step2.body",
            "welcome.step3.title", "welcome.step3.body",
            "welcome.step4.title", "welcome.step4.body",
            "welcome.step5.title", "welcome.step5.body",
        )
        for (key in keys) {
            html = html.replace("%$key%", FoxxyCodeBundle.message(key))
        }
        return html
    }
}
