package dev.foxxycode.intellij.process

/**
 * Whether a `foxxycode http` backend that just exited is worth telling the user about.
 *
 * The panel keeps showing its last page after the backend dies, and every request the
 * SPA makes from then on fails. Until this existed the only feedback was the SPA's red
 * "FoxxyCode UI error" overlay quoting a "TypeError: Failed to fetch" stack — which is
 * precisely the noise that overlay no longer paints, so the death has to be reported
 * from here instead.
 *
 * Three exits are expected and must stay silent:
 *  - [deliberate] — this project asked for it (toolbar Restart, project close), recorded
 *    in `FoxxyCodeProcessManager.terminating` before the kill;
 *  - [reapingAll] — the IDE is closing or the plugin is unloading, which reaps every
 *    backend through [FoxxyCodeBackends] **without** going through the per-project stop,
 *    so `terminating` is empty and cannot speak for these;
 *  - a backend that never became ready — [FoxxyCodeProcessManager.waitForReady] already
 *    reports that failure with a far better message than an exit code.
 */
internal fun shouldReportBackendExit(
    deliberate: Boolean,
    reapingAll: Boolean,
    hadBecomeReady: Boolean,
): Boolean = hadBecomeReady && !deliberate && !reapingAll
