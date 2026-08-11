package dev.foxxycode.intellij.process

import com.intellij.ide.AppLifecycleListener
import com.intellij.ide.plugins.DynamicPluginListener
import com.intellij.ide.plugins.IdeaPluginDescriptor

/**
 * Stops every `foxxycode http` backend at the two moments the IDE is about to walk away from
 * them: application shutdown and plugin unload.
 *
 * Project disposal already signals the backend, but it does not wait, and on Windows a child
 * process outlives the IDE that spawned it — leaving an orphan holding its port. Both callbacks
 * below run on the EDT and both deliberately block: waiting asynchronously here means the IDE
 * exits first and the orphan survives, which is the whole problem. The budget is shared across
 * all backends (see [ProcessReaper.reapAll]) and a hard kill on a Go HTTP server returns in
 * milliseconds.
 */
class FoxxyCodeBackendLifecycleListener : AppLifecycleListener, DynamicPluginListener {

    override fun appWillBeClosed(isRestart: Boolean) {
        FoxxyCodeBackends.stopAll()
    }

    override fun beforePluginUnload(pluginDescriptor: IdeaPluginDescriptor, isUpdate: Boolean) {
        if (pluginDescriptor.pluginId.idString == PLUGIN_ID) FoxxyCodeBackends.stopAll()
    }

    private companion object {
        const val PLUGIN_ID = "dev.foxxycode.intellij"
    }
}
