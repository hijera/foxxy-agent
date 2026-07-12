package dev.foxxycode.intellij.ui

import com.intellij.util.messages.Topic

/**
 * Application-wide notification that the FoxxyCode UI language changed. The
 * language has a single source of truth — the backend `ui.locale` config — so
 * this fires when it is read on server start
 * ([dev.foxxycode.intellij.process.FoxxyCodeProcessManager]) or when the user
 * flips the switcher inside the embedded SPA (Settings → General), relayed
 * through the JCEF locale bridge in
 * [dev.foxxycode.intellij.ui.FoxxyCodeBrowserPanel].
 */
interface FoxxyCodeLanguageListener {
    fun languageChanged()

    companion object {
        @JvmField
        val TOPIC: Topic<FoxxyCodeLanguageListener> =
            Topic.create("FoxxyCode Language Changed", FoxxyCodeLanguageListener::class.java)
    }
}
