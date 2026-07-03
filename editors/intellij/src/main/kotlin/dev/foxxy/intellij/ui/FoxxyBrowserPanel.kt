package dev.foxxy.intellij.ui

import com.intellij.icons.AllIcons
import com.intellij.ide.BrowserUtil
import com.intellij.ide.ui.LafManagerListener
import com.intellij.openapi.Disposable
import com.intellij.openapi.actionSystem.ActionManager
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.actionSystem.DefaultActionGroup
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.options.ShowSettingsUtil
import com.intellij.openapi.project.Project
import com.intellij.openapi.util.Disposer
import com.intellij.ui.jcef.JBCefApp
import com.intellij.ui.jcef.JBCefBrowser
import dev.foxxy.intellij.FoxxyBundle
import dev.foxxy.intellij.FoxxyNotifications
import dev.foxxy.intellij.diff.FoxxyIdeDiffService
import dev.foxxy.intellij.process.FoxxyProcessManager
import dev.foxxy.intellij.settings.FoxxySettings
import org.cef.browser.CefBrowser
import org.cef.browser.CefFrame
import org.cef.handler.CefLoadHandlerAdapter
import java.awt.BorderLayout
import javax.swing.JButton
import javax.swing.JComponent
import javax.swing.JLabel
import javax.swing.JPanel
import javax.swing.SwingConstants

/**
 * Tool window content: hosts the Foxxy web UI in a JCEF browser (with graceful fallbacks)
 * and manages the backing `foxxy http` process via [FoxxyProcessManager].
 */
class FoxxyBrowserPanel(private val project: Project) : JPanel(BorderLayout()), Disposable {
    private var browser: JBCefBrowser? = null
    private val center = JPanel(BorderLayout())

    @Volatile
    private var currentUrl: String? = null

    init {
        add(createToolbar(), BorderLayout.NORTH)
        add(center, BorderLayout.CENTER)
        // Re-apply the matching Foxxy theme whenever the IDE look-and-feel changes.
        ApplicationManager.getApplication().messageBus.connect(this)
            .subscribe(LafManagerListener.TOPIC, LafManagerListener { syncTheme() })
        start()
    }

    private fun start() {
        showMessage(FoxxyBundle.message("process.status.starting"))
        FoxxyProcessManager.getInstance(project).ensureStarted(
            onReady = { url ->
                loadUrl(url)
                // Wire native inline diffs once the server (and its /coddy/ide/events stream) is up.
                FoxxyIdeDiffService.getInstance(project).startIfNeeded()
            },
            onError = { msg -> showError(msg) }
        )
    }

    private fun loadUrl(url: String) {
        // Append the IDE-matching theme so the SPA applies it before first paint
        // (contract: docs/intellij-embedding.md). Avoid duplicating the param.
        val themeParam = if (url.contains("theme=")) url
        else url + (if (url.contains("?")) "&" else "?") + "theme=${FoxxyThemeBridge.currentFoxxyTheme()}"
        // Signal the embed mode so the SPA adopts a flatter, more native host-IDE
        // look (data-embed="intellij" on <html>; see docs/intellij-embedding.md).
        val finalUrl = if (themeParam.contains("embed=")) themeParam
        else themeParam + (if (themeParam.contains("?")) "&" else "?") + "embed=intellij"
        currentUrl = finalUrl
        if (!JBCefApp.isSupported()) {
            showFallback(finalUrl)
            return
        }
        val b = browser ?: JBCefBrowser(finalUrl).also {
            browser = it
            Disposer.register(this, it)
            // After each page load: install compatibility shims/error overlay, then sync theme.
            it.jbCefClient.addLoadHandler(object : CefLoadHandlerAdapter() {
                override fun onLoadEnd(b: CefBrowser?, frame: CefFrame?, httpStatusCode: Int) {
                    if (frame?.isMain == true) {
                        injectBootstrap()
                        syncTheme()
                    }
                }
            }, it.cefBrowser)
        }
        b.loadURL(finalUrl)
        setCenter(b.component)
    }

    /**
     * Injects compatibility shims into the embedded page:
     *  - polyfills `crypto.randomUUID` (missing in Chromium < 92, e.g. older JCEF runtimes), which
     *    the Foxxy SPA calls when creating a chat draft — without it the UI crashes to a blank page;
     *  - renders uncaught JS errors as a visible overlay instead of silently unmounting the app.
     */
    private fun injectBootstrap() {
        val b = browser ?: return
        b.cefBrowser.executeJavaScript(BOOTSTRAP_JS, b.cefBrowser.url ?: "", 0)
    }

    /** Injects JS that aligns the Foxxy web UI theme with the current IDE theme. */
    private fun syncTheme() {
        if (!FoxxySettings.getInstance().state.followIdeTheme) return
        val b = browser ?: return
        val js = FoxxyThemeBridge.applyThemeJs(FoxxyThemeBridge.currentFoxxyTheme())
        b.cefBrowser.executeJavaScript(js, b.cefBrowser.url ?: "", 0)
    }

    private fun setCenter(component: JComponent) {
        center.removeAll()
        center.add(component, BorderLayout.CENTER)
        center.revalidate()
        center.repaint()
    }

    private fun showMessage(text: String) {
        setCenter(JLabel(text, SwingConstants.CENTER))
    }

    private fun showError(msg: String) {
        FoxxyNotifications.error(project, FoxxyBundle.message("notification.title.startFailed"), msg)
        val panel = JPanel(BorderLayout())
        panel.add(
            JLabel(FoxxyBundle.message("process.error.startFailedPanel", msg), SwingConstants.CENTER),
            BorderLayout.CENTER
        )
        val south = JPanel()
        south.add(JButton(FoxxyBundle.message("process.button.retry")).apply { addActionListener { start() } })
        south.add(JButton(FoxxyBundle.message("process.button.openSettings")).apply { addActionListener { openSettings() } })
        panel.add(south, BorderLayout.SOUTH)
        setCenter(panel)
    }

    private fun showFallback(url: String) {
        val panel = JPanel(BorderLayout())
        panel.add(
            JLabel(
                FoxxyBundle.message("process.fallback.jcefUnavailable"),
                SwingConstants.CENTER
            ),
            BorderLayout.CENTER
        )
        val south = JPanel()
        south.add(JButton(FoxxyBundle.message("process.button.openUrl", url)).apply { addActionListener { BrowserUtil.browse(url) } })
        panel.add(south, BorderLayout.SOUTH)
        setCenter(panel)
    }

    private fun openSettings() {
        ShowSettingsUtil.getInstance().showSettingsDialog(project, FoxxyBundle.message("settings.displayName"))
    }

    private fun createToolbar(): JComponent {
        val group = DefaultActionGroup()
        group.add(object : AnAction(
            FoxxyBundle.message("toolbar.action.restart"),
            FoxxyBundle.message("toolbar.action.restart.desc"),
            AllIcons.Actions.Restart
        ) {
            override fun actionPerformed(e: AnActionEvent) {
                showMessage(FoxxyBundle.message("process.status.restarting"))
                FoxxyProcessManager.getInstance(project).restart(
                    onReady = { url ->
                        loadUrl(url)
                        FoxxyIdeDiffService.getInstance(project).startIfNeeded()
                    },
                    onError = { msg -> showError(msg) }
                )
            }
        })
        group.add(object : AnAction(
            FoxxyBundle.message("toolbar.action.reload"),
            FoxxyBundle.message("toolbar.action.reload.desc"),
            AllIcons.Actions.Refresh
        ) {
            override fun actionPerformed(e: AnActionEvent) {
                val b = browser
                if (b != null) b.cefBrowser.reload() else start()
            }
        })
        group.add(object : AnAction(
            FoxxyBundle.message("toolbar.action.openBrowser"),
            FoxxyBundle.message("toolbar.action.openBrowser.desc"),
            null
        ) {
            override fun actionPerformed(e: AnActionEvent) {
                currentUrl?.let { BrowserUtil.browse(it) }
            }
        })
        group.add(object : AnAction(
            FoxxyBundle.message("toolbar.action.devtools"),
            FoxxyBundle.message("toolbar.action.devtools.desc"),
            null
        ) {
            override fun actionPerformed(e: AnActionEvent) {
                browser?.openDevtools()
            }
        })
        group.add(object : AnAction(
            FoxxyBundle.message("toolbar.action.settings"),
            FoxxyBundle.message("toolbar.action.settings.desc"),
            AllIcons.General.Settings
        ) {
            override fun actionPerformed(e: AnActionEvent) = openSettings()
        })
        val toolbar = ActionManager.getInstance().createActionToolbar("FoxxyToolbar", group, true)
        toolbar.setTargetComponent(this) // 2021.2 exposes only the setter, not a property
        return toolbar.component
    }

    override fun dispose() {
        // JBCefBrowser is released via its Disposer registration on this panel.
    }

    companion object {
        private val BOOTSTRAP_JS = """
            (function () {
              // Polyfill crypto.randomUUID for older embedded Chromium (< 92).
              try {
                var c = window.crypto || window.msCrypto;
                if (c && typeof c.randomUUID !== "function" && c.getRandomValues) {
                  c.randomUUID = function () {
                    var b = c.getRandomValues(new Uint8Array(16));
                    b[6] = (b[6] & 0x0f) | 0x40;
                    b[8] = (b[8] & 0x3f) | 0x80;
                    var h = [];
                    for (var i = 0; i < 16; i++) h.push((b[i] + 0x100).toString(16).slice(1));
                    return h[0]+h[1]+h[2]+h[3]+"-"+h[4]+h[5]+"-"+h[6]+h[7]+"-"+h[8]+h[9]+"-"+h[10]+h[11]+h[12]+h[13]+h[14]+h[15];
                  };
                }
              } catch (e) {}

              // Show uncaught errors as an overlay instead of letting the SPA go blank.
              try {
                if (!window.__foxxyErrOverlayInstalled) {
                  window.__foxxyErrOverlayInstalled = true;
                  var show = function (title, detail) {
                    try {
                      var el = document.getElementById("foxxy-err-overlay");
                      if (!el) {
                        el = document.createElement("div");
                        el.id = "foxxy-err-overlay";
                        el.style.cssText = "position:fixed;left:0;right:0;bottom:0;z-index:2147483647;max-height:45vh;overflow:auto;background:#7f1d1d;color:#fff;font:12px/1.45 monospace;padding:10px 12px;white-space:pre-wrap;border-top:2px solid #ef4444";
                        (document.body || document.documentElement).appendChild(el);
                      }
                      el.textContent = "Foxxy UI error — " + title + "\n" + (detail || "");
                    } catch (e) {}
                  };
                  window.addEventListener("error", function (ev) {
                    show(ev.message || "error", (ev.error && ev.error.stack) ? ev.error.stack : (ev.filename + ":" + ev.lineno));
                  });
                  window.addEventListener("unhandledrejection", function (ev) {
                    var r = ev.reason;
                    show("unhandled promise rejection", (r && (r.stack || r.message)) ? (r.stack || r.message) : String(r));
                  });
                }
              } catch (e) {}
            })();
        """.trimIndent()
    }
}
