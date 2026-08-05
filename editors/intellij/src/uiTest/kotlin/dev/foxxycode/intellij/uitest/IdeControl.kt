package dev.foxxycode.intellij.uitest

import com.intellij.remoterobot.RemoteRobot
import com.intellij.remoterobot.utils.waitFor
import java.time.Duration

/**
 * IDE-side operations shared by [UiConsole] and the JUnit UI tests.
 *
 * Everything here goes through `runJs` on the EDT (the second argument): these scripts all touch
 * Swing or need a `DataContext`, and off the EDT the platform throws
 * `EventQueue.isDispatchThread()=false`.
 *
 * Rhino on the IDE side is ES5: `var` only, string concatenation, no lambdas.
 */
object IdeControl {

    /** The sandbox always opens exactly one project, so index 0 is unambiguous. */
    const val PROJECT =
        "com.intellij.openapi.project.ProjectManager.getInstance().getOpenProjects()[0]"

    /**
     * Blocks until the sandbox has an open project. robot-server answers well before the project
     * finishes opening, so anything using [PROJECT] right after startup would otherwise die with
     * "Can't find method ToolWindowManager.getInstance(org.mozilla.javascript.Undefined)".
     */
    fun waitForProject(robot: RemoteRobot, timeout: Duration = Duration.ofMinutes(2)) {
        waitFor(timeout, Duration.ofSeconds(1), "the sandbox to open a project") {
            robot.callJs<Boolean>(
                "com.intellij.openapi.project.ProjectManager.getInstance().getOpenProjects().length > 0"
            )
        }
    }

    /**
     * Un-minimises the IDE frame and raises it above other windows.
     *
     * java.awt.Robot clicks land on whatever occupies the coordinates *on screen* — if the
     * sandbox has drifted behind a browser (it will, between script runs on a developer machine),
     * every click silently hits the wrong application. Run this before any mouse interaction.
     * The always-on-top pulse is the standard workaround for Windows' foreground-lock, which
     * ignores a plain `toFront()` from a non-foreground process.
     */
    fun bringIdeToFront(robot: RemoteRobot) = robot.runJs(
        """
        var frames = java.awt.Frame.getFrames();
        for (var i = 0; i < frames.length; i++) {
            var f = frames[i];
            if (!f.isShowing()) continue;
            f.setExtendedState(f.getExtendedState() & ~java.awt.Frame.ICONIFIED);
            f.setAlwaysOnTop(true);
            f.toFront();
            f.requestFocus();
            f.setAlwaysOnTop(false);
        }
        """.trimIndent(),
        true,
    )

    /** Opens and focuses a tool window by its `plugin.xml` id, e.g. `FoxxyCode`. */
    fun activateToolWindow(robot: RemoteRobot, id: String) {
        waitForProject(robot)
        robot.runJs(
            """
            var project = $PROJECT;
            var tw = com.intellij.openapi.wm.ToolWindowManager.getInstance(project).getToolWindow('$id');
            if (tw == null) throw new java.lang.IllegalStateException('no tool window with id $id');
            tw.activate(null);
            """.trimIndent(),
            true,
        )
    }

    /** Hides a tool window — the other half of the "does the choice survive reopening" check. */
    fun hideToolWindow(robot: RemoteRobot, id: String) = robot.runJs(
        """
        var project = $PROJECT;
        var tw = com.intellij.openapi.wm.ToolWindowManager.getInstance(project).getToolWindow('$id');
        if (tw == null) throw new java.lang.IllegalStateException('no tool window with id $id');
        tw.hide(null);
        """.trimIndent(),
        true,
    )

    /**
     * Fires an action by its id, with a real DataContext from the focused component. Cheaper and
     * far less brittle than walking menus — prefer `FoxxyCodeAddFile` over four context clicks.
     */
    fun invokeAction(robot: RemoteRobot, id: String) = robot.runJs(
        """
        var am = com.intellij.openapi.actionSystem.ActionManager.getInstance();
        var action = am.getAction('$id');
        if (action == null) throw new java.lang.IllegalStateException('no action with id $id');
        var focus = java.awt.KeyboardFocusManager.getCurrentKeyboardFocusManager().getFocusOwner();
        var context = com.intellij.ide.DataManager.getInstance().getDataContext(focus);
        var event = com.intellij.openapi.actionSystem.AnActionEvent.createFromAnAction(
            action, null, com.intellij.openapi.actionSystem.ActionPlaces.UNKNOWN, context);
        action.actionPerformed(event);
        """.trimIndent(),
        true,
    )

    /**
     * Resizes a tool window to an absolute pixel width. `stretchWidth` takes a delta, so the
     * current width is measured first. This is the "nothing may scroll horizontally at ~320 px"
     * probe from the visual checklist.
     */
    fun setToolWindowWidth(robot: RemoteRobot, id: String, px: Int) = robot.runJs(
        """
        var project = $PROJECT;
        var tw = com.intellij.openapi.wm.ToolWindowManager.getInstance(project).getToolWindow('$id');
        if (tw == null) throw new java.lang.IllegalStateException('no tool window with id $id');
        var current = tw.getComponent().getWidth();
        tw.stretchWidth($px - current);
        """.trimIndent(),
        true,
    )

    /** Switches the IDE theme; a hardcoded color in the plugin's UI shows up immediately. */
    fun setTheme(robot: RemoteRobot, dark: Boolean) {
        val match = if (dark) "darcula" else "light"
        robot.runJs(
            """
            var laf = com.intellij.ide.ui.LafManager.getInstance();
            var all = laf.getInstalledLookAndFeels();
            var chosen = null;
            for (var i = 0; i < all.length; i++) {
                var n = all[i].getName().toLowerCase();
                if (n.indexOf('$match') >= 0 && n.indexOf('high contrast') < 0) { chosen = all[i]; break; }
            }
            if (chosen == null) throw new java.lang.IllegalStateException('no $match theme installed');
            laf.setCurrentLookAndFeel(chosen);
            laf.updateUI();
            """.trimIndent(),
            true,
        )
    }
}
