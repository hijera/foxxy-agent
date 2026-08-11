package dev.foxxycode.intellij.process

import com.intellij.execution.configurations.GeneralCommandLine
import com.intellij.execution.process.KillableProcessHandler
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
import com.intellij.util.concurrency.annotations.RequiresBackgroundThread
import com.intellij.util.io.BaseOutputReader
import com.intellij.util.execution.ParametersListUtil
import com.google.gson.JsonParser
import dev.foxxycode.intellij.FoxxyCodeBundle
import dev.foxxycode.intellij.FoxxyCodeLocaleState
import dev.foxxycode.intellij.binary.FoxxyCodeBinaryResolver
import dev.foxxycode.intellij.settings.FoxxyCodeSettings
import dev.foxxycode.intellij.ui.FoxxyCodeLanguageListener
import java.net.HttpURLConnection
import java.net.URI
import java.util.concurrent.CopyOnWriteArrayList

/**
 * Owns the per-project `foxxycode http` subprocess: launch, readiness polling, restart, shutdown.
 * The foxxycode binary is bundled with the plugin and resolved by [FoxxyCodeBinaryResolver].
 * Registered as a projectService in plugin.xml (kept annotation-free for 212 compatibility).
 */
class FoxxyCodeProcessManager(private val project: Project) : Disposable {
    private val log = logger<FoxxyCodeProcessManager>()

    @Volatile
    private var handler: KillableProcessHandler? = null

    /** Backends already told to die, whose exit the next launch waits for. */
    private val terminating = CopyOnWriteArrayList<KillableProcessHandler>()

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
        // Signal only: the blocking half runs inside the background task startAndWait owns.
        signalStop()
        ensureStarted(onReady, onError)
    }

    @Synchronized
    private fun startAndWait(indicator: ProgressIndicator): String {
        baseUrl?.let { if (isRunning) return it }
        signalStop()
        // A backend still holding the port would make the fresh one die on bind, and on Windows
        // one still holding its image file would block the next plugin update. This is the
        // background task, so waiting here is free.
        awaitTerminated(RESTART_WAIT_MS)

        val settings = FoxxyCodeSettings.getInstance().state
        // The first launch after a plugin update copies the binary out of the plugin directory,
        // which takes a moment; say so rather than looking stuck.
        indicator.text = FoxxyCodeBundle.message("process.indicator.staging")
        val binary = FoxxyCodeBinaryResolver.resolveForLaunch()
            ?: throw IllegalStateException(FoxxyCodeBundle.message("process.error.binaryNotFound"))

        val host = settings.host.ifBlank { "127.0.0.1" }
        val port = PortUtil.pick(settings.fixedPort)
        // A fixed port can simply be taken - by another IDE window (the port setting is
        // application-wide while this service is per project) or by the previous backend that
        // a plugin update is still shutting down. Waiting covers the second case; the first
        // one has to be told to the user, because the process would otherwise die on bind and
        // surface as the generic "exited before becoming ready".
        if (settings.fixedPort in 1..65535 && !PortUtil.awaitAvailable(port, host)) {
            throw IllegalStateException(FoxxyCodeBundle.message("process.error.portInUse", port.toString()))
        }

        val cmd = GeneralCommandLine(binary.absolutePath)
            .withParameters("http", "-H", host, "-P", port.toString())
        project.basePath?.let { cmd.addParameters("--cwd", it) }
        if (settings.foxxycodeHome.isNotBlank()) cmd.addParameters("--home", settings.foxxycodeHome)
        // Panels default to guarded planning: the model may not leave plan mode itself.
        cmd.addParameters("--plan-no-self-run=" + settings.planNoSelfRun)
        if (settings.extraArgs.isNotBlank()) cmd.addParameters(ParametersListUtil.parse(settings.extraArgs))
        val proxy = ProxyEnvironment.resolveProxyEnvironment()
        log.info("[foxxycode] " + ProxyEnvironment.describe(proxy))
        cmd.withEnvironment(proxy.env)
        cmd.withWorkDirectory(project.basePath ?: System.getProperty("user.home"))

        indicator.text = FoxxyCodeBundle.message("process.indicator.launching", host, port.toString())
        recentOutput.clear()
        // The backend is a long-running server that prints almost nothing after startup. The
        // default reader polls it as if output were imminent, which the IDE itself warns about
        // ("Process hasn't generated any output for a long time") and which costs CPU for
        // nothing.
        val h = object : KillableProcessHandler(cmd) {
            override fun readerOptions(): BaseOutputReader.Options =
                BaseOutputReader.Options.forMostlySilentProcess()
        }
        // `foxxycode http` blocks in ListenAndServe with no signal handling, so a soft kill would
        // be reported as delivered and the hard one skipped - leaving the backend alive. With it
        // off, destroyProcess() goes straight to the recursive kill, which also takes down the
        // shells and MCP servers the backend spawned.
        h.setShouldKillProcessSoftly(false)
        h.addProcessListener(object : ProcessAdapter() {
            override fun onTextAvailable(event: ProcessEvent, outputType: Key<*>) {
                val line = event.text.trimEnd()
                log.info("[foxxycode] " + line)
                rememberOutput(line)
            }

            override fun processTerminated(event: ProcessEvent) {
                log.info("[foxxycode] process terminated, exit=${event.exitCode}")
                baseUrl = null
                terminating.remove(h)
                FoxxyCodeBackends.unregister(h)
            }
        })
        h.startNotify()
        handler = h
        FoxxyCodeBackends.register(h)

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

    /** Last lines the backend printed, kept so a startup failure can quote them to the user. */
    private val recentOutput = ArrayDeque<String>()

    private fun rememberOutput(line: String) {
        if (line.isBlank()) return
        synchronized(recentOutput) {
            recentOutput.addLast(line)
            while (recentOutput.size > MAX_REMEMBERED_OUTPUT) recentOutput.removeFirst()
        }
    }

    /** The remembered backend output as one block, or "" when it printed nothing. */
    private fun recentOutputText(): String = synchronized(recentOutput) { recentOutput.joinToString("\n") }

    private fun waitForReady(url: String, indicator: ProgressIndicator) {
        val probe = url + "v1/models"
        val deadline = System.currentTimeMillis() + 30_000
        var lastError = "timeout"
        while (System.currentTimeMillis() < deadline) {
            if (!isRunning) {
                // The backend already said why it died - "bind: address already in use", a bad
                // config key, a missing provider. Quoting it beats the generic guess.
                val detail = recentOutputText()
                val base = FoxxyCodeBundle.message("process.error.exitedBeforeReady")
                throw IllegalStateException(if (detail.isEmpty()) base else "$base\n\n$detail")
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

    /**
     * Tells the backend to terminate and forgets it. Never blocks: this runs on the EDT during
     * dispose()/plugin unload/restart, where waiting would freeze the whole IDE. The wait lives
     * in [awaitTerminated], which every caller schedules off the EDT.
     *
     * Not `@Synchronized` on purpose: `startAndWait` holds that lock for as long as the readiness
     * probe runs, and dispose()/restart call this from the EDT.
     */
    private fun signalStop() {
        val h = handler
        handler = null
        baseUrl = null
        if (h != null && !h.isProcessTerminated) {
            terminating.addIfAbsent(h)
            try {
                h.destroyProcess()
            } catch (e: Exception) {
                log.warn("Error stopping FoxxyCode", e)
            }
        }
    }

    /**
     * Waits for everything [signalStop] signalled to actually exit, killing whatever is left
     * along with its children. Blocking; never call from the EDT.
     */
    @RequiresBackgroundThread
    private fun awaitTerminated(budgetMs: Long) {
        val pending = terminating.toList()
        if (pending.isEmpty()) return
        val survivors = ProcessReaper.reapAll(pending.map { FoxxyCodeBackends.asKillable(it) }, budgetMs)
        if (survivors > 0) log.warn("$survivors FoxxyCode backend(s) still alive after ${budgetMs}ms")
        terminating.removeAll(pending)
        for (h in pending) FoxxyCodeBackends.unregister(h)
    }

    override fun dispose() {
        signalStop()
        // Project close and plugin unload both land here on the EDT, so the wait moves to a
        // pooled thread - but it has to happen: an unwaited backend outlives the IDE on Windows.
        ApplicationManager.getApplication().executeOnPooledThread { awaitTerminated(DISPOSE_WAIT_MS) }
    }

    companion object {
        /** Enough backend output to carry a stack-free error message, not enough to flood a balloon. */
        private const val MAX_REMEMBERED_OUTPUT = 20

        /** How long a relaunch waits for the outgoing backend to release its port. */
        private const val RESTART_WAIT_MS = 2_500L

        /** How long project close waits for the backend, on a pooled thread. */
        private const val DISPOSE_WAIT_MS = 3_000L

        fun getInstance(project: Project): FoxxyCodeProcessManager =
            project.getService(FoxxyCodeProcessManager::class.java)
    }
}
