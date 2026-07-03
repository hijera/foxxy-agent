package dev.foxxy.intellij

import dev.foxxy.intellij.settings.FoxxySettings
import java.util.Locale
import java.util.ResourceBundle

/**
 * Localized strings for the Foxxy IntelliJ plugin.
 *
 * Resolution order: explicit language from [FoxxySettings.State.language] ("en" / "ru"),
 * or [Locale.getDefault] when set to "system". Missing keys fall back to
 * [messages/FoxxyBundle.properties] (English).
 */
object FoxxyBundle {
    private const val PATH = "messages.FoxxyBundle"

    fun locale(): Locale {
        val lang = FoxxySettings.getInstance().state.language
        return when (lang) {
            "en" -> Locale.ENGLISH
            "ru" -> Locale.forLanguageTag("ru")
            else -> Locale.getDefault()
        }
    }

    @JvmStatic
    fun message(key: String, vararg params: Any): String {
        val raw = ResourceBundle.getBundle(PATH, locale(), FoxxyBundle::class.java.classLoader)
            .getString(key)
        return if (params.isEmpty()) raw else String.format(raw, *params)
    }
}
