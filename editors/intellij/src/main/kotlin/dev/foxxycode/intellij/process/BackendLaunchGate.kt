package dev.foxxycode.intellij.process

import java.util.concurrent.CancellationException

/**
 * Invalidates queued launches and makes process creation atomic with respect to stop.
 *
 * [run] holds the monitor for the whole action and [cancel] needs the same monitor, so a stop
 * arriving while a launch is inside [run] waits until that action finishes - that is the
 * atomicity, not an oversight: the stop then either sees and kills the handler the action
 * registered, or the action never runs because [cancel] won first.
 *
 * Because the panel calls stop from the EDT (the tool window visibility listener), everything
 * wrapped in [run] must stay short and non-blocking: process creation, the destroy signal, and
 * publishing readiness - never a wait. Waiting for termination belongs to `awaitTerminated`,
 * which the caller runs on a pooled thread.
 */
internal class BackendLaunchGate {
    private var generation = Any()
    private var closed = false

    @Synchronized
    fun ticket(): Any = generation

    @Synchronized
    fun isCurrent(ticket: Any): Boolean = !closed && ticket === generation

    @Synchronized
    fun <T> run(ticket: Any, action: () -> T): T {
        if (!isCurrent(ticket)) throw CancellationException("FoxxyCode launch cancelled")
        return action()
    }

    @Synchronized
    fun cancel(close: Boolean = false, stop: () -> Unit) {
        generation = Any()
        closed = closed || close
        stop()
    }
}
