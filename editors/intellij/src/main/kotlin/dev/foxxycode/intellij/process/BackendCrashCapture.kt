package dev.foxxycode.intellij.process

/**
 * Keeps the part of a crashing Go backend's output that says why it crashed.
 *
 * A Go runtime crash prints a headline (`fatal error: …` or `panic: …`), sometimes preceded by
 * `runtime:` lines with the cause (`runtime: VirtualAlloc of 8192 bytes failed with errno=1455`),
 * then the crashing goroutine and after it every other goroutine: thousands of lines. The last
 * lines of that output - all a rolling tail keeps - are some idle goroutine and explain nothing.
 * This captures the headline, the `runtime:` lines right before it and the frames that follow,
 * up to [MAX_LINES].
 */
class BackendCrashCapture {
    private val runtimeLines = ArrayDeque<String>()
    private val captured = ArrayList<String>()

    /** Feed one non-blank output line, in the order the backend printed it. */
    @Synchronized
    fun offer(line: String) {
        if (captured.isEmpty()) {
            when {
                isHeadline(line) -> {
                    captured.addAll(runtimeLines)
                    captured.add(line)
                }
                line.startsWith("runtime: ") -> {
                    runtimeLines.addLast(line)
                    while (runtimeLines.size > MAX_RUNTIME_LINES) runtimeLines.removeFirst()
                }
                else -> runtimeLines.clear()
            }
            return
        }
        if (captured.size < MAX_LINES) captured.add(line)
    }

    /** The captured crash, one line per line, or "" when the backend did not crash. */
    @Synchronized
    fun text(): String = captured.joinToString("\n")

    /** True when the crash was the runtime failing to get memory from the system. */
    val outOfMemory: Boolean
        @Synchronized get() = captured.any { it.contains("out of memory") }

    @Synchronized
    fun clear() {
        runtimeLines.clear()
        captured.clear()
    }

    private fun isHeadline(line: String): Boolean =
        line.startsWith("fatal error: ") || line.startsWith("panic: ")

    companion object {
        /** Headline plus the top of the crashing goroutine: enough to name the cause and the call. */
        const val MAX_LINES = 16

        /** `runtime:` lines kept waiting for a headline; the cause is printed right before it. */
        private const val MAX_RUNTIME_LINES = 3
    }
}
