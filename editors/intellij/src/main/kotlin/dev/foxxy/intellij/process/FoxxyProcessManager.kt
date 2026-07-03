package dev.foxxy.intellij.process

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
import dev.foxxy.intellij.FoxxyBundle
import dev.foxxy.intellij.binary.FoxxyBinaryResolver
import dev.foxxy.intellij.settings.FoxxySettings
import java.net.HttpURLConnection
import java.net.URI

/**
 * Owns the per-project `foxxy http` subprocess: launch, readiness polling, restart, shutdown.
 * The foxxy binary is bundled with the plugin and resolved by [FoxxyBinaryResolver].
 * Registered as a projectService in plugin.xml (kept annotation-free for 212 compatibility).
 */
class FoxxyProcessManager(private val project: Project) : Disposable {
    private val log = logger<FoxxyProcessManager>()

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
        ProgressManager.getInstance().run(object : Task.Backgroundable(project, FoxxyBundle.message("process.task.starting"), false) {
            override fun run(indicator: ProgressIndicator) {
                try {
                    val ready = startAndWait(indicator)
                    ApplicationManager.getApplication().invokeLater { onReady(ready) }
                } catch (e: Exception) {
                    log.warn("Foxxy failed to start", e)
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

        val settings = FoxxySettings.getInstance().state
        val binary = FoxxyBinaryResolver.resolveExisting()
            ?: throw IllegalStateException(FoxxyBundle.message("process.error.binaryNotFound"))

        val host = settings.host.ifBlank { "127.0.0.1" }
        val port = PortUtil.pick(settings.fixedPort)

        val cmd = GeneralCommandLine(binary.absolutePath)
            .withParameters("http", "-H", host, "-P", port.toString())
        project.basePath?.let { cmd.addParameters("--cwd", it) }
        if (settings.foxxyHome.isNotBlank()) cmd.addParameters("--home", settings.foxxyHome)
        if (settings.extraArgs.isNotBlank()) cmd.addParameters(ParametersListUtil.parse(settings.extraArgs))
        cmd.withWorkDirectory(project.basePath ?: System.getProperty("user.home"))

        indicator.text = FoxxyBundle.message("process.indicator.launching", host, port.toString())
        val h = OSProcessHandler(cmd)
        h.addProcessListener(object : ProcessAdapter() {
            override fun onTextAvailable(event: ProcessEvent, outputType: Key<*>) {
                log.info("[foxxy] " + event.text.trimEnd())
            }

            override fun processTerminated(event: ProcessEvent) {
                log.info("[foxxy] process terminated, exit=${event.exitCode}")
                baseUrl = null
            }
        })
        h.startNotify()
        handler = h

        val url = "http://$host:$port/"
        waitForReady(url, indicator)
        baseUrl = url
        log.info("Foxxy ready at $url")
        return url
    }

    private fun waitForReady(url: String, indicator: ProgressIndicator) {
        val probe = url + "v1/models"
        val deadline = System.currentTimeMillis() + 30_000
        var lastError = "timeout"
        while (System.currentTimeMillis() < deadline) {
            if (!isRunning) {
                throw IllegalStateException(FoxxyBundle.message("process.error.exitedBeforeReady"))
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
        throw IllegalStateException(FoxxyBundle.message("process.error.notReady", lastError))
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
                log.warn("Error stopping Foxxy", e)
            }
        }
    }

    override fun dispose() = stopInternal()

    companion object {
        fun getInstance(project: Project): FoxxyProcessManager =
            project.getService(FoxxyProcessManager::class.java)
    }
}
