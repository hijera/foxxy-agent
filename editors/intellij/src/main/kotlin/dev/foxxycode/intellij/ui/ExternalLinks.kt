package dev.foxxycode.intellij.ui

import java.net.URI

/**
 * New-tab links in the SPA (Settings → API docs, the website) reach JCEF as popups. Left alone,
 * CEF opens them in an empty native window; the panel cancels the popup and hands the URL to the
 * system browser instead. Platform-free so the check is unit-tested.
 */
object ExternalLinks {
    /** The URL to open in the system browser for a popup, or null when it must not be opened. */
    fun systemBrowserUrl(targetUrl: String?): String? {
        val url = targetUrl?.trim().orEmpty()
        if (url.isEmpty()) return null
        val scheme = try {
            URI(url).scheme?.lowercase()
        } catch (e: Exception) {
            return null
        }
        return if (scheme == "http" || scheme == "https") url else null
    }
}
