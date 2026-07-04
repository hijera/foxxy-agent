package dev.foxxycode.intellij.ui

import com.intellij.util.messages.Topic

/**
 * Application-wide notification when the FoxxyCode UI language setting changes
 * (Settings | Tools | FoxxyCode → Language).
 */
interface FoxxyCodeLanguageListener {
    fun languageChanged()

    companion object {
        @JvmField
        val TOPIC: Topic<FoxxyCodeLanguageListener> =
            Topic.create("FoxxyCode Language Changed", FoxxyCodeLanguageListener::class.java)
    }
}
