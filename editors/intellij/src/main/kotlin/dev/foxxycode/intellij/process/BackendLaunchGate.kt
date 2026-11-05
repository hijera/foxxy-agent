package dev.foxxycode.intellij.process

import java.util.concurrent.CancellationException

/** Invalidates queued launches and makes process creation atomic with respect to stop. */
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
