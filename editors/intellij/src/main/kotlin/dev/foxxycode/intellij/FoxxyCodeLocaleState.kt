package dev.foxxycode.intellij

/**
 * Effective UI locale for the plugin, driven by the backend config.
 *
 * The single language switcher for the whole application lives in the SPA
 * (Settings → General) and persists `ui.locale` into the backend config.yaml.
 * The plugin has no language setting of its own: it fetches `ui.locale` once
 * the server is ready ([dev.foxxycode.intellij.process.FoxxyCodeProcessManager])
 * and receives live updates from the embedded SPA via the JCEF
 * `window.foxxycodeUi.onLocaleChange` bridge
 * ([dev.foxxycode.intellij.ui.FoxxyCodeBrowserPanel]).
 *
 * `null` means "auto": follow [java.util.Locale.getDefault].
 */
object FoxxyCodeLocaleState {
    @Volatile
    var effectiveLocale: String? = null
        private set

    /**
     * Adopt a backend/SPA locale. Accepts only "en"/"ru"; anything else means
     * auto (null). Returns true when the value actually changed, so callers can
     * decide whether to publish a change notification.
     */
    fun update(value: String?): Boolean {
        val next = if (value == "en" || value == "ru") value else null
        if (next == effectiveLocale) return false
        effectiveLocale = next
        return true
    }
}
