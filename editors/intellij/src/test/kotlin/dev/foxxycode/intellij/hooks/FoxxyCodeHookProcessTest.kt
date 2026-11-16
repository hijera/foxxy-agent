package dev.foxxycode.intellij.hooks

import com.sun.net.httpserver.HttpServer
import java.io.File
import java.net.InetSocketAddress
import java.nio.charset.StandardCharsets
import java.util.concurrent.TimeUnit
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class FoxxyCodeHookProcessTest {
    @Test
    fun `java helper forwards stdin and returns the IDE answer`() {
        val token = "secret"
        var received = ""
        var receivedToken = ""
        val server = HttpServer.create(InetSocketAddress("127.0.0.1", 0), 0)
        server.createContext("/") { exchange ->
            receivedToken = exchange.requestHeaders.getFirst("X-FoxxyCode-Token").orEmpty()
            received = exchange.requestBody.bufferedReader(StandardCharsets.UTF_8).use { it.readText() }
            val answer = """{"hookSpecificOutput":{"handled":true,"toolResult":"from IDE"}}"""
            val bytes = answer.toByteArray(StandardCharsets.UTF_8)
            exchange.sendResponseHeaders(200, bytes.size.toLong())
            exchange.responseBody.use { it.write(bytes) }
        }
        server.start()
        try {
            val java = File(File(System.getProperty("java.home"), "bin"), if (System.getProperty("os.name").startsWith("Windows")) "java.exe" else "java")
            val classPath = File(FoxxyCodeHookProcess::class.java.protectionDomain.codeSource.location.toURI())
            val process = ProcessBuilder(
                java.absolutePath,
                "-cp",
                classPath.absolutePath,
                FoxxyCodeHookProcess::class.java.name,
                "http://127.0.0.1:${server.address.port}/",
                token,
            ).start()
            process.outputStream.use { it.write("""{"hook_event_name":"ToolExecute"}""".toByteArray(StandardCharsets.UTF_8)) }
            val output = process.inputStream.bufferedReader(StandardCharsets.UTF_8).readText()
            assertTrue("hook process did not exit", process.waitFor(10, TimeUnit.SECONDS))
            assertEquals(0, process.exitValue())
            assertEquals("""{"hook_event_name":"ToolExecute"}""", received)
            assertEquals(token, receivedToken)
            assertEquals("""{"hookSpecificOutput":{"handled":true,"toolResult":"from IDE"}}""", output)
        } finally {
            server.stop(0)
        }
    }
}
