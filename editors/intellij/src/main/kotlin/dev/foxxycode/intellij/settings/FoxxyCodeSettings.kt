package dev.foxxycode.intellij.settings

import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.components.PersistentStateComponent
import com.intellij.openapi.components.State
import com.intellij.openapi.components.Storage

/**
 * Application-wide FoxxyCode plugin settings, persisted in foxxycode.xml.
 *
 * The foxxycode binary is bundled with the plugin and resolved automatically;
 * [State.binaryPath] is an optional override for development/custom builds.
 *
 * Registered as an applicationService in plugin.xml (kept annotation-free for 212 compatibility).
 */
@State(name = "FoxxyCodeSettings", storages = [Storage("foxxycode.xml")])
class FoxxyCodeSettings : PersistentStateComponent<FoxxyCodeSettings.State> {

    class State {
        var binaryPath: String = ""
        var host: String = "127.0.0.1"
        var fixedPort: Int = 0
        var foxxycodeHome: String = ""
        var extraArgs: String = ""
        var firstRunCompleted: Boolean = false
        var followIdeTheme: Boolean = true

        // When true, agent file edits are applied without prompting: the plugin auto-accepts
        // each proposed edit and shows the resulting diff inline (with a Revert affordance).
        // When false, each edit is shown inline in the editor with Accept/Reject.
        var autoApproveEdits: Boolean = false

        // Show native inline diffs in the editor when the agent edits files.
        var nativeDiffs: Boolean = true

        // UI language: "system" | "en" | "ru". "system" follows Locale.getDefault().
        var language: String = "system"
    }

    private var myState = State()

    override fun getState(): State = myState

    override fun loadState(state: State) {
        myState = state
    }

    companion object {
        fun getInstance(): FoxxyCodeSettings =
            ApplicationManager.getApplication().getService(FoxxyCodeSettings::class.java)
    }
}
