package dev.foxxycode.intellij.ui

/** Tracks visibility transitions on the EDT; hiding a tool window does not dispose its content. */
internal class PanelBackendLifecycle(
    private val shouldStop: () -> Boolean,
    private val start: () -> Unit,
    private val stop: () -> Unit,
) {
    // The panel starts its backend when constructed, before the first visibility notification.
    private var visible = true
    private var stopped = false
    private var disposed = false

    fun visibilityChanged(visible: Boolean) {
        if (disposed || this.visible == visible) return
        this.visible = visible
        if (!visible && shouldStop()) {
            stopped = true
            stop()
        } else if (visible && stopped) {
            stopped = false
            start()
        }
    }

    fun dispose() {
        if (disposed) return
        disposed = true
        if (!stopped && shouldStop()) stop()
    }
}
