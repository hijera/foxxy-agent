package dev.foxxycode.intellij.uitest.fixtures

import com.intellij.remoterobot.RemoteRobot
import com.intellij.remoterobot.data.RemoteComponent
import com.intellij.remoterobot.fixtures.CommonContainerFixture
import com.intellij.remoterobot.fixtures.ComponentFixture
import com.intellij.remoterobot.fixtures.DefaultXpath
import com.intellij.remoterobot.fixtures.FixtureName
import com.intellij.remoterobot.search.locators.byXpath
import java.time.Duration

/**
 * PageObject for the FoxxyCode tool window content ([dev.foxxycode.intellij.ui.FoxxyCodeBrowserPanel]).
 *
 * Reach: the panel is a toolbar plus a JCEF browser, and the browser's content is not Swing —
 * everything the chat renders (composer, messages, mention popup) is invisible to XPath. This
 * fixture therefore covers the toolbar and the panel's own state cards; anything inside the chat
 * has to be checked on a screenshot or through JS (see the skill).
 *
 * Toolbar buttons are located by `myicon`, not by accessible name: the names come from
 * FoxxyCodeBundle and follow the backend's `ui.locale`, so a name-based locator breaks the moment
 * the sandbox comes up in another language. Icons are the same in every locale. The icon constants
 * mirror the `AllIcons` used in FoxxyCodeBrowserPanel.createToolbar() — if a button's icon changes
 * there, change it here.
 */
@FixtureName("FoxxyCode tool window")
@DefaultXpath("type", "//div[@class='FoxxyCodeBrowserPanel']")
class FoxxyCodeToolWindowFixture(
    remoteRobot: RemoteRobot,
    remoteComponent: RemoteComponent,
) : CommonContainerFixture(remoteRobot, remoteComponent) {

    companion object {
        const val ICON_RESTART = "restart.svg"
        const val ICON_RELOAD = "refresh.svg"
        const val ICON_OPEN_BROWSER = "web.svg"
        const val ICON_DEVTOOLS = "console.svg"
        const val ICON_SETTINGS = "settings.svg"

        /** Every toolbar action the panel registers, in the order createToolbar() adds them. */
        val TOOLBAR_ICONS = listOf(
            ICON_RESTART,
            ICON_RELOAD,
            ICON_OPEN_BROWSER,
            ICON_DEVTOOLS,
            ICON_SETTINGS,
        )

        fun find(robot: RemoteRobot, timeout: Duration = Duration.ofSeconds(15)): FoxxyCodeToolWindowFixture =
            robot.find(FoxxyCodeToolWindowFixture::class.java, timeout)
    }

    fun toolbarButton(icon: String): ComponentFixture =
        find(ComponentFixture::class.java, byXpath("//div[@myicon='$icon']"))

    fun hasToolbarButton(icon: String): Boolean =
        findAll(ComponentFixture::class.java, byXpath("//div[@myicon='$icon']")).isNotEmpty()

    /**
     * The JCEF surface hosting the SPA, when the browser card is showing.
     *
     * The class depends on the IDE runtime: windowed JCEF renders into a heavyweight
     * `BrowserCanvas` (what PhpStorm/PyCharm 2022.2 use here), off-screen rendering into a
     * `JBCefOsrComponent`. Both are accepted rather than pinning the one this machine happens
     * to use; `discover.uiscript` prints whichever is live.
     */
    fun browserSurfaces(): List<ComponentFixture> =
        findAll(
            ComponentFixture::class.java,
            byXpath(
                "//div[@class='BrowserCanvas' or @class='JBCefOsrComponent' " +
                    "or @class='JBCefBrowserComponent']",
            ),
        )

    /** True when the panel is showing its "failed to start" card instead of the browser. */
    fun hasStartError(): Boolean =
        findAll(
            ComponentFixture::class.java,
            byXpath("//div[contains(@visible_text,'failed to start')]"),
        ).isNotEmpty()
}
