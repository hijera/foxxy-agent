package dev.foxxycode.intellij.autocomplete

import com.google.gson.JsonObject
import com.google.gson.JsonParser

/**
 * The autocomplete settings the backend hands to editor clients
 * (GET /foxxycode/completion/config). They live in config.autocomplete, edited in the Autocomplete
 * tab of the FoxxyCode settings form, so the plugin never keeps a second copy of these knobs.
 *
 * [DISABLED] is the safe default used until the backend answers: an unreachable server must not
 * start firing suggestions on its own.
 */
data class AutocompleteClientConfig(
    val enabled: Boolean,
    val trigger: String,
    val debounceMs: Int,
    val multiLine: Boolean,
    val timeoutMs: Int,
    val maxPrefixBytes: Int,
    val maxSuffixBytes: Int,
) {
    /** True when suggestions should be requested while the user types, rather than on the shortcut. */
    val automatic: Boolean get() = enabled && trigger != TRIGGER_MANUAL

    companion object {
        const val TRIGGER_AUTO = "auto"
        const val TRIGGER_MANUAL = "manual"

        val DISABLED = AutocompleteClientConfig(
            enabled = false,
            trigger = TRIGGER_AUTO,
            debounceMs = 350,
            multiLine = true,
            timeoutMs = 4000,
            maxPrefixBytes = 4000,
            maxSuffixBytes = 1500,
        )

        /** Parses the endpoint's JSON body, falling back per field so a partial answer still works. */
        fun parse(json: String): AutocompleteClientConfig? {
            val root = runCatching { JsonParser.parseString(json) }.getOrNull() ?: return null
            if (!root.isJsonObject) return null
            val o = root.asJsonObject
            return AutocompleteClientConfig(
                enabled = o.bool("enabled", DISABLED.enabled),
                trigger = o.str("trigger", DISABLED.trigger),
                debounceMs = o.int("debounce_ms", DISABLED.debounceMs),
                multiLine = o.bool("multi_line", DISABLED.multiLine),
                timeoutMs = o.int("timeout_ms", DISABLED.timeoutMs),
                maxPrefixBytes = o.int("max_prefix_bytes", DISABLED.maxPrefixBytes),
                maxSuffixBytes = o.int("max_suffix_bytes", DISABLED.maxSuffixBytes),
            )
        }

        private fun JsonObject.bool(key: String, fallback: Boolean): Boolean =
            runCatching { get(key)?.asBoolean ?: fallback }.getOrDefault(fallback)

        private fun JsonObject.int(key: String, fallback: Int): Int =
            runCatching { get(key)?.asInt ?: fallback }.getOrDefault(fallback).let {
                if (it > 0) it else fallback
            }

        private fun JsonObject.str(key: String, fallback: String): String =
            runCatching { get(key)?.asString?.takeIf { it.isNotBlank() } ?: fallback }.getOrDefault(fallback)
    }
}
