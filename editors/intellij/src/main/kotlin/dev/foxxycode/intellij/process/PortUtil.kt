package dev.foxxycode.intellij.process

import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.ServerSocket

object PortUtil {
    /** How long a fixed port is given to come free before the launch is reported as blocked. */
    const val FIXED_PORT_WAIT_MS = 3_000L

    /** Poll interval while waiting for a fixed port to come free. */
    private const val RETRY_INTERVAL_MS = 500L

    /** Returns [fixed] when it is a valid port, otherwise a free ephemeral port. */
    fun pick(fixed: Int): Int {
        if (fixed in 1..65535) return fixed
        ServerSocket(0).use { return it.localPort }
    }

    /** True when nothing is currently listening on [port] of [host]. */
    fun isAvailable(port: Int, host: String): Boolean =
        try {
            // reuseAddress stays off on purpose: with it on, a socket in TIME_WAIT would bind
            // here and report "free" while the old server still owns the listening socket.
            ServerSocket().use { s ->
                s.reuseAddress = false
                s.bind(InetSocketAddress(resolveHost(host), port))
                true
            }
        } catch (e: Exception) {
            false
        }

    /**
     * Waits up to [waitMs] for [port] to come free and reports whether it did. Installing a new
     * plugin build restarts the backend, and the outgoing process keeps the port for a moment
     * after `destroyProcess()` signals it — without this wait the fresh one loses a race it
     * would win a second later.
     */
    fun awaitAvailable(
        port: Int,
        host: String,
        waitMs: Long = FIXED_PORT_WAIT_MS,
        sleep: (Long) -> Unit = { Thread.sleep(it) },
    ): Boolean {
        val deadline = System.currentTimeMillis() + waitMs
        while (true) {
            if (isAvailable(port, host)) return true
            if (System.currentTimeMillis() >= deadline) return false
            sleep(RETRY_INTERVAL_MS)
        }
    }

    /** Loopback for the wildcard/empty host, so probing never needs an external interface. */
    private fun resolveHost(host: String): InetAddress {
        val h = host.trim()
        if (h.isEmpty() || h == "0.0.0.0" || h == "::") return InetAddress.getLoopbackAddress()
        return InetAddress.getByName(h)
    }
}
