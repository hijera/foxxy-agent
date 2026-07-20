package dev.foxxycode.intellij.ui

import com.google.gson.JsonPrimitive
import com.intellij.icons.AllIcons
import com.intellij.ide.BrowserUtil
import com.intellij.ide.dnd.DnDDropHandler
import com.intellij.ide.dnd.DnDEvent
import com.intellij.ide.dnd.DnDSupport
import com.intellij.ide.dnd.DnDTargetChecker
import com.intellij.ide.dnd.TransferableWrapper
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
import com.intellij.ui.jcef.JBCefBrowserBase
import com.intellij.ui.jcef.JBCefJSQuery
import dev.foxxycode.intellij.FoxxyCodeBundle
import dev.foxxycode.intellij.FoxxyCodeLocaleState
import dev.foxxycode.intellij.FoxxyCodeNotifications
import dev.foxxycode.intellij.diff.FoxxyCodeIdeDiffService
import dev.foxxycode.intellij.editor.FoxxyCodeEditorContextService
import dev.foxxycode.intellij.process.FoxxyCodeProcessManager
import dev.foxxycode.intellij.terminal.FoxxyCodeTerminalContextService
import dev.foxxycode.intellij.settings.FoxxyCodeSettings
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
 * Tool window content: hosts the FoxxyCode web UI in a JCEF browser (with graceful fallbacks)
 * and manages the backing `foxxycode http` process via [FoxxyCodeProcessManager].
 */
class FoxxyCodeBrowserPanel(private val project: Project) : JPanel(BorderLayout()), Disposable {
    private var browser: JBCefBrowser? = null
    /** JS→Kotlin channel: the SPA calls this when its locale changes (single app-wide switcher). */
    private var localeQuery: JBCefJSQuery? = null
    private val center = JPanel(BorderLayout())
    private var toolbarComponent: JComponent? = null

    @Volatile
    private var currentUrl: String? = null

    /** Last status/error message key for re-localization on language change. */
    private var statusMessageKey: String? = null
    private var statusMessageParams: Array<out Any> = emptyArray()
    private enum class PanelMode { NONE, MESSAGE, ERROR, FALLBACK, BROWSER }
    private var panelMode = PanelMode.NONE

    init {
        val tb = createToolbar()
        toolbarComponent = tb
        add(tb, BorderLayout.NORTH)
        add(center, BorderLayout.CENTER)
        ApplicationManager.getApplication().messageBus.connect(this)
            .subscribe(LafManagerListener.TOPIC, LafManagerListener { syncTheme() })
        ApplicationManager.getApplication().messageBus.connect(this)
            .subscribe(
                FoxxyCodeLanguageListener.TOPIC,
                object : FoxxyCodeLanguageListener {
                    override fun languageChanged() = onLanguageChanged()
                },
            )
        start()
    }

    private fun onLanguageChanged() {
        refreshToolbar()
        when (panelMode) {
            PanelMode.MESSAGE -> statusMessageKey?.let {
                showMessage(FoxxyCodeBundle.message(it, *statusMessageParams))
            }
            PanelMode.ERROR -> lastErrorDetail?.let { showError(it) }
            PanelMode.FALLBACK -> currentUrl?.let { showFallback(it) }
            PanelMode.BROWSER, PanelMode.NONE -> {}
        }
        syncLocale()
    }

    private fun refreshToolbar() {
        val old = toolbarComponent
        if (old != null) {
            remove(old)
        }
        val tb = createToolbar()
        toolbarComponent = tb
        add(tb, BorderLayout.NORTH)
        revalidate()
        repaint()
    }

    private fun start() {
        showStatusMessage("process.status.starting")
        FoxxyCodeProcessManager.getInstance(project).ensureStarted(
            onReady = { url ->
                loadUrl(url)
                // Wire native inline diffs once the server (and its /foxxycode/ide/events stream) is up.
                FoxxyCodeIdeDiffService.getInstance(project).startIfNeeded()
                // Start reporting open tabs / active file to the agent.
                FoxxyCodeEditorContextService.getInstance(project).startIfNeeded()
                // Start reporting open terminals + recent output to the agent.
                FoxxyCodeTerminalContextService.getInstance(project).startIfNeeded()
            },
            onError = { msg -> showError(msg) }
        )
    }

    private fun loadUrl(url: String) {
        // Append the IDE-matching theme so the SPA applies it before first paint
        // (contract: docs/intellij-embedding.md). Avoid duplicating the param.
        val themeParam = if (url.contains("theme=")) url
        else url + (if (url.contains("?")) "&" else "?") + "theme=${FoxxyCodeThemeBridge.currentFoxxyCodeTheme()}"
        // Signal the embed mode so the SPA adopts a flatter, more native host-IDE
        // look (data-embed="intellij" on <html>; see docs/intellij-embedding.md).
        var finalUrl = if (themeParam.contains("embed=")) themeParam
        else themeParam + (if (themeParam.contains("?")) "&" else "?") + "embed=intellij"
        finalUrl = appendLangParam(finalUrl)
        currentUrl = finalUrl
        if (!JBCefApp.isSupported()) {
            showFallback(finalUrl)
            return
        }
        val b = browser ?: JBCefBrowser(finalUrl).also {
            browser = it
            Disposer.register(this, it)
            // JS→Kotlin channel for SPA-originated locale changes. Created before
            // the first load so onLoadEnd can inject its subscription.
            val query = JBCefJSQuery.create(it as JBCefBrowserBase)
            query.addHandler { value -> onSpaLocale(value); null }
            Disposer.register(it, query)
            localeQuery = query
            // After each page load: install compatibility shims/error overlay, then sync theme + locale.
            it.jbCefClient.addLoadHandler(object : CefLoadHandlerAdapter() {
                override fun onLoadEnd(b: CefBrowser?, frame: CefFrame?, httpStatusCode: Int) {
                    if (frame?.isMain == true) {
                        injectBootstrap()
                        injectLocaleBridge()
                        syncTheme()
                        syncLocale()
                    }
                }
            }, it.cefBrowser)
        }
        b.loadURL(finalUrl)
        setCenter(b.component)
        installFileDropTarget(b.component)
        panelMode = PanelMode.BROWSER
    }

    /** Guard so the drop target is registered on the browser component only once. */
    private var fileDropInstalled = false

    /**
     * Accepts file drags from the Project view onto the browser and inserts each dropped
     * file into the composer as a short `@`-mention (via [insertFileMention]). Uses the
     * IntelliJ DnD framework so it works over the heavyweight JCEF component. Best-effort:
     * a drag that carries no files is ignored so normal editor DnD is unaffected.
     */
    private fun installFileDropTarget(component: JComponent) {
        if (fileDropInstalled) return
        fileDropInstalled = true
        val browserRef = browser ?: return
        DnDSupport.createBuilder(component)
            .setTargetChecker(
                DnDTargetChecker { event ->
                    if (droppedProjectFiles(event).isNotEmpty()) {
                        event.isDropPossible = true
                        true
                    } else {
                        false
                    }
                },
            )
            .setDropHandler(
                DnDDropHandler { event ->
                    for (rel in droppedProjectFiles(event)) {
                        insertFileMention(rel)
                    }
                },
            )
            .setDisposableParent(browserRef)
            .install()
    }

    /** Project-relative POSIX paths for files carried by a DnD event (empty when none). */
    private fun droppedProjectFiles(event: DnDEvent): List<String> {
        val attached = event.attachedObject
        if (attached !is TransferableWrapper) return emptyList()
        val files = attached.asFileList() ?: return emptyList()
        val base = project.basePath ?: return emptyList()
        val basePath = try {
            java.nio.file.Paths.get(base)
        } catch (e: Exception) {
            return emptyList()
        }
        val out = ArrayList<String>()
        for (f in files) {
            if (f.isDirectory) continue
            val rel = try {
                basePath.relativize(f.toPath()).toString().replace('\\', '/')
            } catch (e: Exception) {
                continue
            }
            if (rel.isEmpty() || rel.startsWith("..")) continue
            out.add(rel)
        }
        return out
    }

    /** Pushes a workspace-relative path into the SPA composer as an `@`-mention chip. */
    private fun insertFileMention(relPath: String) {
        val b = browser ?: return
        val json = JsonPrimitive(relPath).toString()
        val js =
            "(function(){ try { if (window.foxxycodeUi && window.foxxycodeUi.insertFileMention) window.foxxycodeUi.insertFileMention($json); } catch (e) {} })();"
        b.cefBrowser.executeJavaScript(js, b.cefBrowser.url ?: "", 0)
    }

    private fun appendLangParam(url: String): String {
        if (url.contains("lang=")) return url
        val lang = FoxxyCodeBundle.spaLanguageCode()
        return url + (if (url.contains("?")) "&" else "?") + "lang=$lang"
    }

    /** Injects JS that aligns the FoxxyCode web UI locale with plugin settings. */
    private fun syncLocale() {
        val b = browser ?: return
        val lang = FoxxyCodeBundle.spaLanguageCode().replace("\"", "")
        val js = "(function(){ try { if (window.foxxycodeUi) window.foxxycodeUi.setLocale(\"$lang\"); } catch (e) {} })();"
        b.cefBrowser.executeJavaScript(js, b.cefBrowser.url ?: "", 0)
    }

    /**
     * Subscribes to SPA-driven locale changes: when the user flips the single
     * app-wide switcher (SPA Settings → General), the SPA calls back through the
     * JBCefJSQuery channel so plugin chrome re-localizes. The guard resets on each
     * page load, so a reload re-subscribes exactly once.
     */
    private fun injectLocaleBridge() {
        val b = browser ?: return
        val query = localeQuery ?: return
        val js = """
            (function(){ try {
              if (window.foxxycodeUi && !window.__foxxycodeLocaleBridge) {
                window.__foxxycodeLocaleBridge = true;
                window.foxxycodeUi.onLocaleChange(function (l) { ${query.inject("l")} });
              }
            } catch (e) {} })();
        """.trimIndent()
        b.cefBrowser.executeJavaScript(js, b.cefBrowser.url ?: "", 0)
    }

    /** Handles a locale value pushed from the SPA; publishes a change when it differs. */
    private fun onSpaLocale(value: String?) {
        val lang = value?.trim()
        if (lang != "en" && lang != "ru") return
        if (FoxxyCodeLocaleState.update(lang)) {
            ApplicationManager.getApplication().invokeLater {
                ApplicationManager.getApplication().messageBus
                    .syncPublisher(FoxxyCodeLanguageListener.TOPIC)
                    .languageChanged()
            }
        }
    }

    /**
     * Injects compatibility shims into the embedded page:
     *  - polyfills `crypto.randomUUID` (missing in Chromium < 92, e.g. older JCEF runtimes), which
     *    the FoxxyCode SPA calls when creating a chat draft — without it the UI crashes to a blank page;
     *  - renders uncaught JS errors as a visible overlay instead of silently unmounting the app.
     */
    private fun injectBootstrap() {
        val b = browser ?: return
        b.cefBrowser.executeJavaScript(BOOTSTRAP_JS, b.cefBrowser.url ?: "", 0)
    }

    /** Injects JS that aligns the FoxxyCode web UI theme with the current IDE theme. */
    private fun syncTheme() {
        if (!FoxxyCodeSettings.getInstance().state.followIdeTheme) return
        val b = browser ?: return
        val js = FoxxyCodeThemeBridge.applyThemeJs(FoxxyCodeThemeBridge.currentFoxxyCodeTheme())
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

    /** Show a localized status message; [key] is a FoxxyCodeBundle key. */
    private fun showStatusMessage(key: String, vararg params: Any) {
        panelMode = PanelMode.MESSAGE
        statusMessageKey = key
        statusMessageParams = params
        showMessage(FoxxyCodeBundle.message(key, *params))
    }

    private var lastErrorDetail: String? = null

    private fun showError(msg: String) {
        panelMode = PanelMode.ERROR
        lastErrorDetail = msg
        FoxxyCodeNotifications.error(project, FoxxyCodeBundle.message("notification.title.startFailed"), msg)
        val panel = JPanel(BorderLayout())
        panel.add(
            JLabel(FoxxyCodeBundle.message("process.error.startFailedPanel", msg), SwingConstants.CENTER),
            BorderLayout.CENTER
        )
        val south = JPanel()
        south.add(JButton(FoxxyCodeBundle.message("process.button.retry")).apply { addActionListener { start() } })
        south.add(JButton(FoxxyCodeBundle.message("process.button.openSettings")).apply { addActionListener { openSettings() } })
        panel.add(south, BorderLayout.SOUTH)
        setCenter(panel)
    }

    private fun showFallback(url: String) {
        panelMode = PanelMode.FALLBACK
        val panel = JPanel(BorderLayout())
        panel.add(
            JLabel(
                FoxxyCodeBundle.message("process.fallback.jcefUnavailable"),
                SwingConstants.CENTER
            ),
            BorderLayout.CENTER
        )
        val south = JPanel()
        south.add(JButton(FoxxyCodeBundle.message("process.button.openUrl", url)).apply { addActionListener { BrowserUtil.browse(url) } })
        panel.add(south, BorderLayout.SOUTH)
        setCenter(panel)
    }

    private fun openSettings() {
        ShowSettingsUtil.getInstance().showSettingsDialog(project, FoxxyCodeBundle.message("settings.displayName"))
    }

    private fun createToolbar(): JComponent {
        val group = DefaultActionGroup()
        group.add(object : AnAction("", "", AllIcons.Actions.Restart) {
            override fun update(e: AnActionEvent) {
                e.presentation.text = FoxxyCodeBundle.message("toolbar.action.restart")
                e.presentation.description = FoxxyCodeBundle.message("toolbar.action.restart.desc")
            }
            override fun actionPerformed(e: AnActionEvent) {
                showStatusMessage("process.status.restarting")
                FoxxyCodeProcessManager.getInstance(project).restart(
                    onReady = { url ->
                        loadUrl(url)
                        FoxxyCodeIdeDiffService.getInstance(project).startIfNeeded()
                        FoxxyCodeEditorContextService.getInstance(project).startIfNeeded()
                        FoxxyCodeTerminalContextService.getInstance(project).startIfNeeded()
                    },
                    onError = { msg -> showError(msg) }
                )
            }
        })
        group.add(object : AnAction("", "", AllIcons.Actions.Refresh) {
            override fun update(e: AnActionEvent) {
                e.presentation.text = FoxxyCodeBundle.message("toolbar.action.reload")
                e.presentation.description = FoxxyCodeBundle.message("toolbar.action.reload.desc")
            }
            override fun actionPerformed(e: AnActionEvent) {
                val b = browser
                if (b != null) b.cefBrowser.reload() else start()
            }
        })
        group.add(object : AnAction("", "", AllIcons.General.Web) {
            override fun update(e: AnActionEvent) {
                e.presentation.text = FoxxyCodeBundle.message("toolbar.action.openBrowser")
                e.presentation.description = FoxxyCodeBundle.message("toolbar.action.openBrowser.desc")
            }
            override fun actionPerformed(e: AnActionEvent) {
                currentUrl?.let { BrowserUtil.browse(it) }
            }
        })
        group.add(object : AnAction("", "", AllIcons.Debugger.Console) {
            override fun update(e: AnActionEvent) {
                e.presentation.text = FoxxyCodeBundle.message("toolbar.action.devtools")
                e.presentation.description = FoxxyCodeBundle.message("toolbar.action.devtools.desc")
            }
            override fun actionPerformed(e: AnActionEvent) {
                browser?.openDevtools()
            }
        })
        group.add(object : AnAction("", "", AllIcons.General.Settings) {
            override fun update(e: AnActionEvent) {
                e.presentation.text = FoxxyCodeBundle.message("toolbar.action.settings")
                e.presentation.description = FoxxyCodeBundle.message("toolbar.action.settings.desc")
            }
            override fun actionPerformed(e: AnActionEvent) = openSettings()
        })
        val toolbar = ActionManager.getInstance().createActionToolbar("FoxxyCodeToolbar", group, true)
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
                if (!window.__foxxycodeErrOverlayInstalled) {
                  window.__foxxycodeErrOverlayInstalled = true;
                  var show = function (title, detail) {
                    try {
                      var el = document.getElementById("foxxycode-err-overlay");
                      if (!el) {
                        el = document.createElement("div");
                        el.id = "foxxycode-err-overlay";
                        el.style.cssText = "position:fixed;left:0;right:0;bottom:0;z-index:2147483647;max-height:45vh;overflow:auto;background:#7f1d1d;color:#fff;font:12px/1.45 monospace;padding:10px 12px;white-space:pre-wrap;border-top:2px solid #ef4444";
                        (document.body || document.documentElement).appendChild(el);
                      }
                      el.textContent = "FoxxyCode UI error — " + title + "\n" + (detail || "");
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
