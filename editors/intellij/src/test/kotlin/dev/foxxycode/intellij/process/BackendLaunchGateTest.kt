package dev.foxxycode.intellij.process

import java.util.concurrent.CancellationException
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import kotlin.concurrent.thread
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class BackendLaunchGateTest {
    @Test
    fun `closing while a launch is queued prevents it from spawning or publishing readiness`() {
        val gate = BackendLaunchGate()
        val ticket = gate.ticket()
        gate.cancel {}
        var launched = false
        try {
            gate.run(ticket) { launched = true }
        } catch (_: CancellationException) {
            // Expected: neither process creation nor a stale callback may run.
        }
        assertFalse(launched)
        assertFalse(gate.isCurrent(ticket))
        assertTrue(gate.isCurrent(gate.ticket()))
    }

    @Test
    fun `disposing the service also rejects future launches`() {
        val gate = BackendLaunchGate()
        gate.cancel(close = true) {}
        assertFalse(gate.isCurrent(gate.ticket()))
    }

    @Test
    fun `a stop racing with process creation always sees and stops the new process`() {
        val gate = BackendLaunchGate()
        val ticket = gate.ticket()
        val creating = CountDownLatch(1)
        val release = CountDownLatch(1)
        val stopped = CountDownLatch(1)
        val calls = mutableListOf<String>()
        val launch = thread {
            gate.run(ticket) {
                creating.countDown()
                check(release.await(5, TimeUnit.SECONDS))
                calls.add("spawn")
            }
        }
        assertTrue(creating.await(5, TimeUnit.SECONDS))
        val stop = thread {
            gate.cancel { calls.add("stop") }
            stopped.countDown()
        }
        release.countDown()
        assertTrue(stopped.await(5, TimeUnit.SECONDS))
        launch.join(5_000)
        stop.join(5_000)
        assertEquals(listOf("spawn", "stop"), calls)
        assertFalse(gate.isCurrent(ticket))
    }
}
