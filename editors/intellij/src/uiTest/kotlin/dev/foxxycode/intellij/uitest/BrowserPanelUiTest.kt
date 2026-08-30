package dev.foxxycode.intellij.uitest

import com.intellij.remoterobot.RemoteRobot
import com.intellij.remoterobot.utils.waitFor
import dev.foxxycode.intellij.uitest.fixtures.FoxxyCodeToolWindowFixture
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
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
