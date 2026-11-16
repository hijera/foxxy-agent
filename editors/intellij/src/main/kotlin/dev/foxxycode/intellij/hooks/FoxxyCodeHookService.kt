package dev.foxxycode.intellij.hooks

import com.google.gson.JsonObject
import com.google.gson.JsonParser
import com.intellij.openapi.Disposable
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.logger
import com.intellij.openapi.project.Project
import com.intellij.openapi.util.SystemInfo
import com.sun.net.httpserver.HttpServer
import dev.foxxycode.intellij.diff.FoxxyCodeToolExecutor
import dev.foxxycode.intellij.editor.FoxxyCodeEditorContextService
import java.io.File
import java.net.InetSocketAddress
import java.nio.charset.StandardCharsets
import java.util.UUID
import java.util.concurrent.Executors

/** Coddy's command-hook endpoint inside this IDE project. */
class FoxxyCodeHookService(private val project: Project) : Disposable {
    private val log = logger<FoxxyCodeHookService>()
    private val id = UUID.randomUUID().toString()
    private val token = UUID.randomUUID().toString()
    private var server: HttpServer? = null
    private var home: File? = null

    @Synchronized
    fun start(coddyHome: File) {
        if (server != null && home == coddyHome) return
        stop()
        val http = HttpServer.create(InetSocketAddress("127.0.0.1", 0), 0)
        http.executor = Executors.newCachedThreadPool { runnable ->
            Thread(runnable, "foxxycode-coddy-hook").apply { isDaemon = true }
        }
        http.createContext("/") { exchange ->
            val response = try {
                if (exchange.requestHeaders.getFirst("X-FoxxyCode-Token") != token) {
                    exchange.sendResponseHeaders(401, -1)
                    exchange.close()
                    return@createContext
                }
                val payload = exchange.requestBody.bufferedReader(StandardCharsets.UTF_8).use { it.readText() }
                dispatch(JsonParser.parseString(payload).asJsonObject).toString()
            } catch (e: Exception) {
                log.warn("Coddy hook failed", e)
                ""
            }
            if (response.isNotEmpty()) {
                val bytes = response.toByteArray(StandardCharsets.UTF_8)
                exchange.responseHeaders.set("Content-Type", "application/json")
                exchange.sendResponseHeaders(200, bytes.size.toLong())
                exchange.responseBody.use { it.write(bytes) }
            } else {
                exchange.sendResponseHeaders(200, -1)
            }
            exchange.close()
        }
        http.start()
        server = http
        home = coddyHome
        try {
            val classPath = File(FoxxyCodeHookProcess::class.java.protectionDomain.codeSource.location.toURI())
            val executable = if (SystemInfo.isWindows) "java.exe" else "java"
            val java = File(File(System.getProperty("java.home"), "bin"), executable)
            val url = "http://127.0.0.1:${http.address.port}/"
            HookDefinitionInstaller.install(coddyHome, id, java, classPath, url, token)
        } catch (e: Exception) {
            stop()
            throw e
        }
    }

    private fun dispatch(payload: JsonObject): JsonObject {
        if (!HookScope.owns(project.basePath, payload.get("cwd")?.asString)) return JsonObject()
        return when (payload.get("hook_event_name")?.asString) {
            "ToolExecute" -> FoxxyCodeToolExecutor(project).executeHook(payload)
            "FirstPrompt" -> firstPrompt()
            else -> JsonObject()
        }
    }

    private fun firstPrompt(): JsonObject {
        val context = FoxxyCodeEditorContextService.getInstance(project).firstPromptContext()
        if (context.isBlank()) return JsonObject()
        return JsonObject().apply {
            add("hookSpecificOutput", JsonObject().apply {
                addProperty("hookEventName", "FirstPrompt")
                addProperty("additionalContext", context)
            })
        }
    }

    @Synchronized
    fun stop() {
        val oldHome = home
        home = null
        server?.stop(0)
        server = null
        if (oldHome != null) {
            try {
                HookDefinitionInstaller.remove(oldHome, id)
            } catch (e: Exception) {
                log.warn("Could not remove Coddy IDE hooks from $oldHome", e)
            }
        }
    }

    override fun dispose() = stop()

    companion object {
        const val HOOKS_FILE = "hooks.json"

        fun getInstance(project: Project): FoxxyCodeHookService =
            project.getService(FoxxyCodeHookService::class.java)
    }
}
