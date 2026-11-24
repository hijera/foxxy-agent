package dev.foxxycode.intellij.terminal

import org.junit.Assert.assertEquals
import org.junit.Assert.assertSame
import org.junit.Test
import javax.swing.JPanel

class TerminalWidgetFactoryTest {
    class FakeTerminalView {
        val widget = JPanel()
        var directory: String? = null
        var title: String? = null

        fun createLocalShellWidget(workingDirectory: String?, tabName: String): JPanel {
            directory = workingDirectory
            title = tabName
            return widget
        }
    }

    @Test
    fun `working directory precedes tab title in the terminal API`() {
        val view = FakeTerminalView()
        assertSame(view.widget, createLocalShellWidget(view, "/workspace/ui", "FoxxyCode: npm test"))
        assertEquals("/workspace/ui", view.directory)
        assertEquals("FoxxyCode: npm test", view.title)
    }
}
