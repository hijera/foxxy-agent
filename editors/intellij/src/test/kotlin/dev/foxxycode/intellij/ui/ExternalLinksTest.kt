package dev.foxxycode.intellij.ui

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * Unit tests for [ExternalLinks.systemBrowserUrl], the check behind opening the SPA's new-tab
 * links (API docs, the website) in the system browser instead of an empty CEF popup window.
 */
class ExternalLinksTest {

    @Test
    fun `http and https popups go to the system browser`() {
        assertEquals(
            "https://hijera.github.io/foxxy-agent/",
            ExternalLinks.systemBrowserUrl("https://hijera.github.io/foxxy-agent/"),
        )
        assertEquals("http://127.0.0.1:40123/docs/", ExternalLinks.systemBrowserUrl("http://127.0.0.1:40123/docs/"))
        assertEquals("HTTPS://example.com/", ExternalLinks.systemBrowserUrl("HTTPS://example.com/"))
    }

    @Test
    fun `other schemes and empty targets are never opened`() {
        for (url in listOf("file:///C:/Windows/System32/calc.exe", "javascript:alert(1)", "about:blank", "", "   ")) {
            assertNull(url, ExternalLinks.systemBrowserUrl(url))
        }
        assertNull(ExternalLinks.systemBrowserUrl(null))
    }
}
