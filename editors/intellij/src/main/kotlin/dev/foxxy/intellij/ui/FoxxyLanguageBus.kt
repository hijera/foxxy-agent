package dev.foxxy.intellij.ui

import com.intellij.util.messages.Topic

/**
 * Application-wide notification when the Foxxy UI language setting changes
 * (Settings | Tools | Foxxy → Language).
 */
interface FoxxyLanguageListener {
    fun languageChanged()

    companion object {
        @JvmField
        val TOPIC: Topic<FoxxyLanguageListener> =
            Topic.create("Foxxy Language Changed", FoxxyLanguageListener::class.java)
    }
}
