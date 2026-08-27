package dev.foxxycode.intellij.process

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Unit tests for the signal → wait → tree-kill sequence that turns "we asked the backend to
 * stop" into "the backend is gone". Driven by a fake, so no process is ever spawned.
 */
class ProcessReaperTest {

    /**
     * @param exitsOnSignal the process ends within the grace wait
     * @param exitsOnKill the process ends once the tree is killed
     * @param sleepWhileWaiting burn the requested budget, the way a real process handler would
     */
    private class Fake(
        private val exitsOnSignal: Boolean = false,
        private val exitsOnKill: Boolean = true,
        private val sleepWhileWaiting: Boolean = false,
        private val log: MutableList<String> = mutableListOf(),
        private val name: String = "p",
    ) : ProcessReaper.Killable {
        val calls: List<String> get() = log
        val waits = mutableListOf<Long>()
        private var killed = false

        override fun signal() {
            log.add("$name:signal")
        }

        override fun awaitExit(ms: Long): Boolean {
            log.add("$name:await")
            waits.add(ms)
            if (killed) return exitsOnKill
            if (exitsOnSignal) return true
            if (sleepWhileWaiting && ms > 0) Thread.sleep(ms)
            return false
        }

        override fun killTree(): Boolean {
            log.add("$name:kill")
            killed = true
            return true
        }
    }

    @Test
    fun `a process that exits on the signal is never tree-killed`() {
        val fake = Fake(exitsOnSignal = true)

        assertTrue(ProcessReaper.reap(fake))

        assertEquals(listOf("p:signal", "p:await"), fake.calls)
    }

    @Test
    fun `a process that ignores the signal is tree-killed exactly once`() {
        val fake = Fake(exitsOnSignal = false, exitsOnKill = true)

        assertTrue(ProcessReaper.reap(fake))

        assertEquals(listOf("p:signal", "p:await", "p:kill", "p:await"), fake.calls)
    }

    @Test
    fun `a process that survives everything is reported, not retried`() {
        val fake = Fake(exitsOnSignal = false, exitsOnKill = false)

        assertFalse(ProcessReaper.reap(fake))

        assertEquals(1, fake.calls.count { it == "p:signal" })
        assertEquals(1, fake.calls.count { it == "p:kill" })
    }

    @Test
    fun `reap never asks for more than the grace plus hard budget`() {
        val fake = Fake(exitsOnSignal = false, exitsOnKill = false)

        ProcessReaper.reap(fake, graceMs = 40, hardMs = 20)

        assertEquals(listOf(40L, 20L), fake.waits)
    }

    @Test
    fun `reapAll signals every process before waiting on any`() {
        val log = mutableListOf<String>()
        val fakes = (1..3).map { Fake(exitsOnSignal = true, log = log, name = "p$it") }

        assertEquals(0, ProcessReaper.reapAll(fakes, 100))

        assertEquals(listOf("p1:signal", "p2:signal", "p3:signal"), log.take(3))
        assertTrue(log.drop(3).all { it.endsWith(":await") })
    }

    @Test
    fun `reapAll spends one budget across all processes, not one each`() {
        val budget = 200L
        val fakes = (1..5).map { Fake(sleepWhileWaiting = true, exitsOnKill = true, name = "p$it") }

        val started = System.currentTimeMillis()
        ProcessReaper.reapAll(fakes, budget)
        val elapsed = System.currentTimeMillis() - started

        assertTrue("waited ${elapsed}ms for a ${budget}ms budget", elapsed < budget * 3)
        assertTrue(fakes.sumOf { it.waits.first() } <= budget)
        // The first one drains the budget; the rest are asked for whatever is left, down to zero.
        assertEquals(0L, fakes.last().waits.first())
    }

    @Test
    fun `reapAll reports how many processes are still alive`() {
        val fakes = (1..4).map { Fake(exitsOnKill = it > 2, name = "p$it") }

        assertEquals(2, ProcessReaper.reapAll(fakes, 10))
    }

    @Test
    fun `reapAll on an empty list does nothing`() {
        assertEquals(0, ProcessReaper.reapAll(emptyList(), 100))
    }
}
