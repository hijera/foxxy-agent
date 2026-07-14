package dev.foxxycode.intellij.process

import com.intellij.execution.configurations.GeneralCommandLine
import com.intellij.execution.process.OSProcessHandler
import com.intellij.execution.process.ProcessAdapter
import com.intellij.execution.process.ProcessEvent
import com.intellij.openapi.Disposable
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.logger
import com.intellij.openapi.progress.ProgressIndicator
import com.intellij.openapi.progress.ProgressManager
import com.intellij.openapi.progress.Task
import com.intellij.openapi.project.Project
import com.intellij.openapi.util.Key
import com.intellij.util.execution.ParametersListUtil
import com.google.gson.JsonParser
import dev.foxxycode.intellij.FoxxyCodeBundle
import dev.foxxycode.intellij.FoxxyCodeLocaleState
import dev.foxxycode.intellij.binary.FoxxyCodeBinaryResolver
import dev.foxxycode.intellij.settings.FoxxyCodeSettings
import dev.foxxycode.intellij.ui.FoxxyCodeLanguageListener
import java.net.HttpURLConnection
import java.net.URI

/**
 * Owns the per-project `foxxycode http` subprocess: launch, readiness polling, restart, shutdown.
 * The foxxycode binary is bundled with the plugin and resolved by [FoxxyCodeBinaryResolver].
 * Registered as a projectService in plugin.xml (kept annotation-free for 212 compatibility).
 */
class FoxxyCodeProcessManager(private val project: Project) : Disposable {
    private val log = logger<FoxxyCodeProcessManager>()

    @Volatile
    private var handler: OSProcessHandler? = null

    @Volatile
    var baseUrl: String? = null
        private set

    val isRunning: Boolean
        get() = handler?.let { !it.isProcessTerminated } ?: false

    /** Ensures the server is running and ready, then invokes [onReady]/[onError] on the EDT. */
    fun ensureStarted(onReady: (String) -> Unit, onError: (String) -> Unit) {
        val url = baseUrl
        if (isRunning && url != null) {
            onReady(url)
            return
        }
        ProgressManager.getInstance().run(object : Task.Backgroundable(project, FoxxyCodeBundle.message("process.task.starting"), false) {
            override fun run(indicator: ProgressIndicator) {
                try {
                    val ready = startAndWait(indicator)
                    ApplicationManager.getApplication().invokeLater { onReady(ready) }
                } catch (e: Exception) {
                    log.warn("FoxxyCode failed to start", e)
                    ApplicationManager.getApplication().invokeLater { onError(e.message ?: e.toString()) }
                }
            }
        })
    }

    fun restart(onReady: (String) -> Unit, onError: (String) -> Unit) {
        stop()
        ensureStarted(onReady, onError)
    }

    @Synchronized
    private fun startAndWait(indicator: ProgressIndicator): String {
        baseUrl?.let { if (isRunning) return it }
        stopInternal()

        val settings = FoxxyCodeSettings.getInstance().state
        val binary = FoxxyCodeBinaryResolver.resolveExisting()
            ?: throw IllegalStateException(FoxxyCodeBundle.message("process.error.binaryNotFound"))

        val host = settings.host.ifBlank { "127.0.0.1" }
        val port = PortUtil.pick(settings.fixedPort)

        val cmd = GeneralCommandLine(binary.absolutePath)
            .withParameters("http", "-H", host, "-P", port.toString())
        project.basePath?.let { cmd.addParameters("--cwd", it) }
        if (settings.foxxycodeHome.isNotBlank()) cmd.addParameters("--home", settings.foxxycodeHome)
        if (settings.extraArgs.isNotBlank()) cmd.addParameters(ParametersListUtil.parse(settings.extraArgs))
        val proxy = ProxyEnvironment.resolveProxyEnvironment()
        log.info("[foxxycode] " + ProxyEnvironment.describe(proxy))
        cmd.withEnvironment(proxy.env)
        cmd.withWorkDirectory(project.basePath ?: System.getProperty("user.home"))

        indicator.text = FoxxyCodeBundle.message("process.indicator.launching", host, port.toString())
        val h = OSProcessHandler(cmd)
        h.addProcessListener(object : ProcessAdapter() {
            override fun onTextAvailable(event: ProcessEvent, outputType: Key<*>) {
                log.info("[foxxycode] " + event.text.trimEnd())
            }

            override fun processTerminated(event: ProcessEvent) {
                log.info("[foxxycode] process terminated, exit=${event.exitCode}")
                baseUrl = null
            }
        })
        h.startNotify()
        handler = h

        val url = "http://$host:$port/"
        waitForReady(url, indicator)
        baseUrl = url
        adoptBackendLocale(url)
        log.info("FoxxyCode ready at $url")
        return url
    }

    /**
     * Fetch `ui.locale` from the backend config (the single app-wide language
     * source) and adopt it before the browser panel loads, so its `?lang=` and
     * the plugin chrome agree from the first frame. Best-effort: any failure
     * keeps the current locale. Publishes a language-change notification when
     * the value actually changed so already-open panels re-localize.
     */
    private fun adoptBackendLocale(url: String) {
        val locale = try {
            val conn = URI.create(url + "foxxycode/config").toURL().openConnection() as HttpURLConnection
            conn.connectTimeout = 3000
            conn.readTimeout = 3000
            conn.requestMethod = "GET"
            val code = conn.responseCode
            val body = if (code in 200..299) conn.inputStream.bufferedReader().use { it.readText() } else null
            conn.disconnect()
            body?.let {
                val o = JsonParser.parseString(it).asJsonObject
                val ui = if (o.has("ui") && o.get("ui").isJsonObject) o.getAsJsonObject("ui") else null
                val raw = ui?.let { u -> if (u.has("locale") && !u.get("locale").isJsonNull) u.get("locale").asString else null }
                if (raw == "en" || raw == "ru") raw else null
            }
        } catch (e: Exception) {
            log.info("could not read backend ui.locale: ${e.message}")
            return
        }
        if (FoxxyCodeLocaleState.update(locale)) {
            ApplicationManager.getApplication().invokeLater {
                ApplicationManager.getApplication().messageBus
                    .syncPublisher(FoxxyCodeLanguageListener.TOPIC)
                    .languageChanged()
            }
        }
    }

    private fun waitForReady(url: String, indicator: ProgressIndicator) {
        val probe = url + "v1/models"
        val deadline = System.currentTimeMillis() + 30_000
        var lastError = "timeout"
        while (System.currentTimeMillis() < deadline) {
            if (!isRunning) {
                throw IllegalStateException(FoxxyCodeBundle.message("process.error.exitedBeforeReady"))
            }
            indicator.checkCanceled()
            try {
                val conn = URI.create(probe).toURL().openConnection() as HttpURLConnection
                conn.connectTimeout = 1500
                conn.readTimeout = 1500
                conn.requestMethod = "GET"
                val code = conn.responseCode
                conn.disconnect()
                if (code in 200..499) return // server is accepting requests
            } catch (e: Exception) {
                lastError = e.message ?: e.toString()
            }
            Thread.sleep(300)
        }
        throw IllegalStateException(FoxxyCodeBundle.message("process.error.notReady", lastError))
    }

    @Synchronized
    fun stop() = stopInternal()

    private fun stopInternal() {
        val h = handler
        handler = null
        baseUrl = null
        // destroyProcess() only signals termination; never block here. stopInternal() runs on the
        // EDT during dispose()/plugin unload/restart, where waitFor would freeze the whole IDE.
        if (h != null && !h.isProcessTerminated) {
            try {
                h.destroyProcess()
            } catch (e: Exception) {
                log.warn("Error stopping FoxxyCode", e)
            }
        }
    }

    override fun dispose() = stopInternal()

    companion object {
        fun getInstance(project: Project): FoxxyCodeProcessManager =
            project.getService(FoxxyCodeProcessManager::class.java)
    }
}
