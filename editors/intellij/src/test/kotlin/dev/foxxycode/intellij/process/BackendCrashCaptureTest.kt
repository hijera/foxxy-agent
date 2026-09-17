package dev.foxxycode.intellij.process

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class BackendCrashCaptureTest {

    // What a Go backend printed when Windows ran out of commit memory during a turn (a user's
    // idea.log, 2026-09-17): two lines that say why, then thousands of goroutine frames. The
    // balloon quoted the last 20 lines, which were an idle goroutine and explained nothing.
    private val outOfMemoryDump = listOf(
        "time=2026-09-17T15:44:30.001+03:00 level=INFO msg=\"turn started\"",
        "runtime: VirtualAlloc of 8192 bytes failed with errno=1455",
        "fatal error: out of memory",
        "runtime stack:",
        "runtime.throw({0x1a2b3c4?, 0xc000123456?})",
        "\truntime/panic.go:1101 +0x4d fp=0x9e7fdff5d8 sp=0x9e7fdff5a8 pc=0x7ff6a1b2c3d4",
        "goroutine 214 [running]:",
        "os.ReadFile({0xc0012a8000, 0x5f})",
        "github.com/hijera/foxxycode-agent/internal/session.TakeWorkspaceSnapshot.func1(...)",
        "\tgithub.com/hijera/foxxycode-agent/internal/session/turn_diff.go:89 +0x2c5",
    ) + (1..60).flatMap {
        listOf(
            "goroutine ${300 + it} [IO wait]:",
            "internal/poll.runtime_pollWait(0x1d2e3f4a5b6, 0x72)",
            "internal/poll.(*FD).Read(0xc000456000, {0xc000789000, 0x1000, 0x1000})",
            "created by net/http.(*Server).Serve in goroutine 1",
        )
    }

    @Test
    fun `an out of memory crash keeps its headline instead of the goroutine tail`() {
        val capture = BackendCrashCapture()
        outOfMemoryDump.forEach(capture::offer)

        val text = capture.text()
        val lines = text.lines()
        assertEquals("runtime: VirtualAlloc of 8192 bytes failed with errno=1455", lines[0])
        assertEquals("fatal error: out of memory", lines[1])
        assertTrue("the crashing goroutine's frames are kept", text.contains("TakeWorkspaceSnapshot"))
        assertTrue("the capture is bounded", lines.size <= BackendCrashCapture.MAX_LINES)
        assertTrue(capture.outOfMemory)
        assertFalse("the tail the balloon used to quote says nothing", outOfMemoryDump.takeLast(20).any { it.contains("fatal error") })
    }

    @Test
    fun `a panic is captured and is not out of memory`() {
        val capture = BackendCrashCapture()
        listOf(
            "level=INFO msg=\"serving\"",
            "panic: runtime error: index out of range [3] with length 3",
            "goroutine 57 [running]:",
            "github.com/hijera/foxxycode-agent/internal/agent.(*Loop).step(...)",
        ).forEach(capture::offer)

        assertTrue(capture.text().startsWith("panic: runtime error: index out of range"))
        assertFalse(capture.outOfMemory)
    }

    @Test
    fun `ordinary output and a stray runtime line capture nothing`() {
        val capture = BackendCrashCapture()
        listOf(
            "level=INFO msg=\"starting HTTP server\"",
            "runtime: note: something the runtime logged but survived",
            "level=INFO msg=\"ready\"",
        ).forEach(capture::offer)

        assertEquals("", capture.text())
        assertFalse(capture.outOfMemory)
    }

    @Test
    fun `clear forgets a previous run's crash`() {
        val capture = BackendCrashCapture()
        outOfMemoryDump.forEach(capture::offer)
        capture.clear()

        assertEquals("", capture.text())
        assertFalse(capture.outOfMemory)
    }
}
