package dev.foxxycode.intellij.autocomplete

import com.google.gson.JsonObject
import com.google.gson.JsonParser
import com.intellij.openapi.diagnostic.logger
import java.io.OutputStreamWriter
import java.net.HttpURLConnection
import java.net.URI
import java.nio.charset.StandardCharsets
import java.util.concurrent.atomic.AtomicReference

/**
 * Talks to the backend's autocomplete endpoints. Requests run on a pooled thread and are
 * cancellable: [InFlight.cancel] disconnects the socket, which drops the server's request context
 * and kills the upstream LLM call with it. That matters more here than anywhere else in the plugin,
 * because the next keystroke makes the answer in flight worthless.
 *
 * Mirrors the plain HttpURLConnection style of
 * [dev.foxxycode.intellij.editor.FoxxyCodeEditorContextService]; the plugin has no shared HTTP
 * helper and this is the only place that needs cancellation.
 */
object CompletionClient {
    private val log = logger<CompletionClient>()

    /** Handle on a running request so a newer keystroke can abandon it. */
    class InFlight {
        private val conn = AtomicReference<HttpURLConnection?>(null)

        @Volatile
        var cancelled: Boolean = false
            private set

        internal fun attach(c: HttpURLConnection) {
            conn.set(c)
            if (cancelled) c.disconnect()
        }

        internal fun detach() = conn.set(null)

        fun cancel() {
            cancelled = true
            conn.getAndSet(null)?.let { runCatching { it.disconnect() } }
        }
    }

    data class Request(
        val prefix: String,
        val suffix: String,
        val path: String,
        val language: String,
    )

    /** Reads the client-facing settings; null when the backend is unreachable or answers badly. */
    fun fetchConfig(baseUrl: String, timeoutMs: Int = 3000): AutocompleteClientConfig? {
        return try {
            val conn = open(baseUrl, "foxxycode/completion/config", timeoutMs)
            conn.requestMethod = "GET"
            val body = conn.readBody() ?: return null
            AutocompleteClientConfig.parse(body)
        } catch (e: Exception) {
            log.debug("completion config fetch failed: ${e.message}")
            null
        }
    }

    /**
     * When the backend last answered 429, automatic requests stay off until this wall-clock
     * millisecond. The provider has already said no; asking again sooner only costs money.
     */
    @Volatile
    var pausedUntilMillis: Long = 0L
        private set

    fun isPaused(nowMillis: Long = System.currentTimeMillis()): Boolean = nowMillis < pausedUntilMillis

    /**
     * Asks for the text to insert at the caret. Returns null when there is nothing to show, which
     * covers every failure too: a suggestion that cannot be fetched is simply not drawn, never an
     * error the user has to dismiss mid-typing.
     */
    fun fetchCompletion(baseUrl: String, req: Request, timeoutMs: Int, inFlight: InFlight): String? {
        val body = JsonObject().apply {
            addProperty("prefix", req.prefix)
            addProperty("suffix", req.suffix)
            addProperty("path", req.path)
            addProperty("language", req.language)
        }.toString()

        return try {
            val conn = open(baseUrl, "foxxycode/completion", timeoutMs)
            conn.requestMethod = "POST"
            conn.doOutput = true
            conn.setRequestProperty("Content-Type", "application/json")
            inFlight.attach(conn)
            OutputStreamWriter(conn.outputStream, StandardCharsets.UTF_8).use { it.write(body) }
            if (conn.responseCode == HTTP_TOO_MANY_REQUESTS) {
                val seconds = SuggestionText.retryAfterSeconds(conn.getHeaderField("Retry-After"))
                pausedUntilMillis = System.currentTimeMillis() + seconds * 1000
                log.info("completion rate limited by the provider; pausing automatic requests for ${seconds}s")
                conn.disconnect()
                return null
            }
            val text = conn.readBody() ?: return null
            val root = JsonParser.parseString(text)
            if (!root.isJsonObject) return null
            root.asJsonObject.get("completion")?.asString?.takeIf { it.isNotEmpty() }
        } catch (e: Exception) {
            // A cancelled request lands here as a socket error; that is the normal path, not a fault.
            if (!inFlight.cancelled) log.debug("completion fetch failed: ${e.message}")
            null
        } finally {
            inFlight.detach()
        }
    }

    /**
     * Reports what happened to a suggestion (shown, accepted, dismissed, cache_hit). Failures are
     * ignored: the counters are diagnostics, never something the editor should stall on.
     */
    fun sendFeedback(baseUrl: String, event: String) {
        try {
            val conn = open(baseUrl, "foxxycode/completion/feedback", FEEDBACK_TIMEOUT_MS)
            conn.requestMethod = "POST"
            conn.doOutput = true
            conn.setRequestProperty("Content-Type", "application/json")
            val body = JsonObject().apply { addProperty("event", event) }.toString()
            OutputStreamWriter(conn.outputStream, StandardCharsets.UTF_8).use { it.write(body) }
            conn.responseCode
            conn.disconnect()
        } catch (e: Exception) {
            log.debug("completion feedback failed: ${e.message}")
        }
    }

    private fun open(baseUrl: String, path: String, timeoutMs: Int): HttpURLConnection {
        val base = if (baseUrl.endsWith("/")) baseUrl else "$baseUrl/"
        val conn = URI.create(base + path).toURL().openConnection() as HttpURLConnection
        conn.connectTimeout = CONNECT_TIMEOUT_MS
        conn.readTimeout = timeoutMs
        return conn
    }

    private fun HttpURLConnection.readBody(): String? {
        return try {
            if (responseCode != HttpURLConnection.HTTP_OK) return null
            inputStream.bufferedReader(StandardCharsets.UTF_8).use { it.readText() }
        } finally {
            disconnect()
        }
    }

    private const val CONNECT_TIMEOUT_MS = 2000
    private const val FEEDBACK_TIMEOUT_MS = 2000
    private const val HTTP_TOO_MANY_REQUESTS = 429
}
