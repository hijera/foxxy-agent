package dev.foxxycode.intellij

import dev.foxxycode.intellij.settings.FoxxyCodeSettings
import java.util.Locale
import java.util.ResourceBundle

/**
 * Localized strings for the FoxxyCode IntelliJ plugin.
 *
 * Resolution order: explicit language from [FoxxyCodeSettings.State.language] ("en" / "ru"),
 * or [Locale.getDefault] when set to "system". Missing keys fall back to
 * [messages/FoxxyCodeBundle.properties] (English).
 */
object FoxxyCodeBundle {
    private const val PATH = "messages.FoxxyCodeBundle"

    fun locale(): Locale {
        val lang = FoxxyCodeSettings.getInstance().state.language
        return when (lang) {
            "en" -> Locale.ENGLISH
            "ru" -> Locale.forLanguageTag("ru")
            else -> Locale.getDefault()
        }
    }

    /** SPA locale id passed as `?lang=` and to `window.foxxycodeUi.setLocale` ("en" or "ru"). */
    fun spaLanguageCode(): String {
        val lang = FoxxyCodeSettings.getInstance().state.language
        return when (lang) {
            "en" -> "en"
            "ru" -> "ru"
            else -> {
                val tag = Locale.getDefault().language.lowercase(Locale.ROOT)
                if (tag == "ru") "ru" else "en"
            }
        }
    }

    @JvmStatic
    fun message(key: String, vararg params: Any): String {
        val raw = ResourceBundle.getBundle(PATH, locale(), FoxxyCodeBundle::class.java.classLoader)
            .getString(key)
        return if (params.isEmpty()) raw else String.format(raw, *params)
    }
}
