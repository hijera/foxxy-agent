package dev.foxxycode.intellij.process

import com.intellij.execution.process.KillableProcessHandler
import com.intellij.execution.process.OSProcessUtil
import com.intellij.openapi.diagnostic.logger
import com.intellij.openapi.util.ShutDownTracker
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean

/**
 * Every `foxxycode http` process this IDE started, across all open projects.
 *
 * [FoxxyCodeProcessManager] is per project and only knows its own backend, but the IDE closes
 * once for all of them — and on Windows a backend that outlives the IDE keeps its port and, if
 * it were still running from the plugin directory, would block the next plugin update. This
 * registry is what the app-close and plugin-unload hooks reap through.
 */
object FoxxyCodeBackends {
    private val LOG = logger<FoxxyCodeBackends>()

    /** Default ceiling for [stopAll]; shared across all backends, not spent per backend. */
    const val STOP_BUDGET_MS = 2_000L

    private val live = CopyOnWriteArrayList<KillableProcessHandler>()
    private val shutdownTaskInstalled = AtomicBoolean(false)

    fun register(handler: KillableProcessHandler) {
        live.addIfAbsent(handler)
        // The last-resort net: ShutDownTracker fires on shutdown paths that never reach
        // AppLifecycleListener, and it runs off the EDT.
        if (shutdownTaskInstalled.compareAndSet(false, true)) {
            ShutDownTracker.getInstance().registerShutdownTask { killAllNow() }
        }
    }

    fun unregister(handler: KillableProcessHandler) {
        live.remove(handler)
    }

    /** Signals every backend, then waits within one shared [budgetMs], killing stragglers. */
    fun stopAll(budgetMs: Long = STOP_BUDGET_MS) {
        val targets = live.toList().filter { !it.isProcessTerminated }
        if (targets.isEmpty()) return
        val survivors = ProcessReaper.reapAll(targets.map { asKillable(it) }, budgetMs)
        if (survivors > 0) LOG.warn("$survivors FoxxyCode backend(s) still alive after ${budgetMs}ms")
    }

    /** Kills every backend and its children without waiting. For the shutdown hook only. */
    fun killAllNow() {
        for (handler in live.toList()) {
            if (handler.isProcessTerminated) continue
            try {
                OSProcessUtil.killProcessTree(handler.process)
            } catch (e: Exception) {
                LOG.warn("Could not kill the FoxxyCode backend on shutdown", e)
            }
        }
    }

    /** Adapts a process handler to the reaper's view of it. */
    fun asKillable(handler: KillableProcessHandler): ProcessReaper.Killable =
        object : ProcessReaper.Killable {
            override fun signal() {
                if (!handler.isProcessTerminated) handler.destroyProcess()
            }

            // Waits on the OS process rather than ProcessHandler.waitFor(), which asserts it is
            // off the EDT and logs a SEVERE naming this plugin. Blocking here IS on the EDT, by
            // design (see FoxxyCodeBackendLifecycleListener), and all we need is the exit - not
            // the handler's output-reader bookkeeping.
            override fun awaitExit(ms: Long): Boolean {
                if (handler.isProcessTerminated) return true
                return try {
                    handler.process.waitFor(ms, TimeUnit.MILLISECONDS)
                } catch (e: InterruptedException) {
                    Thread.currentThread().interrupt()
                    !handler.process.isAlive
                }
            }

            override fun killTree(): Boolean =
                try {
                    OSProcessUtil.killProcessTree(handler.process)
                } catch (e: Exception) {
                    LOG.warn("Could not kill the FoxxyCode process tree", e)
                    false
                }
        }
}
