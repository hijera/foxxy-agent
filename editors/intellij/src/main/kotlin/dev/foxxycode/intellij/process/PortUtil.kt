package dev.foxxycode.intellij.process

import java.net.ServerSocket

object PortUtil {
    /** Returns [fixed] when it is a valid port, otherwise a free ephemeral port. */
    fun pick(fixed: Int): Int {
        if (fixed in 1..65535) return fixed
        ServerSocket(0).use { return it.localPort }
    }
}
