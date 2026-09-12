package dev.foxxycode.intellij.uitest

import com.intellij.remoterobot.RemoteRobot
import com.intellij.remoterobot.utils.waitFor
import dev.foxxycode.intellij.uitest.fixtures.FoxxyCodeToolWindowFixture
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.io.File
import java.time.Duration

/**
 * Regression tests for the FoxxyCode tool window as the user first meets it: the window opens
 * without a modal in the way, the toolbar carries every action, the panel reaches the SPA rather
 * than its start-error card, and closing/reopening does not leave a dead panel behind.
 *
 * Deliberately narrow on the Swing side: Remote Robot cannot see inside JCEF (see
 * [FoxxyCodeToolWindowFixture]). What the chat itself renders is asserted through [CefChat]
 * (Chrome DevTools Protocol) instead — real DOM, not pixels. Visual judgement calls still
 * belong in a `uiConsole` script whose screenshots a human (or an agent) reads.
 *
 * Requires a sandbox IDE already running: `gradlew runIdeForUiTests` (see the
 * intellij-plugin-uitest skill).
 */
class BrowserPanelUiTest {

    private val robot = RemoteRobot(System.getProperty("robot-server.url", "http://127.0.0.1:8580"))

    @Before
    fun openToolWindow() {
        IdeControl.bringIdeToFront(robot)
        IdeControl.activateToolWindow(robot, TOOL_WINDOW_ID)
        // The first open builds the panel lazily; wait for it to exist.
        FoxxyCodeToolWindowFixture.find(robot)
    }

    @Test
    fun theToolbarCarriesEveryAction() {
        val panel = FoxxyCodeToolWindowFixture.find(robot)
        for (icon in FoxxyCodeToolWindowFixture.TOOLBAR_ICONS) {
            assertTrue("toolbar is missing $icon", panel.hasToolbarButton(icon))
        }
    }

    @Test
    fun theBackendStartsAndTheBrowserSurfaceIsShown() {
        val panel = FoxxyCodeToolWindowFixture.find(robot)
        // Starting the bundled binary and polling it to readiness takes a few seconds on a cold
        // sandbox, and the panel shows a plain status label until then.
        waitFor(Duration.ofSeconds(60), Duration.ofMillis(500), "the browser surface to appear") {
            panel.browserSurfaces().isNotEmpty()
        }
        assertFalse("the panel is showing its start-error card", panel.hasStartError())

        val surface = panel.browserSurfaces().first()
        val size = surface.callJs<String>("component.getWidth() + 'x' + component.getHeight()", true)
        assertFalse("the browser surface has no size ($size)", size.startsWith("0x") || size.endsWith("x0"))
    }

    /**
     * Crosses the Swing boundary: the SPA must have actually mounted inside JCEF — React
     * rendered into `#root`, the composer exists, and the plugin's error overlay
     * (`injectBootstrap` in FoxxyCodeBrowserPanel) is not showing. Catches the "browser surface
     * is up but the page is blank/crashed" class of failure the Swing-side tests cannot see.
     */
    @Test
    fun theSpaMountsInsideTheBrowser() {
        val panel = FoxxyCodeToolWindowFixture.find(robot)
        waitFor(Duration.ofSeconds(60), Duration.ofMillis(500), "the browser surface to appear") {
            panel.browserSurfaces().isNotEmpty()
        }
        CefChat.connectWithRetry().use { chat ->
            waitFor(Duration.ofSeconds(60), Duration.ofSeconds(1), "the SPA to mount into #root") {
                chat.eval(
                    "(function(){var r=document.getElementById('root');return !!(r&&r.childElementCount>0);})()"
                ) == "true"
            }
            assertTrue(
                "the composer textarea is missing from the SPA",
                chat.eval("!!document.getElementById('composer')") == "true",
            )
            assertFalse(
                "the FoxxyCode UI error overlay is showing",
                chat.eval("!!document.getElementById('foxxycode-err-overlay')") == "true",
            )
        }
    }

    /**
     * JCEF is Chromium 104, which raises "ResizeObserver loop limit exceeded" as a window
     * error whenever a resize observation re-layouts what it has just measured. Opening an
     * old transcript did exactly that (the observer that sizes the scroll tail after the
     * composer), and the plugin's overlay painted the notice over the chat. Seeds a long
     * transcript into the sandbox backend's home and opens it at a tool-window width of
     * 360 px: neither the raw error event nor the overlay may appear. Twin of
     * features/ide_panel_resize_loop.feature, which runs in a current Chrome.
     */
    @Test
    fun anOldTranscriptOpensWithoutAResizeObserverLoop() {
        val sessionId = seedLongTranscript()
        val panel = FoxxyCodeToolWindowFixture.find(robot)
        waitFor(Duration.ofSeconds(60), Duration.ofMillis(500), "the browser surface to appear") {
            panel.browserSurfaces().isNotEmpty()
        }
        IdeControl.setToolWindowWidth(robot, TOOL_WINDOW_ID, 360)
        CefChat.connectWithRetry().use { chat ->
            waitFor(Duration.ofSeconds(60), Duration.ofSeconds(1), "the SPA to mount into #root") {
                chat.eval(
                    "(function(){var r=document.getElementById('root');return !!(r&&r.childElementCount>0);})()"
                ) == "true"
            }
            chat.eval(
                "(function(){window.__roErrs=[];window.addEventListener('error',function(e){" +
                    "if(/ResizeObserver loop/.test(e.message||''))window.__roErrs.push(e.message)});return 'armed'})()"
            )
            // The way a user reaches an old dialog: through the history list, which also
            // makes the SPA fetch the session list the transcript route resolves against.
            chat.eval("(function(){location.hash='#/history';return 'history'})()")
            // The list is filtered by workspace, so the seeded chat need not be visible;
            // the route only has to have fetched it.
            Thread.sleep(1500)
            chat.eval("(function(){location.hash='#/s/$sessionId';return 'nav'})()")
            waitFor(Duration.ofSeconds(30), Duration.ofMillis(500), "the transcript's tool rows to render") {
                (chat.eval("document.querySelectorAll('#messages details').length").toIntOrNull() ?: 0) >= 10
            }
            // Let the layout settle through a few more frames before judging.
            Thread.sleep(1500)
            val errors = chat.eval("JSON.stringify(window.__roErrs)")
            assertEquals("the page raised ResizeObserver loop errors: $errors", "[]", errors)
            assertFalse(
                "the FoxxyCode UI error overlay is showing",
                chat.eval("!!document.getElementById('foxxycode-err-overlay')") == "true",
            )
        }
    }

    /**
     * Writes a session bundle with 60 tool-call exchanges into the sandbox backend's home
     * (`build/uitest-foxxycode-home`, the working directory is the plugin project) and
     * returns its id. The backend lists sessions from disk, so no restart is needed.
     */
    private fun seedLongTranscript(): String {
        // Unique per run: the backend keeps loaded sessions in memory, so reusing an id
        // would serve a stale transcript from an earlier run.
        val id = "sess_uitestlong" + System.currentTimeMillis()
        val dir = File("build/uitest-foxxycode-home/sessions/$id")
        dir.mkdirs()
        val cwd = File(".").absolutePath.replace("\\", "/")
        File(dir, "session.json").writeText(
            """{"version":1,"id":"$id","cwd":"$cwd","mode":"agent","title":"long transcript","updatedAt":"2026-09-01T10:00:00Z"}"""
        )
        val rows = StringBuilder()
        for (i in 0 until 60) {
            if (i > 0) rows.append(',')
            rows.append(
                """{"role":"user","content":"Read notes.txt and report line $i."},""" +
                    """{"role":"assistant","content":"Reading line $i.","tool_calls":[{"id":"call_$i","name":"read","input":"{\"path\":\"notes.txt\"}"}]},""" +
                    """{"role":"tool","content":"MARKER-LINE-$i","tool_call_id":"call_$i"},""" +
                    """{"role":"assistant","content":"Line $i carries the marker."}"""
            )
        }
        File(dir, "messages.json").writeText("""{"version":1,"messages":[$rows]}""")
        return id
    }

    @Test
    fun reopeningTheToolWindowKeepsAWorkingPanel() {
        IdeControl.hideToolWindow(robot, TOOL_WINDOW_ID)
        IdeControl.activateToolWindow(robot, TOOL_WINDOW_ID)

        // Re-find: hiding may dispose the content, so the old fixture reference can be stale.
        val panel = FoxxyCodeToolWindowFixture.find(robot)
        waitFor(Duration.ofSeconds(30), Duration.ofMillis(300), "the panel to come back") {
            panel.hasToolbarButton(FoxxyCodeToolWindowFixture.ICON_RESTART) &&
                panel.browserSurfaces().isNotEmpty()
        }
        assertFalse("the panel is showing its start-error card", panel.hasStartError())
    }

    private companion object {
        const val TOOL_WINDOW_ID = "FoxxyCode"
    }
}
