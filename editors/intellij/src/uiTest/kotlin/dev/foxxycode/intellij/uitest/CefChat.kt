package dev.foxxycode.intellij.uitest

import com.google.gson.Gson
import com.google.gson.JsonObject
import com.google.gson.JsonParser
import java.net.URI
import java.net.http.HttpClient
import java.net.http.HttpRequest
import java.net.http.HttpResponse
import java.net.http.WebSocket
import java.time.Duration
import java.util.concurrent.CompletableFuture
import java.util.concurrent.CompletionStage
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger

/**
 * Reaches *inside* the chat: a Chrome DevTools Protocol client for the SPA hosted in the
 * plugin's JCEF browser.
 *
 * Remote Robot ends at the Swing boundary — the composer, the messages and every SPA control
 * render inside Chromium and have no Swing peers. The sandbox therefore runs JCEF with its
 * remote-debugging port open (`ide.browser.jcef.debug.port` in `runIdeForUiTests`), and this
 * client talks CDP to the page: evaluate JS, read the rendered text, click by CSS selector,
 * and send *trusted* input events (`Input.insertText` / `Input.dispatchKeyEvent`), which is
 * what React-controlled inputs require — assigning `.value` from JS never updates their state.
 *
 * The page target exists only once [dev.foxxycode.intellij.ui.FoxxyCodeBrowserPanel] has
 * loaded the SPA, so connect with [connectWithRetry] after opening the tool window.
 */
class CefChat private constructor(private val ws: WebSocket) : AutoCloseable {

    /** Evaluates a JS [expression] in the page; returns its value rendered as a plain string. */
    fun eval(expression: String): String {
        val params = JsonObject()
        params.addProperty("expression", expression)
        params.addProperty("returnByValue", true)
        params.addProperty("awaitPromise", true)
        val result = call("Runtime.evaluate", params)
        val ex = result.getAsJsonObject("exceptionDetails")
        if (ex != null) {
            val detail = ex.getAsJsonObject("exception")?.get("description")?.asString
                ?: ex.get("text")?.asString ?: ex.toString()
            throw IllegalStateException("page JS failed: $detail")
        }
        val value = result.getAsJsonObject("result") ?: return ""
        return when {
            value.has("value") -> {
                val v = value.get("value")
                if (v.isJsonPrimitive && v.asJsonPrimitive.isString) v.asString else v.toString()
            }
            // undefined / functions and other non-serialisable results.
            else -> value.get("type")?.asString ?: ""
        }
    }

    /** The text the page currently renders — the CDP twin of the Swing-side `text` command. */
    fun pageText(): String = eval("document.body ? document.body.innerText : ''")

    /** Scrolls the first element matching the CSS [selector] into view, focuses and clicks it. */
    fun click(selector: String) {
        val sel = gson.toJson(selector)
        eval(
            """
            (function () {
              var el = document.querySelector($sel);
              if (!el) throw new Error('no element matches ' + $sel);
              if (el.scrollIntoView) el.scrollIntoView({block: 'center'});
              if (el.focus) el.focus();
              if (el.click) el.click();
              return 'clicked';
            })()
            """.trimIndent(),
        )
    }

    /**
     * Types [text] into whatever element the page has focused, as a trusted input event.
     * Focus something first ([click] does), or the text goes nowhere. Replaces the current
     * selection, so `el.select()` + insertText overwrites a React-controlled input cleanly.
     */
    fun insertText(text: String) {
        val params = JsonObject()
        params.addProperty("text", text)
        call("Input.insertText", params)
    }

    /** Presses one named key ([KEYS]) in the page — Enter to send, Escape to close a popup. */
    fun pressKey(name: String) {
        val key = KEYS[name.uppercase()]
            ?: throw IllegalArgumentException("unknown cef key '$name' (known: ${KEYS.keys.sorted()})")
        dispatchKey("rawKeyDown", key)
        if (key.text != null) dispatchKey("char", key)
        dispatchKey("keyUp", key)
    }

    /** One CDP key event; a full press is rawKeyDown (+char for keys that produce text) +keyUp. */
    private fun dispatchKey(type: String, key: Key) {
        val params = JsonObject()
        params.addProperty("type", type)
        params.addProperty("key", key.key)
        params.addProperty("code", key.code)
        params.addProperty("windowsVirtualKeyCode", key.vk)
        params.addProperty("nativeVirtualKeyCode", key.vk)
        if (type == "char" && key.text != null) params.addProperty("text", key.text)
        call("Input.dispatchKeyEvent", params)
    }

    /** Polls [pageText] until it contains [needle]; fails with the last text it saw. */
    fun waitForText(needle: String, timeout: Duration = Duration.ofSeconds(30)) {
        val deadline = System.nanoTime() + timeout.toNanos()
        var last = ""
        while (System.nanoTime() < deadline) {
            last = pageText()
            if (last.contains(needle)) return
            Thread.sleep(500)
        }
        throw AssertionError("page text never contained '$needle' within ${timeout.seconds}s; last text:\n$last")
    }

    override fun close() {
        try {
            ws.sendClose(WebSocket.NORMAL_CLOSURE, "done").get(3, TimeUnit.SECONDS)
        } catch (e: Exception) {
            ws.abort()
        }
    }

    // ---------------------------------------------------------------- protocol plumbing

    private val gson = Gson()
    private val nextId = AtomicInteger(1)
    private val pending = ConcurrentHashMap<Int, CompletableFuture<JsonObject>>()

    /** Sends one CDP command and blocks for its response's `result` object. */
    private fun call(method: String, params: JsonObject): JsonObject {
        val id = nextId.getAndIncrement()
        val future = CompletableFuture<JsonObject>()
        pending[id] = future
        val msg = JsonObject()
        msg.addProperty("id", id)
        msg.addProperty("method", method)
        msg.add("params", params)
        try {
            ws.sendText(gson.toJson(msg), true).get(10, TimeUnit.SECONDS)
            val reply = future.get(20, TimeUnit.SECONDS)
            reply.getAsJsonObject("error")?.let {
                throw IllegalStateException("CDP $method failed: $it")
            }
            return reply.getAsJsonObject("result") ?: JsonObject()
        } finally {
            pending.remove(id)
        }
    }

    /** Routes response frames to their waiting [call]; CDP events are ignored. */
    private fun onMessage(text: String) {
        try {
            val msg = JsonParser.parseString(text).asJsonObject
            val id = msg.get("id")?.asInt ?: return
            pending.remove(id)?.complete(msg)
        } catch (e: Exception) {
            // A frame that is not a JSON response concerns no pending call.
        }
    }

    companion object {

        private data class Key(val key: String, val code: String, val vk: Int, val text: String? = null)

        private val KEYS = mapOf(
            "ENTER" to Key("Enter", "Enter", 13, "\r"),
            "TAB" to Key("Tab", "Tab", 9, "\t"),
            "ESC" to Key("Escape", "Escape", 27),
            "ESCAPE" to Key("Escape", "Escape", 27),
            "UP" to Key("ArrowUp", "ArrowUp", 38),
            "DOWN" to Key("ArrowDown", "ArrowDown", 40),
            "BACKSPACE" to Key("Backspace", "Backspace", 8),
            "DELETE" to Key("Delete", "Delete", 46),
        )

        /** Where JCEF's DevTools endpoint listens; set by the Gradle uiConsole/uiTest tasks. */
        fun debugUrl(): String =
            System.getProperty("foxxycode.cef.debug.url", "http://127.0.0.1:8581")

        /**
         * Connects to the SPA page, retrying until the panel has actually loaded it: the target
         * list is empty while the backend is still starting, and the debug endpoint itself
         * refuses connections until the first JCEF browser is created.
         */
        fun connectWithRetry(timeout: Duration = Duration.ofSeconds(60)): CefChat {
            val deadline = System.nanoTime() + timeout.toNanos()
            var lastError: Exception? = null
            while (System.nanoTime() < deadline) {
                try {
                    return connect()
                } catch (e: Exception) {
                    lastError = e
                    Thread.sleep(1000)
                }
            }
            throw IllegalStateException(
                "no SPA page reachable at ${debugUrl()} within ${timeout.seconds}s — is the sandbox " +
                    "running with the FoxxyCode tool window open?",
                lastError,
            )
        }

        fun connect(): CefChat {
            val http = HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(5)).build()
            val wsUrl = findSpaPage(http)
            val holder = arrayOfNulls<CefChat>(1)
            val listener = object : WebSocket.Listener {
                private val buffer = StringBuilder()
                override fun onText(webSocket: WebSocket, data: CharSequence, last: Boolean): CompletionStage<*>? {
                    buffer.append(data)
                    if (last) {
                        val text = buffer.toString()
                        buffer.setLength(0)
                        holder[0]?.onMessage(text)
                    }
                    webSocket.request(1)
                    return null
                }
            }
            val ws = http.newWebSocketBuilder()
                .connectTimeout(Duration.ofSeconds(5))
                .buildAsync(URI(wsUrl), listener)
                .get(10, TimeUnit.SECONDS)
            return CefChat(ws).also { holder[0] = it }
        }

        /**
         * Picks the SPA out of `/json`: the panel loads the backend URL with `embed=intellij`
         * appended, which no other JCEF page in the IDE carries (DevTools windows and the IDE's
         * own JCEF uses are also listed there).
         */
        private fun findSpaPage(http: HttpClient): String {
            val req = HttpRequest.newBuilder(URI("${debugUrl()}/json")).GET().build()
            val body = http.send(req, HttpResponse.BodyHandlers.ofString()).body()
            val pages = JsonParser.parseString(body).asJsonArray
                .map { it.asJsonObject }
                .filter { it.get("type")?.asString == "page" }
            val spa = pages.firstOrNull { it.get("url")?.asString?.contains("embed=intellij") == true }
                ?: throw IllegalStateException(
                    "no page with embed=intellij among ${pages.size} page(s): " +
                        pages.joinToString { it.get("url")?.asString ?: "?" },
                )
            return spa.get("webSocketDebuggerUrl")?.asString
                ?: throw IllegalStateException("SPA page carries no webSocketDebuggerUrl")
        }
    }
}
