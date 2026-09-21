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

    /**
     * Pids of the IDE's JVM (first) and of everything it spawned: the `foxxycode` backend, JCEF
     * helpers. Read before asking the IDE to exit, so whatever outlives the shutdown can be named
     * and cleaned up. Off the EDT: nothing here touches Swing.
     *
     * Rhino hands a Java `long` over as a JS number, so a pid can come back as `52528.0`
     * (`String.valueOf` picks its `double` overload); hence the parse through Double.
     */
    fun processTree(robot: RemoteRobot): List<Long> = robot.callJs<String>(
        """
        var self = java.lang.ProcessHandle.current();
        var out = '' + self.pid();
        var it = self.descendants().iterator();
        while (it.hasNext()) out += ',' + it.next().pid();
        out;
        """.trimIndent(),
        false,
    ).split(',').map { it.trim().toDouble().toLong() }

    /**
     * Asks the IDE to quit, skipping the "Are you sure you want to exit?" dialog (`exitConfirmed`)
     * and the `canExit` vetoes such as "terminate the running process?" (`force`). The shutdown
     * still goes through `appWillBeClosed`, so the plugin reaps its backend as on a normal exit.
     *
     * Scheduled with `invokeLater` so this robot call returns before the shutdown starts. The
     * `Exit` action instead runs inside robot-server's own request: it parks that request on the
     * modal confirmation until a human clicks, then tears the IDE down underneath it, which has
     * left a JVM running with no window.
     */
    fun requestExit(robot: RemoteRobot) = robot.runJs(
        """
        var app = com.intellij.openapi.application.ApplicationManager.getApplication();
        app.invokeLater(new java.lang.Runnable({
            run: function () {
                com.intellij.openapi.application.ex.ApplicationManagerEx.getApplicationEx().exit(true, true);
            }
        }));
        """.trimIndent(),
        false,
    )

    /**
     * What is popping up over the IDE right now, one line each: dialogs (title, buttons, text),
     * notifications (group id, title, content: the balloons in the corner and their entries in
     * the Notifications tool window) and whether a popup (menu, list, completion) is open.
     */
    fun listPopups(robot: RemoteRobot): List<String> = popups(robot, dismiss = false)

    /**
     * Closes everything [listPopups] reports and returns the same lines. Dialogs are cancelled,
     * as Esc would (`DialogWrapper.doCancelAction`, else a WINDOW_CLOSING event), because cancel
     * commits to nothing: a "Terminate the process?" is left unanswered, not answered yes.
     * Notifications are expired, which takes their balloons down; popups are closed.
     */
    fun dismissPopups(robot: RemoteRobot): List<String> = popups(robot, dismiss = true)

    /** Only notifications: the noise sweep that is safe to run without looking first. */
    fun dismissNotifications(robot: RemoteRobot): List<String> =
        popups(robot, dismiss = true, dialogs = false, menus = false)

    private fun popups(
        robot: RemoteRobot,
        dismiss: Boolean,
        dialogs: Boolean = true,
        menus: Boolean = true,
    ): List<String> = robot.callJs<String>(
        """
        var dismiss = $dismiss;
        var lines = [];
        function flat(s) {
            if (s == null) return '';
            var t = String(s).replace(/<[^>]*>/g, ' ').replace(/&nbsp;/g, ' ').replace(/\s+/g, ' ');
            t = t.replace(/^ +| +$/g, '');
            return t.length > 200 ? t.substring(0, 200) + '...' : t;
        }
        function collect(c, buttons, texts) {
            if (!c.isShowing()) return;
            if (c instanceof javax.swing.AbstractButton) {
                var b = flat(c.getText());
                if (b.length > 0) buttons.push(b);
            } else if (c instanceof javax.swing.JLabel || c instanceof javax.swing.text.JTextComponent) {
                var t = flat(c.getText());
                if (t.length > 0) texts.push(t);
            }
            if (c instanceof java.awt.Container) {
                var kids = c.getComponents();
                for (var k = 0; k < kids.length; k++) collect(kids[k], buttons, texts);
            }
        }
        if ($dialogs) {
            // Newest first: a dialog opened from another dialog has to go before its parent.
            var windows = java.awt.Window.getWindows();
            for (var i = windows.length - 1; i >= 0; i--) {
                var w = windows[i];
                if (!(w instanceof java.awt.Dialog) || !w.isShowing()) continue;
                // A floating tool window is a JDialog too, and is not a popup.
                if (String(w.getClass().getName()).match(/Decorator${'$'}/)) continue;
                var buttons = [], texts = [];
                collect(w, buttons, texts);
                lines.push('dialog "' + flat(w.getTitle()) + '"' +
                    (buttons.length > 0 ? ' [' + buttons.join(' | ') + ']' : '') +
                    (texts.length > 0 ? ': ' + texts.join(' / ') : ''));
                if (dismiss) {
                    var wrapper = com.intellij.openapi.ui.DialogWrapper.findInstance(w);
                    if (wrapper != null) wrapper.doCancelAction();
                    else w.dispatchEvent(new java.awt.event.WindowEvent(w, java.awt.event.WindowEvent.WINDOW_CLOSING));
                }
            }
        }
        var manager = com.intellij.notification.NotificationsManager.getNotificationsManager();
        var seen = new java.util.HashSet();
        var scopes = [null];
        var projects = com.intellij.openapi.project.ProjectManager.getInstance().getOpenProjects();
        for (var p = 0; p < projects.length; p++) scopes.push(projects[p]);
        for (var s = 0; s < scopes.length; s++) {
            var notes = manager.getNotificationsOfType(com.intellij.notification.Notification, scopes[s]);
            for (var n = 0; n < notes.length; n++) {
                var note = notes[n];
                if (note.isExpired() || !seen.add(note)) continue;
                lines.push('notification [' + note.getGroupId() + '] ' + flat(note.getTitle()) + ': ' + flat(note.getContent()));
                if (dismiss) note.expire();
            }
        }
        if ($menus) {
            // IdePopupManager is not reachable from here; these two are the public way in.
            var factory = com.intellij.openapi.ui.popup.JBPopupFactory.getInstance();
            if (factory.isPopupActive()) {
                lines.push('popup (a list, chooser or completion is open)');
                if (dismiss) {
                    var dispatcher = com.intellij.openapi.ui.popup.StackingPopupDispatcher.getInstance();
                    for (var guard = 0; guard < 10 && factory.isPopupActive(); guard++) {
                        if (!dispatcher.closeActivePopup()) break;
                    }
                }
            }
            // A right-click menu is a plain Swing JPopupMenu, outside the JBPopup stack.
            var menus = javax.swing.MenuSelectionManager.defaultManager();
            if (menus.getSelectedPath().length > 0) {
                lines.push('menu (a context or main menu is open)');
                if (dismiss) menus.clearSelectedPath();
            }
        }
        lines.join('\n');
        """.trimIndent(),
        true,
    ).lines().filter { it.isNotBlank() }

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
