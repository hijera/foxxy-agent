package dev.foxxycode.intellij.process

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class BackendExitReportTest {

    @Test
    fun `a ready backend that dies on its own is reported`() {
        assertTrue(shouldReportBackendExit(deliberate = false, reapingAll = false, hadBecomeReady = true))
    }

    @Test
    fun `a backend this project stopped is not reported`() {
        assertFalse(shouldReportBackendExit(deliberate = true, reapingAll = false, hadBecomeReady = true))
    }

    // FoxxyCodeBackends.stopAll/killAllNow reap handlers directly, so the per-project
    // `terminating` list is empty for them: without this guard every IDE shutdown and
    // plugin unload would raise a "the backend died" balloon on the way out.
    @Test
    fun `a backend reaped by IDE shutdown is not reported`() {
        assertFalse(shouldReportBackendExit(deliberate = false, reapingAll = true, hadBecomeReady = true))
    }

    // waitForReady already reports a startup failure, with the backend's own output.
    @Test
    fun `a backend that never became ready is not reported twice`() {
        assertFalse(shouldReportBackendExit(deliberate = false, reapingAll = false, hadBecomeReady = false))
    }
}
