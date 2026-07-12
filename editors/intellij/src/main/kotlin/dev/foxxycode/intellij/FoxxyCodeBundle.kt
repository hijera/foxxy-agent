package dev.foxxycode.intellij

import java.util.Locale
import java.util.ResourceBundle

/**
 * Localized strings for the FoxxyCode IntelliJ plugin.
 *
 * Resolution order: explicit backend `ui.locale` from config.yaml (the single
 * app-wide language switcher, SPA Settings → General), exposed via
 * [FoxxyCodeLocaleState]; or [Locale.getDefault] when the config says auto.
 * Missing keys fall back to [messages/FoxxyCodeBundle.properties] (English).
 */
object FoxxyCodeBundle {
    private const val PATH = "messages.FoxxyCodeBundle"

    fun locale(): Locale {
        return when (FoxxyCodeLocaleState.effectiveLocale) {
            "en" -> Locale.ENGLISH
            "ru" -> Locale.forLanguageTag("ru")
            else -> Locale.getDefault()
        }
    }

    /** SPA locale id passed as `?lang=` and to `window.foxxycodeUi.setLocale` ("en" or "ru"). */
    fun spaLanguageCode(): String {
        return when (FoxxyCodeLocaleState.effectiveLocale) {
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
