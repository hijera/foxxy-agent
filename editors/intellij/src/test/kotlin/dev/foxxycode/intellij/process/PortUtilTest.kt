package dev.foxxycode.intellij.process

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.ServerSocket

/**
 * Unit tests for the port probing that stands between plugin settings and the backend launch.
 * Everything binds on loopback ephemeral ports, so the tests need no fixed port to be free.
 */
class PortUtilTest {

    private fun freePort(): Int = ServerSocket(0).use { it.localPort }

    private fun holdPort(): ServerSocket =
        ServerSocket().apply {
            reuseAddress = false
            bind(InetSocketAddress(InetAddress.getLoopbackAddress(), 0))
        }

    @Test
    fun `pick returns the configured port when it is valid`() {
        assertEquals(8123, PortUtil.pick(8123))
        assertEquals(65535, PortUtil.pick(65535))
    }

    @Test
    fun `pick falls back to an ephemeral port for the auto sentinel`() {
        assertTrue(PortUtil.pick(0) in 1..65535)
        assertTrue(PortUtil.pick(-1) in 1..65535)
        assertTrue(PortUtil.pick(70000) in 1..65535)
    }

    @Test
    fun `an unused port reads as available`() {
        assertTrue(PortUtil.isAvailable(freePort(), "127.0.0.1"))
    }

    @Test
    fun `a port someone is listening on reads as taken`() {
        holdPort().use { held ->
            assertFalse(PortUtil.isAvailable(held.localPort, "127.0.0.1"))
        }
    }

    @Test
    fun `the wildcard host is probed on loopback`() {
        holdPort().use { held ->
            assertFalse(PortUtil.isAvailable(held.localPort, "0.0.0.0"))
        }
    }

    @Test
    fun `awaitAvailable returns at once for a free port`() {
        val slept = mutableListOf<Long>()
        assertTrue(PortUtil.awaitAvailable(freePort(), "127.0.0.1", 1_000) { slept.add(it) })
        assertTrue("should not have waited: $slept", slept.isEmpty())
    }

    @Test
    fun `awaitAvailable retries a busy port and then gives up`() {
        holdPort().use { held ->
            val slept = mutableListOf<Long>()
            val ok = PortUtil.awaitAvailable(held.localPort, "127.0.0.1", 20) { slept.add(it) }
            assertFalse(ok)
            assertTrue("expected at least one retry", slept.isNotEmpty())
        }
    }

    @Test
    fun `awaitAvailable succeeds once the holder lets go`() {
        val held = holdPort()
        val port = held.localPort
        val slept = mutableListOf<Long>()
        val ok = PortUtil.awaitAvailable(port, "127.0.0.1", 1_000) {
            slept.add(it)
            held.close()
        }
        assertTrue(ok)
        assertEquals(1, slept.size)
    }
}
