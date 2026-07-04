package dev.foxxycode.intellij.diff

import com.google.gson.JsonParser
import com.intellij.openapi.diagnostic.logger
import java.io.BufferedReader
import java.io.InputStreamReader
import java.net.HttpURLConnection
import java.net.URI
import java.nio.charset.StandardCharsets

/**
 * Reads the foxxycode `GET /foxxycode/ide/events` Server-Sent-Events stream on a background daemon
 * thread and delivers parsed [FoxxyCodeEditEvent]s to [onEvent]. Reconnects with backoff until
 * [stop] is called. Pure `HttpURLConnection` (no extra deps), matching FoxxyCodeProcessManager.
 */
class FoxxyCodeIdeEventClient(
    private val baseUrl: String,
    private val onEvent: (FoxxyCodeEditEvent) -> Unit,
) {
    private val log = logger<FoxxyCodeIdeEventClient>()

    @Volatile
    private var running = false
    private var thread: Thread? = null
    private var connection: HttpURLConnection? = null

    fun start() {
        if (running) return
        running = true
        thread = Thread({ loop() }, "foxxycode-ide-events").apply {
            isDaemon = true
            start()
        }
    }

    fun stop() {
        running = false
        try {
            connection?.disconnect()
        } catch (_: Exception) {
        }
        thread?.interrupt()
        thread = null
    }

    private fun eventsUrl(): String {
        val base = if (baseUrl.endsWith("/")) baseUrl else "$baseUrl/"
        return base + "foxxycode/ide/events"
    }

    private fun loop() {
        while (running) {
            try {
                readStream()
            } catch (e: Exception) {
                if (running) log.debug("ide events stream ended: ${e.message}")
            }
            if (!running) break
            try {
                Thread.sleep(1500) // backoff before reconnecting
            } catch (_: InterruptedException) {
                break
            }
        }
    }

    private fun readStream() {
        val conn = URI.create(eventsUrl()).toURL().openConnection() as HttpURLConnection
        conn.connectTimeout = 3000
        conn.readTimeout = 0 // stream stays open
        conn.requestMethod = "GET"
        conn.setRequestProperty("Accept", "text/event-stream")
        connection = conn

        BufferedReader(InputStreamReader(conn.inputStream, StandardCharsets.UTF_8)).use { reader ->
            // The event type is carried inside each JSON `data` payload, so `event:` lines are
            // ignored here and the type is read from the parsed object instead.
            val data = StringBuilder()
            while (running) {
                val line = reader.readLine() ?: break
                when {
                    line.startsWith(":") -> {
                        // comment / heartbeat — ignore
                    }
                    line.startsWith("data:") -> {
                        if (data.isNotEmpty()) data.append('\n')
                        data.append(line.removePrefix("data:").trim())
                    }
                    line.isEmpty() -> {
                        if (data.isNotEmpty()) dispatch(data.toString())
                        data.setLength(0)
                    }
                }
            }
        }
    }

    private fun dispatch(payload: String) {
        val ev = parse(payload) ?: return
        try {
            onEvent(ev)
        } catch (e: Exception) {
            log.warn("ide event handler failed", e)
        }
    }

    private fun parse(payload: String): FoxxyCodeEditEvent? {
        return try {
            val o = JsonParser.parseString(payload).asJsonObject
            fun str(key: String) = if (o.has(key) && !o.get(key).isJsonNull) o.get(key).asString else ""
            val type = str("type")
            if (type.isEmpty()) return null
            FoxxyCodeEditEvent(
                type = type,
                toolCallId = str("toolCallId"),
                sessionId = str("sessionId"),
                path = str("path"),
                before = str("before"),
                after = str("after"),
            )
        } catch (e: Exception) {
            log.warn("failed to parse ide event: ${e.message}")
            null
        }
    }
}
