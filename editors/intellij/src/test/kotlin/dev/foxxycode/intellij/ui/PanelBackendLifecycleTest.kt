package dev.foxxycode.intellij.ui

import org.junit.Assert.assertEquals
import org.junit.Test

class PanelBackendLifecycleTest {
    private var stopOnClose = false
    private val calls = mutableListOf<String>()
    private val lifecycle = PanelBackendLifecycle(
        shouldStop = { stopOnClose },
        start = { calls.add("start") },
        stop = { calls.add("stop") },
    )

    @Test
    fun `closing the panel keeps the backend by default`() {
        lifecycle.visibilityChanged(false)
        lifecycle.visibilityChanged(true)
        assertEquals(emptyList<String>(), calls)
    }

    @Test
    fun `closing with the setting enabled stops and reopening starts the backend`() {
        stopOnClose = true
        lifecycle.visibilityChanged(false)
        assertEquals(listOf("stop"), calls)
        lifecycle.visibilityChanged(true)
        assertEquals(listOf("stop", "start"), calls)
    }

    @Test
    fun `unrelated tool window events do not restart or stop twice`() {
        stopOnClose = true
        lifecycle.visibilityChanged(true)
        lifecycle.visibilityChanged(false)
        lifecycle.visibilityChanged(false)
        lifecycle.visibilityChanged(true)
        lifecycle.visibilityChanged(true)
        assertEquals(listOf("stop", "start"), calls)
    }

    @Test
    fun `disabling the setting while closed still restarts on reopening`() {
        stopOnClose = true
        lifecycle.visibilityChanged(false)
        stopOnClose = false
        lifecycle.visibilityChanged(true)
        lifecycle.visibilityChanged(false)
        assertEquals(listOf("stop", "start"), calls)
    }

    @Test
    fun `enabling the setting applies to the next close without recreating the panel`() {
        lifecycle.visibilityChanged(false)
        lifecycle.visibilityChanged(true)
        stopOnClose = true
        lifecycle.visibilityChanged(false)
        assertEquals(listOf("stop"), calls)
    }

    @Test
    fun `disposing the panel stops its backend when enabled and never restarts it`() {
        stopOnClose = true
        lifecycle.dispose()
        lifecycle.dispose()
        lifecycle.visibilityChanged(false)
        lifecycle.visibilityChanged(true)
        assertEquals(listOf("stop"), calls)
    }

    @Test
    fun `disposing an already closed panel does not stop twice`() {
        stopOnClose = true
        lifecycle.visibilityChanged(false)
        lifecycle.dispose()
        assertEquals(listOf("stop"), calls)
    }

    @Test
    fun `disposing with the setting disabled preserves the backend`() {
        lifecycle.dispose()
        assertEquals(emptyList<String>(), calls)
    }
}
