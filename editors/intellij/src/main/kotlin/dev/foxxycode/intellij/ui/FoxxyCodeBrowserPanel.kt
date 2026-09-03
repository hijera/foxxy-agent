package dev.foxxycode.intellij.ui

import com.google.gson.JsonPrimitive
import com.intellij.icons.AllIcons
import com.intellij.ide.BrowserUtil
import com.intellij.ide.dnd.DnDDropHandler
import com.intellij.ide.dnd.DnDEvent
import com.intellij.ide.dnd.DnDNativeTarget
import com.intellij.ide.dnd.DnDSupport
import com.intellij.ide.dnd.DnDTargetChecker
import com.intellij.ide.dnd.TransferableWrapper
import com.intellij.ide.plugins.PluginManagerCore
import com.intellij.ide.ui.LafManagerListener
import com.intellij.openapi.Disposable
import com.intellij.openapi.actionSystem.ActionManager
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.actionSystem.DefaultActionGroup
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.logger
import com.intellij.openapi.extensions.PluginId
import com.intellij.openapi.options.ShowSettingsUtil
import com.intellij.openapi.project.Project
import com.intellij.openapi.util.Disposer
import com.intellij.openapi.vfs.VfsUtilCore
import com.intellij.openapi.vfs.VirtualFile
import com.intellij.ui.docking.DockableContent
import com.intellij.ui.jcef.JBCefApp
import com.intellij.ui.jcef.JBCefBrowser
import com.intellij.ui.jcef.JBCefBrowserBase
import com.intellij.ui.jcef.JBCefJSQuery
import com.intellij.util.ui.JBUI
import com.intellij.util.ui.UIUtil
import dev.foxxycode.intellij.FoxxyCodeBundle
import dev.foxxycode.intellij.FoxxyCodeLocaleState
import dev.foxxycode.intellij.FoxxyCodeNotifications
import dev.foxxycode.intellij.autocomplete.FoxxyCodeAutocompleteService
import dev.foxxycode.intellij.diff.FoxxyCodeIdeDiffService
import dev.foxxycode.intellij.editor.FoxxyCodeEditorContextService
import dev.foxxycode.intellij.process.FoxxyCodeProcessManager
import dev.foxxycode.intellij.terminal.FoxxyCodeTerminalContextService
import dev.foxxycode.intellij.settings.FoxxyCodeSettings
import org.cef.browser.CefBrowser
import org.cef.callback.CefDragData
import org.cef.handler.CefDragHandler
import org.cef.browser.CefFrame
import org.cef.callback.CefBeforeDownloadCallback
import org.cef.callback.CefDownloadItem
import org.cef.handler.CefDownloadHandlerAdapter
import org.cef.handler.CefLoadHandlerAdapter
import java.awt.BorderLayout
import java.awt.datatransfer.DataFlavor
import java.io.File
import java.util.Vector
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

    /**
     * True once the SPA page has loaded, so JS calls will land.
     *
     * Cleared **only** where this panel itself navigates the browser (see [markPageNavigating]),
     * never from CEF's loading-state callbacks. A live SPA changes loading state on its own all
     * the time, and a flag cleared by those events but restored only by a full main-frame load
     * stays false forever - which silently sent every mention to the queue and left it there.
     */
    @Volatile
    private var browserReady = false

    /** Clears readiness right before this panel loads or reloads the page. */
    private fun markPageNavigating() {
        browserReady = false
    }

    /** File mentions requested before the page was ready; flushed on first load end. */
    private val pendingMentions = ArrayList<String>()

    @Volatile
    private var currentUrl: String? = null

    /** Last status/error message key for re-localization on language change. */
    private var statusMessageKey: String? = null
    private var statusMessageParams: Array<out Any> = emptyArray()
    private enum class PanelMode { NONE, MESSAGE, ERROR, FALLBACK, BROWSER }
    private var panelMode = PanelMode.NONE

    init {
        panelsByProject[project] = this
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
                // Pick up the backend's autocomplete settings; inline suggestions stay off
                // until config.autocomplete enables them.
                FoxxyCodeAutocompleteService.getInstance(project).startIfNeeded()
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
            // JS→Kotlin channel for "a file was just dropped on the page"; see onHostDrop.
            val drop = JBCefJSQuery.create(it as JBCefBrowserBase)
            drop.addHandler { _ -> onHostDrop(); null }
            Disposer.register(it, drop)
            dropQuery = drop
            installBrowserDragHandler(it)
            // CEF drops every download nobody claims, which is why a plain
            // `<a download>` in the panel used to do nothing at all. Claiming it
            // with showDialog=true hands the user the IDE's native Save As box.
            it.jbCefClient.addDownloadHandler(object : CefDownloadHandlerAdapter() {
                override fun onBeforeDownload(
                    browser: CefBrowser?,
                    item: CefDownloadItem?,
                    suggestedName: String?,
                    callback: CefBeforeDownloadCallback?,
                ) {
                    // Empty path + showDialog: CEF picks the download directory and
                    // opens the save dialog seeded with the suggested name.
                    callback?.Continue(suggestedName ?: "", true)
                }
            }, it.cefBrowser)
            // After each page load: install compatibility shims/error overlay, then sync theme + locale.
            it.jbCefClient.addLoadHandler(object : CefLoadHandlerAdapter() {
                override fun onLoadEnd(b: CefBrowser?, frame: CefFrame?, httpStatusCode: Int) {
                    if (frame?.isMain == true) {
                        injectBootstrap()
                        injectLocaleBridge()
                        injectDropBridge()
                        syncTheme()
                        syncLocale()
                        browserReady = true
                        flushPendingMentions()
                    }
                }
            }, it.cefBrowser)
        }
        markPageNavigating()
        b.loadURL(finalUrl)
        setCenter(b.component)
        installFileDropTarget(b.component)
        panelMode = PanelMode.BROWSER
    }

    /** Components that already carry the composer drop target, so it is installed once each. */
    private val fileDropTargets = HashSet<JComponent>()

    override fun addNotify() {
        super.addNotify()
        // The panel itself is a drop target, not just the browser surface: a drag that lands
        // while the server is still starting (the "Starting FoxxyCode…" label) or while an
        // error panel is shown has no browser component to hit. Registered here rather than
        // in the constructor because DnDSupport needs a Disposable already in the Disposer
        // tree, and the tool window content only becomes this panel's disposer afterwards.
        installFileDropTarget(this)
    }

    /**
     * Accepts file drags from the Project view **and the editor tab strip** onto the panel's
     * own chrome — the toolbar row, the status and error cards, the border.
     *
     * It deliberately does **not** cover the chat itself. JCEF renders into a native child
     * window stacked above every Swing component here, so a drag over the chat is delivered to
     * the browser, not to any target registered on this side (verified in a sandbox IDE: the
     * targets install, and no drag event ever arrives). The chat's drops are therefore handled
     * where they actually land — as HTML5 drops in the SPA, see the document-level drop
     * listener in `Composer.tsx`. Both paths end at the same `@`-mention.
     */
    private fun installFileDropTarget(component: JComponent) {
        if (!fileDropTargets.add(component)) return
        val where = component.javaClass.simpleName
        LOG.info("FoxxyCode DnD: drop target installed on $where")
        DnDSupport.createBuilder(component)
            .setTargetChecker(
                DnDTargetChecker { event ->
                    val rels = droppedProjectFiles(event)
                    logDragGesture(where, event, rels.size)
                    if (rels.isNotEmpty()) {
                        event.isDropPossible = true
                        true
                    } else {
                        false
                    }
                },
            )
            .setDropHandler(
                DnDDropHandler { event ->
                    val rels = droppedProjectFiles(event)
                    LOG.info(
                        "FoxxyCode DnD drop #$dnDGesture on $where: ${rels.size} file(s)" +
                            ", browserReady=$browserReady, paths=$rels",
                    )
                    requestInsertFileMentions(rels)
                },
            )
            .setDisposableParent(this)
            .install()
    }

    /** JS→Kotlin channel telling us the page received a drop (see [installBrowserDragHandler]). */
    private var dropQuery: JBCefJSQuery? = null

    /** Absolute paths of the files the current drag carries, as reported by CEF. */
    @Volatile
    private var draggedNativeFiles: List<String> = emptyList()

    /**
     * Reads the **absolute paths** of a file drag out of CEF.
     *
     * Dropping onto the chat is an ordinary HTML5 drop — JCEF renders into a native child
     * window, so no Swing drop target on this side ever sees it. But the page cannot resolve
     * the paths either: a real OS file drop arrives as `DataTransfer.files`, whose `File`
     * objects deliberately expose only a base name, and Chromium leaves `text/uri-list` empty
     * for security. CEF, sitting below that boundary, still has the full paths — this is the
     * only place they are available.
     *
     * The drop itself is then signalled back from the page ([injectDropBridge]), because
     * `CefDragHandler` has no drop callback at all: it only reports what a drag entered with.
     */
    private fun installBrowserDragHandler(browser: JBCefBrowser) {
        browser.jbCefClient.addDragHandler(
            object : CefDragHandler {
                override fun onDragEnter(b: CefBrowser?, dragData: CefDragData?, mask: Int): Boolean {
                    val names = Vector<String>()
                    if (dragData != null && dragData.isFile) {
                        dragData.getFileNames(names)
                    }
                    draggedNativeFiles = names.toList()
                    LOG.info("FoxxyCode DnD: browser drag entered with ${names.size} file(s)")
                    // false = let the drag proceed; the page still decides what to do with it.
                    return false
                }
            },
            browser.cefBrowser,
        )
    }

    /**
     * Adds a capture-phase `drop` listener that pings [onHostDrop]. Capture phase so it runs
     * whatever the SPA does with the event, and independently of it: the SPA's own handler is
     * what stops the browser from navigating to the dropped file, this one only reports that a
     * drop happened so the paths CEF captured can be turned into mentions.
     */
    private fun injectDropBridge() {
        val b = browser ?: return
        val query = dropQuery ?: return
        val js = """
            (function(){ try {
              if (!window.__foxxycodeDropBridge) {
                window.__foxxycodeDropBridge = true;
                document.addEventListener('drop', function () { ${query.inject("''")} }, true);
              }
            } catch (e) {} })();
        """.trimIndent()
        b.cefBrowser.executeJavaScript(js, b.cefBrowser.url ?: "", 0)
    }

    /** Turns the paths captured on drag-enter into composer mentions, once per drop. */
    private fun onHostDrop() {
        val files = draggedNativeFiles
        draggedNativeFiles = emptyList()
        if (files.isEmpty()) return
        val rels = ProjectRelativePaths.relativize(project.basePath, files.map { File(it) })
        LOG.info("FoxxyCode DnD browser drop: ${files.size} file(s), paths=$rels")
        requestInsertFileMentions(rels)
    }

    /** Counts drag gestures over the panel, so a log line can be tied to one attempt. */
    private var dnDGesture = 0

    /** When the target checker last fired; a gap means the user started a new drag. */
    private var lastCheckerAtMs = 0L

    /**
     * Logs one line per drag **gesture** — not per mouse move, and not once per payload class:
     * the question these logs answer is "did the first attempt reach the plugin at all", which
     * a per-class guard would hide the moment the second attempt carries the same payload.
     *
     * Editor-tab / Project-view drags are intra-IDE, and the heavyweight JCEF surface can
     * swallow the mouse tracking `DnDManager` relies on, so whether the checker fires is the
     * first thing worth knowing when a drop does nothing.
     */
    private fun logDragGesture(where: String, event: DnDEvent, files: Int) {
        val now = System.currentTimeMillis()
        val newGesture = now - lastCheckerAtMs > NEW_DRAG_GESTURE_GAP_MS
        lastCheckerAtMs = now
        if (!newGesture) return
        dnDGesture++
        val cls = event.attachedObject?.javaClass?.name ?: "null"
        LOG.info("FoxxyCode DnD drag #$dnDGesture over $where: attachedObject=$cls, files=$files")
    }

    /** Project-relative POSIX paths for files carried by a DnD event (empty when none). */
    private fun droppedProjectFiles(event: DnDEvent): List<String> {
        val files = draggedFiles(event.attachedObject)
        if (files.isEmpty()) return emptyList()
        return ProjectRelativePaths.relativize(project.basePath, files)
    }

    /**
     * Files carried by a drag, from every source the panel accepts:
     *  - [TransferableWrapper] — Project view / any tree selection;
     *  - [DockableContent] — an **editor tab** drag, whose key is the tab's VirtualFile;
     *  - [DnDNativeTarget.EventInfo] — native OS drags exposing `javaFileListFlavor`.
     * Anything else yields an empty list so unrelated drags pass through untouched.
     */
    @Suppress("UNCHECKED_CAST")
    private fun draggedFiles(attached: Any?): List<File> = try {
        when (attached) {
            is TransferableWrapper -> attached.asFileList().orEmpty()
            is DockableContent<*> -> {
                val vf = attached.key as? VirtualFile
                if (vf != null && vf.isInLocalFileSystem) listOf(VfsUtilCore.virtualToIoFile(vf))
                else emptyList()
            }
            is DnDNativeTarget.EventInfo -> {
                val t = attached.transferable
                if (t != null && t.isDataFlavorSupported(DataFlavor.javaFileListFlavor)) {
                    (t.getTransferData(DataFlavor.javaFileListFlavor) as? List<File>).orEmpty()
                } else {
                    emptyList()
                }
            }
            else -> emptyList()
        }
    } catch (e: Exception) {
        // A drag we cannot read is simply not ours.
        emptyList()
    }

    /**
     * Inserts one or more workspace-relative paths into the composer as `@`-mentions.
     * Entry point for both the **Add to FoxxyCode** action (right-click editor tab / file)
     * and the drop handler. Paths requested before the page has loaded are queued and
     * flushed on load end — without that, the first drop after the tool window opens
     * reaches a page whose `window.foxxycodeUi` does not exist yet and is lost silently.
     */
    fun requestInsertFileMentions(paths: List<String>) {
        if (paths.isEmpty()) return
        ApplicationManager.getApplication().invokeLater {
            if (!browserReady || browser == null) {
                LOG.info("FoxxyCode mention: queued $paths (page not ready)")
                pendingMentions.addAll(paths)
                return@invokeLater
            }
            paths.forEach { insertFileMention(it) }
        }
    }

    private fun flushPendingMentions() {
        if (pendingMentions.isEmpty()) return
        val queued = pendingMentions.toList()
        pendingMentions.clear()
        LOG.info("FoxxyCode mention: flushing ${queued.size} queued path(s)")
        queued.forEach { insertFileMention(it) }
    }

    /** Pushes a workspace-relative path into the SPA composer as an `@`-mention. */
    private fun insertFileMention(relPath: String) {
        LOG.info("FoxxyCode mention: pushing to composer: $relPath")
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
        FoxxyCodeNotifications.error(
            project,
            FoxxyCodeBundle.message("notification.title.startFailed"),
            asHtmlBody(msg),
        )
        val panel = JPanel(BorderLayout())
        panel.add(
            JLabel(
                FoxxyCodeBundle.message("process.error.startFailedPanel", asHtmlBody(msg)),
                SwingConstants.CENTER,
            ),
            BorderLayout.CENTER
        )
        val south = JPanel()
        south.add(JButton(FoxxyCodeBundle.message("process.button.retry")).apply { addActionListener { start() } })
        south.add(JButton(FoxxyCodeBundle.message("process.button.openSettings")).apply { addActionListener { openSettings() } })
        panel.add(south, BorderLayout.SOUTH)
        setCenter(panel)
    }

    /**
     * Both the error panel and the notification balloon render HTML, while the message can now
     * carry quoted backend output: escape it, and keep its line breaks visible.
     */
    private fun asHtmlBody(text: String): String =
        text
            .replace("&", "&amp;")
            .replace("<", "&lt;")
            .replace(">", "&gt;")
            .replace("\n", "<br/>")

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
                        FoxxyCodeAutocompleteService.getInstance(project).startIfNeeded()
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
                if (b != null) {
                    markPageNavigating()
                    b.cefBrowser.reload()
                } else {
                    start()
                }
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

        // "FoxxyCode <version>" on the same row as the buttons, so a bug report can be
        // pinned to a build without digging through Settings | Plugins.
        val row = JPanel(BorderLayout())
        row.isOpaque = false
        row.add(toolbar.component, BorderLayout.WEST)
        row.add(versionLabel(), BorderLayout.EAST)
        return row
    }

    private fun versionLabel(): JComponent {
        val version = pluginVersion()
        val label = JLabel(if (version.isEmpty()) PRODUCT_NAME else "$PRODUCT_NAME  $version")
        label.foreground = UIUtil.getContextHelpForeground()
        label.border = JBUI.Borders.emptyRight(8)
        label.toolTipText = if (version.isEmpty()) PLUGIN_ID else "$PLUGIN_ID $version"
        return label
    }

    /** Version from the installed plugin descriptor, or "" when it cannot be resolved. */
    private fun pluginVersion(): String =
        try {
            PluginManagerCore.getPlugin(PluginId.getId(PLUGIN_ID))?.version.orEmpty()
        } catch (e: Exception) {
            ""
        }

    override fun dispose() {
        panelsByProject.remove(project, this)
        // JBCefBrowser is released via its Disposer registration on this panel.
    }

    companion object {
        private val LOG = logger<FoxxyCodeBrowserPanel>()

        /** Must match `<id>` in META-INF/plugin.xml (asserted by plugin_build_test.go). */
        const val PLUGIN_ID = "dev.foxxycode.intellij"

        /**
         * Quiet time that separates two drag gestures in the log. The target checker fires on
         * every mouse move, so gestures are told apart by the pause between them rather than by
         * any event the framework offers.
         */
        private const val NEW_DRAG_GESTURE_GAP_MS = 700L
        private const val PRODUCT_NAME = "FoxxyCode"

        /** Live panels by project, so an action can reach the composer of the right window. */
        private val panelsByProject = java.util.concurrent.ConcurrentHashMap<Project, FoxxyCodeBrowserPanel>()

        /** The panel hosting the FoxxyCode tool window for [project], if it is open. */
        fun forProject(project: Project): FoxxyCodeBrowserPanel? = panelsByProject[project]

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
