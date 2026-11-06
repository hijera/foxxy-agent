package dev.foxxycode.intellij.uitest

import com.google.gson.Gson
import com.intellij.remoterobot.RemoteRobot
import com.intellij.remoterobot.utils.waitFor
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File
import java.nio.file.Files
import java.time.Duration

/** Exercises the real JCEF history with legacy bundles in a non-VCS folder. */
class HistoryUiTest {
    private val gson = Gson()

    @Test
    fun historyFilterStaysBesideSearchAtNarrowWidths() {
        val robot = RemoteRobot(System.getProperty("robot-server.url", "http://127.0.0.1:8580"))
        IdeControl.dismissPopups(robot)
        IdeControl.activateToolWindow(robot, "FoxxyCode")
        CefChat.connectWithRetry().use { chat ->
            chat.eval("location.hash='#/history'; 'history'")
            waitFor(Duration.ofSeconds(15), Duration.ofMillis(100), "history search") {
                chat.eval("!!document.querySelector('[data-testid=sessions-search]')") == "true"
            }
            for (width in listOf(360, 640)) {
                IdeControl.setToolWindowWidth(robot, "FoxxyCode", width)
                for (query in listOf("", "a long search with a clear button")) {
                    chat.click("[data-testid='sessions-search']")
                    chat.eval("document.querySelector('[data-testid=sessions-search]').select()")
                    if (query.isEmpty()) chat.pressKey("BACKSPACE") else chat.insertText(query)
                    Thread.sleep(200)
                    val geometry = chat.eval("""
                        (function(){
                          var s=document.querySelector('[data-testid=sessions-search]').getBoundingClientRect();
                          var f=document.querySelector('[data-testid=sessions-filter-trigger]').getBoundingClientRect();
                          var r=document.querySelector('.sessions-search-row').getBoundingClientRect();
                          return JSON.stringify({aligned:Math.abs((s.top+s.bottom-f.top-f.bottom)/2)<=1,
                            beside:f.left>=s.right,contained:f.right<=r.right,search:s,filter:f});
                        })()
                    """.trimIndent())
                    val result = gson.fromJson(geometry, com.google.gson.JsonObject::class.java)
                    assertTrue("filter left the search row at $width px: $geometry",
                        result.get("aligned").asBoolean && result.get("beside").asBoolean && result.get("contained").asBoolean)
                }
            }
            chat.click("[data-testid='sessions-search-clear']")
        }
    }

    @Test
    fun plainWorkspaceChatsRemainListedAfterSwitchingAndReloading() {
        val robot = RemoteRobot(System.getProperty("robot-server.url", "http://127.0.0.1:8580"))
        IdeControl.dismissPopups(robot)
        IdeControl.activateToolWindow(robot, "FoxxyCode")
        val home = File(System.getProperty("foxxycode.uitest.home", "build/uitest-foxxycode-home"))
        val workspace = Files.createTempDirectory("foxxy-plain-history-").toFile()
        val stamp = System.currentTimeMillis()
        val ids = (1..2).map { "sess_plainhistory${stamp}_$it" }
        for ((index, id) in ids.withIndex()) {
            val dir = File(home, "sessions/$id")
            assertTrue("cannot create the session fixture", dir.mkdirs())
            File(dir, "session.json").writeText(gson.toJson(mapOf(
                "version" to 1, "id" to id, "cwd" to workspace.absolutePath,
                "mode" to "agent", "title" to "Plain history $stamp $index",
                "updatedAt" to "2026-11-03T00:00:00Z",
            )))
            File(dir, "messages.json").writeText(gson.toJson(mapOf(
                "version" to 1,
                "messages" to listOf(
                    mapOf("role" to "user", "content" to "Plain question $stamp $index"),
                    mapOf("role" to "assistant", "content" to "Plain answer $stamp $index"),
                ),
            )))
        }
        CefChat.connectWithRetry().use { chat ->
            fun waitUntil(label: String, expression: String) {
                waitFor(Duration.ofSeconds(15), Duration.ofMillis(100), label) {
                    try {
                        chat.eval(expression) == "true"
                    } catch (error: IllegalStateException) {
                        val detail = error.message.orEmpty()
                        if (!detail.contains("Cannot find context") && !detail.contains("Execution context was destroyed")) throw error
                        false
                    }
                }
            }
            fun openHistory() {
                waitUntil("history navigation", "!!document.querySelector('[data-testid=nav-history]')")
                if (chat.eval("!!document.querySelector('[data-testid=sessions-close]')") != "true") {
                    chat.click("[data-testid='nav-history']")
                }
                waitUntil("history scope control", "!!document.querySelector('[data-testid=sessions-project-only]')")
            }
            fun assertBothRows() {
                val selectors = ids.map { "a[href='#/s/$it']" }
                val encoded = gson.toJson(selectors)
                try {
                    waitUntil("both plain-folder history rows", "$encoded.every(function(s){return !!document.querySelector(s);})")
                } catch (error: Exception) {
                    val state = chat.eval("JSON.stringify({hash:location.hash,scope:document.querySelector('[data-testid=sessions-project-only]')?.checked,rows:Array.from(document.querySelectorAll('a[href*=sess_]')).map(function(a){return a.getAttribute('href');})})")
                    throw AssertionError("Missing $ids from history: $state", error)
                }
            }
            waitUntil("initial navigation", "!!document.querySelector('[data-testid=nav-history]')")
            // The fixtures were written outside the backend, so reopen an already
            // visible drawer to request a fresh listing before asserting on it.
            if (chat.eval("!!document.querySelector('[data-testid=sessions-close]')") == "true") {
                chat.click("[data-testid='sessions-close']")
            }
            openHistory()
            val originallyScoped = chat.eval("document.querySelector('[data-testid=sessions-project-only]').checked") == "true"
            try {
                // This fixture has its own plain folder, outside the host project.
                if (originallyScoped) chat.click("[data-testid='sessions-project-only']")
                assertBothRows()
                for (round in 0 until 6) {
                    val index = round % ids.size
                    chat.click("a[href='#/s/${ids[index]}']")
                    waitUntil("selected plain-folder transcript", "document.body.innerText.indexOf('Plain answer $stamp $index')>=0")
                    openHistory()
                    assertBothRows()
                }
                chat.eval("window.__plainHistoryBeforeReload=true; setTimeout(function(){location.reload();}, 0); 'reloading'")
                waitUntil("reloaded navigation", "window.__plainHistoryBeforeReload!==true && !!document.querySelector('[data-testid=nav-history]')")
                if (chat.eval("!!document.querySelector('[data-testid=sessions-close]')") != "true") openHistory()
                assertBothRows()
                chat.click("[data-testid='sessions-filter-trigger']")
                chat.click("[data-testid='sessions-filter-section-status']")
                chat.click("[data-testid='sessions-filter-status-only']")
                waitUntil("empty archive explains its filter", "!!document.querySelector('[data-testid=sessions-active-status]') && !!document.querySelector('[data-testid=sessions-empty]')")
                chat.eval("window.__plainHistoryBeforeReload=true; setTimeout(function(){location.reload();}, 0); 'reloading'")
                waitUntil("archive filter survives reload visibly", "window.__plainHistoryBeforeReload!==true && !!document.querySelector('[data-testid=sessions-reset-status]')")
                chat.click("[data-testid='sessions-reset-status']")
                assertBothRows()
            } finally {
                openHistory()
                val scopedNow = chat.eval("document.querySelector('[data-testid=sessions-project-only]').checked") == "true"
                if (scopedNow != originallyScoped) chat.click("[data-testid='sessions-project-only']")
            }
        }
    }
}
