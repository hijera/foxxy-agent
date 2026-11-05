package dev.foxxycode.intellij.uitest

import com.intellij.remoterobot.RemoteRobot
import com.intellij.remoterobot.utils.waitFor
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Duration

/** Exercises the real tool window and OS process, including reopening on a fresh port. */
class TabCloseUiTest {
    private val robot = RemoteRobot(System.getProperty("robot-server.url", "http://127.0.0.1:8580"))

    @Test
    fun closingWithTheOptionEnabledUnloadsTheProcessAndReopeningStartsIt() {
        withSetting(true) {
            IdeControl.activateToolWindow(robot, "FoxxyCode")
            val pid = awaitBackend()
            val process = ProcessHandle.of(pid).get()
            val closing = System.nanoTime()
            IdeControl.hideToolWindow(robot, "FoxxyCode")
            assertTrue("closing the panel blocked the IDE", System.nanoTime() - closing < Duration.ofSeconds(3).toNanos())
            waitFor(Duration.ofSeconds(10), Duration.ofMillis(100), "the backend process to exit") {
                !process.isAlive
            }
            assertFalse(robot.callJs<Boolean>("$services manager.isRunning();", true))
            IdeControl.activateToolWindow(robot, "FoxxyCode")
            assertTrue("reopening must create a live backend", ProcessHandle.of(awaitBackend()).get().isAlive)
        }
    }

    @Test
    fun closingWithTheOptionDisabledKeepsTheSameProcess() {
        withSetting(false) {
            IdeControl.activateToolWindow(robot, "FoxxyCode")
            val pid = awaitBackend()
            IdeControl.hideToolWindow(robot, "FoxxyCode")
            assertTrue("closing must preserve the backend", ProcessHandle.of(pid).get().isAlive)
            IdeControl.activateToolWindow(robot, "FoxxyCode")
            org.junit.Assert.assertEquals(pid, awaitBackend())
        }
    }

    @Test
    fun theCheckboxSavesItsValueAndResetRestoresIt() {
        withSetting(false) {
            robot.runJs(
                """
                $services
                var configurable = loader.loadClass('dev.foxxycode.intellij.settings.FoxxyCodeConfigurable').getDeclaredConstructor().newInstance();
                var panel = configurable.createComponent();
                var checkbox = null;
                function visit(component) {
                    if (component instanceof javax.swing.JCheckBox && String(component.getText()).indexOf('Stop the process when') == 0) checkbox = component;
                    if (component instanceof java.awt.Container) {
                        var children = component.getComponents();
                        for (var i = 0; i < children.length; i++) visit(children[i]);
                    }
                }
                visit(panel);
                if (checkbox == null || checkbox.isSelected()) throw new Error('checkbox missing or wrong default');
                checkbox.setSelected(true);
                if (!configurable.isModified()) throw new Error('checkbox change was not detected');
                configurable.apply();
                if (!settings.getStopOnTabClose()) throw new Error('setting was not saved');
                checkbox.setSelected(false);
                configurable.reset();
                if (!checkbox.isSelected() || configurable.isModified()) throw new Error('reset did not restore the saved setting');
                """.trimIndent(),
                true,
            )
        }
    }

    private fun awaitBackend(): Long {
        waitFor(Duration.ofSeconds(60), Duration.ofMillis(200), "the backend to become ready") {
            robot.callJs<Boolean>("$services manager.getBaseUrl() != null && manager.isRunning();", true)
        }
        return robot.callJs<String>(
            """
            $services
            var field = managerClass.getDeclaredField('handler');
            field.setAccessible(true);
            String(field.get(manager).getProcess().pid());
            """.trimIndent(),
            true,
        ).toLong()
    }

    private fun withSetting(enabled: Boolean, action: () -> Unit) {
        IdeControl.waitForProject(robot)
        IdeControl.dismissPopups(robot)
        val previous = robot.callJs<Boolean>("$services settings.getStopOnTabClose();", true)
        try {
            robot.runJs("$services settings.setStopOnTabClose($enabled);", true)
            action()
        } finally {
            robot.runJs("$services settings.setStopOnTabClose($previous);", true)
            IdeControl.activateToolWindow(robot, "FoxxyCode")
        }
    }

    private val services = """
        var project = ${IdeControl.PROJECT};
        var loader = com.intellij.ide.plugins.PluginManagerCore.getPlugin(com.intellij.openapi.extensions.PluginId.getId('dev.foxxycode.intellij')).getPluginClassLoader();
        var settings = com.intellij.openapi.application.ApplicationManager.getApplication().getService(loader.loadClass('dev.foxxycode.intellij.settings.FoxxyCodeSettings')).getState();
        var managerClass = loader.loadClass('dev.foxxycode.intellij.process.FoxxyCodeProcessManager');
        var manager = project.getService(managerClass);
    """.trimIndent()
}
