package dev.foxxycode.intellij.autocomplete

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Unit tests for parsing GET /foxxycode/completion/config. Platform-free, so it runs in the plain
 * JUnit4 `test` task.
 */
class AutocompleteClientConfigTest {

    @Test
    fun `parses a full answer`() {
        val cfg = AutocompleteClientConfig.parse(
            """{"enabled":true,"trigger":"manual","debounce_ms":700,"multi_line":false,
               "timeout_ms":2500,"max_prefix_bytes":1234,"max_suffix_bytes":567}"""
        )!!
        assertTrue(cfg.enabled)
        assertEquals(AutocompleteClientConfig.TRIGGER_MANUAL, cfg.trigger)
        assertEquals(700, cfg.debounceMs)
        assertFalse(cfg.multiLine)
        assertEquals(2500, cfg.timeoutMs)
        assertEquals(1234, cfg.maxPrefixBytes)
        assertEquals(567, cfg.maxSuffixBytes)
    }

    @Test
    fun `manual trigger never fires automatically`() {
        val manual = AutocompleteClientConfig.parse("""{"enabled":true,"trigger":"manual"}""")!!
        assertTrue(manual.enabled)
        assertFalse(manual.automatic)

        val auto = AutocompleteClientConfig.parse("""{"enabled":true,"trigger":"auto"}""")!!
        assertTrue(auto.automatic)
    }

    @Test
    fun `a disabled backend never fires either way`() {
        val cfg = AutocompleteClientConfig.parse("""{"enabled":false,"trigger":"auto"}""")!!
        assertFalse(cfg.automatic)
    }

    @Test
    fun `missing and nonsensical fields fall back to the defaults`() {
        val cfg = AutocompleteClientConfig.parse("""{"enabled":true,"debounce_ms":0}""")!!
        assertEquals(AutocompleteClientConfig.DISABLED.debounceMs, cfg.debounceMs)
        assertEquals(AutocompleteClientConfig.DISABLED.trigger, cfg.trigger)
        assertEquals(AutocompleteClientConfig.DISABLED.maxPrefixBytes, cfg.maxPrefixBytes)
        assertTrue(cfg.multiLine)
    }

    @Test
    fun `a body that is not an object is rejected`() {
        assertNull(AutocompleteClientConfig.parse("not json"))
        assertNull(AutocompleteClientConfig.parse("[1,2,3]"))
    }

    @Test
    fun `the fallback used before the backend answers is off`() {
        assertFalse(AutocompleteClientConfig.DISABLED.enabled)
        assertFalse(AutocompleteClientConfig.DISABLED.automatic)
    }
}
