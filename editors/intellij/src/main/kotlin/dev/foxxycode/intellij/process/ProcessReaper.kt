package dev.foxxycode.intellij.process

/**
 * Turns "we told the backend to die" into "the backend is dead".
 *
 * `foxxycode http` blocks in `ListenAndServe` with no signal handling and no shutdown endpoint,
 * and on Windows a child does not die with the IDE that spawned it. Signalling without waiting
 * therefore leaves an orphan that keeps its port — and, before the binary was staged out of the
 * plugin directory, kept the plugin folder undeletable through the next update.
 *
 * Deliberately free of IntelliJ types so the sequencing is unit-testable against a fake.
 */
object ProcessReaper {

    /** How long a signalled process is given to exit on its own before the tree kill. */
    const val GRACE_MS = 1_500L

    /** How long the tree kill is given after that. */
    const val HARD_MS = 1_000L

    /** The three things reaping needs from a process handler. */
    interface Killable {
        /** Asks the process to terminate. Returns immediately. */
        fun signal()

        /** Waits up to [ms] for the exit; true when the process is gone. */
        fun awaitExit(ms: Long): Boolean

        /** Kills the process and everything it spawned. True when the kill was issued. */
        fun killTree(): Boolean
    }

    /** signal → bounded wait → tree kill → bounded wait. True when the process is gone. */
    fun reap(target: Killable, graceMs: Long = GRACE_MS, hardMs: Long = HARD_MS): Boolean {
        target.signal()
        if (target.awaitExit(graceMs)) return true
        target.killTree()
        return target.awaitExit(hardMs)
    }

    /**
     * Reaps [targets] within one shared [budgetMs]: every process is signalled first and only
     * then waited on, so N backends cost one budget instead of N. Returns how many survived.
     */
    fun reapAll(targets: List<Killable>, budgetMs: Long): Int {
        if (targets.isEmpty()) return 0
        for (target in targets) target.signal()

        val deadline = System.currentTimeMillis() + budgetMs
        val alive = mutableListOf<Killable>()
        for (target in targets) {
            val left = deadline - System.currentTimeMillis()
            if (!target.awaitExit(if (left > 0) left else 0)) alive.add(target)
        }
        if (alive.isEmpty()) return 0

        // Out of budget and still running: take the whole tree down and give it one short beat.
        for (target in alive) target.killTree()
        return alive.count { !it.awaitExit(HARD_MS) }
    }
}
