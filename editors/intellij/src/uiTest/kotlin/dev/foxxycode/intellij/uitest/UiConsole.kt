package dev.foxxycode.intellij.uitest

import com.intellij.remoterobot.RemoteRobot
import com.intellij.remoterobot.fixtures.ComponentFixture
import com.intellij.remoterobot.search.locators.byXpath
import com.intellij.remoterobot.utils.Keyboard
import com.intellij.remoterobot.utils.waitFor
import java.awt.event.KeyEvent
import java.awt.image.BufferedImage
import java.io.File
import java.time.Duration
import javax.imageio.ImageIO

/**
 * Runs a UI script against the sandbox IDE started by `runIdeForUiTests`.
 *
 * This exists so an agent can drive the plugin's UI in one Gradle round-trip instead of one per
 * click: it writes a script, runs `gradlew uiConsole -PuiScript=<file>`, and then *looks* at the
 * numbered screenshots the run leaves in `build/uiconsole/`.
 *
 * Scripts are line-based. Blank lines and `#` comments are ignored. See the command table in
 * `.claude/skills/intellij-plugin-uitest/SKILL.md`, and `uitest-scripts/` for worked examples.
 *
 * Every interaction (click, type, key, action, toolwindow, theme, width) is followed by an
 * automatic screenshot, and any failure screenshots before it rethrows — so the trace shows what
 * the IDE looked like at the moment things went wrong, not after.
 *
 * Reach: every locator-based command walks the *Swing* hierarchy. The FoxxyCode chat lives in a
 * JCEF browser, whose content has no Swing components at all — `tree`, `assert-text` and `click`
 * reach the toolbar, the tool window chrome, IDE dialogs and popups, but never the composer or a
 * message. For those, read the screenshot.
 */
fun main(args: Array<String>) {
    if (args.size < 2) {
        System.err.println("usage: UiConsole <script> <outputDir> [robotUrl]")
        kotlin.system.exitProcess(2)
    }
    val script = File(args[0])
    require(script.isFile) { "script not found: ${script.absolutePath}" }
    val outputDir = File(args[1])
    val url = args.getOrElse(2) { "http://127.0.0.1:8580" }

    // Wipe first: stale PNGs from a previous run are worse than none, because they look current.
    if (outputDir.exists()) {
        outputDir.listFiles()?.forEach { it.delete() }
    }
    outputDir.mkdirs()

    val robot = RemoteRobot(url)
    // Every mouse interaction assumes the sandbox is the frontmost window; between runs on a
    // developer machine it rarely still is.
    IdeControl.bringIdeToFront(robot)

    val console = UiConsole(robot, outputDir)
    try {
        console.run(script.readLines())
    } finally {
        console.close()
    }
    if (console.failed) kotlin.system.exitProcess(1)
}

private class UiConsole(private val robot: RemoteRobot, private val outputDir: File) {

    private val keyboard = Keyboard(robot)
    private val log = StringBuilder()
    private var step = 0
    var failed = false
        private set

    /**
     * Every script here touches Swing, and `runJs`/`callJs` default to a background thread — which
     * fails with `EventQueue.isDispatchThread()=false`. Route everything through the EDT.
     */
    private fun js(script: String) = robot.runJs(script, true)

    /**
     * Runs on an IDE background thread instead of the EDT. Needed for anything that *drives*
     * input rather than reading state: `java.awt.Robot` posts events the EDT has to process, so
     * a Robot gesture started on the EDT deadlocks until its own timeout. Scripts using this
     * must not touch Swing state directly.
     */
    private fun jsOffEdt(script: String) = robot.runJs(script, false)

    private fun jsString(script: String): String = robot.callJs(script, true)

    fun run(lines: List<String>) {
        for (raw in lines) {
            val line = raw.trim()
            if (line.isEmpty() || line.startsWith("#")) continue
            step++
            say("[$step] $line")
            try {
                execute(line)
            } catch (e: Throwable) {
                failed = true
                say("    FAILED: ${e.javaClass.simpleName}: ${e.message}")
                screenshot("FAILED")
                return
            }
        }
    }

    fun close() {
        File(outputDir, "console.log").writeText(log.toString())
    }

    private fun execute(line: String) {
        val command = line.substringBefore(' ')
        val rest = line.substringAfter(' ', "").trim()
        when (command) {
            "toolwindow" -> { activateToolWindow(rest); shot(command) }
            "toolwindow-hide" -> { IdeControl.hideToolWindow(robot, rest); shot(command) }
            "action" -> { invokeAction(rest); shot(command) }
            "click" -> { find(rest).click(); shot(command) }
            "doubleclick" -> { find(rest).doubleClick(); shot(command) }
            "rightclick" -> { find(rest).rightClick(); shot(command) }
            "type" -> { typeText(rest); shot(command) }
            "keys" -> { keyboard.enterText(rest); shot(command) }
            "key" -> { pressKeys(rest); shot(command) }
            "width" -> { setWidth(rest); shot(command) }
            "theme" -> { setTheme(rest); shot(command) }
            "js" -> { js(rest); shot(command) }
            "js-bg" -> { jsOffEdt(rest); shot(command) }
            "drag" -> { dragProjectFileToPanel(rest); shot(command) }
            "wait" -> awaitComponent(rest)
            "sleep" -> Thread.sleep((rest.toDouble() * 1000).toLong())
            "tree" -> dumpTree(rest)
            "text" -> say(indent(visibleText(DEFAULT_ROOT).joinToString("\n")))
            "assert-text" -> assertText(rest, expected = true)
            "assert-no-text" -> assertText(rest, expected = false)
            "screenshot" -> screenshot(if (rest.isEmpty()) "screenshot" else rest)
            else -> throw IllegalArgumentException("unknown command '$command'")
        }
    }

    // ---------------------------------------------------------------- components

    private fun find(xpath: String): ComponentFixture =
        robot.find(ComponentFixture::class.java, byXpath(xpath), DEFAULT_TIMEOUT)

    private fun awaitComponent(rest: String) {
        // "wait <xpath> [seconds]" — the seconds are optional and always come last.
        val parts = rest.split(' ')
        val seconds = parts.last().toLongOrNull()
        val xpath = if (seconds == null) rest else parts.dropLast(1).joinToString(" ")
        val timeout = Duration.ofSeconds(seconds ?: DEFAULT_TIMEOUT.seconds)
        waitFor(timeout, Duration.ofMillis(500), "component $xpath to appear") {
            robot.findAll(ComponentFixture::class.java, byXpath(xpath)).isNotEmpty()
        }
    }

    /**
     * `drag <fileName>` — drags a file from the Project view onto the FoxxyCode panel with a
     * real mouse gesture.
     *
     * There is no way to fake this: the whole point of the check is whether IntelliJ's DnD
     * machinery reaches a target sitting under a heavyweight JCEF surface, so the gesture has
     * to go through `java.awt.Robot` at real screen coordinates. Coordinates are measured on
     * the EDT and stashed in a system property; the gesture itself runs off the EDT, because
     * Robot needs the EDT free to process the events it posts.
     */
    private fun dragProjectFileToPanel(rest: String) {
        val parts = rest.trim().split(Regex("\\s+"))
        val fileName = parts.firstOrNull().orEmpty()
        // A second argument re-targets the drop. Dropping onto the editor is the control
        // experiment: it is known to work, so if that does nothing either, the synthetic
        // gesture is the problem rather than whatever is being tested.
        val target = parts.getOrNull(1) ?: "FoxxyCodeBrowserPanel"
        require(fileName.isNotBlank()) { "drag needs a file name, e.g. `drag hello.txt`" }
        require("'" !in fileName && "'" !in target) { "drag arguments cannot contain a single quote" }
        js(
            """
            // Match up the superclass chain: the Project view's tree is an *anonymous*
            // subclass of ProjectViewTree, and an anonymous class has an empty simple name.
            function isA(component, simpleName) {
                var c = component.getClass();
                while (c != null) {
                    if (c.getSimpleName() == simpleName) return true;
                    c = c.getSuperclass();
                }
                return false;
            }
            function collect(root, simpleName, sink) {
                if (root == null) return;
                // Only showing components: a hidden one has no screen location, and the
                // hierarchy keeps stale editors and collapsed tool windows around.
                if (isA(root, simpleName) && root.isShowing()) sink.add(root);
                if (root instanceof java.awt.Container) {
                    var kids = root.getComponents();
                    for (var i = 0; i < kids.length; i++) collect(kids[i], simpleName, sink);
                }
            }
            var trees = new java.util.ArrayList();
            var panels = new java.util.ArrayList();
            var frames = java.awt.Frame.getFrames();
            for (var f = 0; f < frames.length; f++) {
                if (!frames[f].isShowing()) continue;
                collect(frames[f], 'ProjectViewTree', trees);
                collect(frames[f], '$target', panels);
            }
            if (trees.size() == 0) throw new java.lang.IllegalStateException('no ProjectViewTree is showing');
            if (panels.size() == 0) throw new java.lang.IllegalStateException('no $target is showing');
            var tree = trees.get(0);
            var row = -1;
            for (var r = 0; r < tree.getRowCount(); r++) {
                var path = tree.getPathForRow(r);
                if (path == null) continue;
                if (String(path.getLastPathComponent().toString()).indexOf('$fileName') >= 0) { row = r; break; }
            }
            if (row < 0) throw new java.lang.IllegalStateException('no Project view row for $fileName');
            var bounds = tree.getRowBounds(row);
            var treeAt = tree.getLocationOnScreen();
            var panel = panels.get(0);
            var panelAt = panel.getLocationOnScreen();
            java.lang.System.setProperty('foxxycode.uitest.drag',
                (treeAt.x + bounds.x + 30) + ',' + (treeAt.y + bounds.y + bounds.height / 2) + ',' +
                (panelAt.x + panel.getWidth() / 2) + ',' + (panelAt.y + panel.getHeight() / 2));
            """.trimIndent(),
        )
        jsOffEdt(
            """
            var parts = java.lang.System.getProperty('foxxycode.uitest.drag').split(',');
            var sx = parseInt(parts[0]), sy = parseInt(parts[1]);
            var tx = parseInt(parts[2]), ty = parseInt(parts[3]);
            var robot = new java.awt.Robot();
            robot.setAutoDelay(25);
            robot.mouseMove(sx, sy);
            robot.delay(300);
            robot.mousePress(java.awt.event.InputEvent.BUTTON1_DOWN_MASK);
            robot.delay(200);
            var steps = 30;
            for (var i = 1; i <= steps; i++) {
                robot.mouseMove(Math.round(sx + (tx - sx) * i / steps), Math.round(sy + (ty - sy) * i / steps));
                robot.delay(25);
            }
            // Hover before releasing: the target checker only runs while the pointer moves over
            // the component, and a drop on a target that was never "entered" is discarded.
            robot.mouseMove(tx, ty - 3);
            robot.delay(150);
            robot.mouseMove(tx, ty);
            robot.delay(400);
            robot.mouseRelease(java.awt.event.InputEvent.BUTTON1_DOWN_MASK);
            robot.delay(300);
            """.trimIndent(),
        )
    }

    private fun visibleText(xpath: String): List<String> =
        find(xpath).findAllText().map { it.text }

    private fun assertText(needle: String, expected: Boolean) {
        require("'" !in needle) { "assert-text cannot contain a single quote" }
        // An xpath probe, not a scan under the IDE frame: popups (completion, JBPopup) are
        // separate windows that live outside IdeFrameImpl, and they are exactly what these
        // assertions usually target.
        val found = robot.findAll(
            ComponentFixture::class.java,
            byXpath("//div[contains(@visible_text,'$needle') or contains(@accessiblename,'$needle')]"),
        ).isNotEmpty()
        if (found != expected) {
            val what = if (expected) "expected to find" else "expected NOT to find"
            throw AssertionError("$what visible text '$needle'")
        }
        say("    ok: '$needle' ${if (expected) "present" else "absent"}")
    }

    /**
     * Dumps the Swing hierarchy of every showing window as an indented text tree.
     *
     * The full tree goes to a file (it runs to thousands of nodes in a real IDE frame — grep it);
     * only the filtered part is echoed. Each line carries exactly the attributes the XPath
     * locators match on, so a line here translates directly into a `click` argument:
     * `ActionButton name="New Chat"` becomes `//div[@accessiblename='New Chat']`.
     */
    private fun dumpTree(rest: String) {
        val parts = rest.split(' ').filter { it.isNotEmpty() }
        val maxDepth = parts.lastOrNull()?.toIntOrNull()
        val filter = (if (maxDepth == null) parts else parts.dropLast(1)).joinToString(" ")
        val tree = jsString(treeScript(filter, maxDepth ?: 40))
        val file = File(outputDir, "%02d-tree.txt".format(step))
        file.writeText(tree)
        say("    tree -> ${file.name} (${tree.lines().size} nodes)")
        say(indent(tree))
    }

    // ---------------------------------------------------------------- IDE control
    // The scripts themselves live in IdeControl so the JUnit tests drive the IDE the same way.

    private fun activateToolWindow(id: String) = IdeControl.activateToolWindow(robot, id)

    private fun invokeAction(id: String) = IdeControl.invokeAction(robot, id)

    /** `width <px> [toolWindowId]` */
    private fun setWidth(rest: String) {
        val parts = rest.split(' ').filter { it.isNotEmpty() }
        IdeControl.setToolWindowWidth(robot, if (parts.size > 1) parts[1] else "FoxxyCode", parts[0].toInt())
    }

    private fun setTheme(name: String) = when (name.lowercase()) {
        "dark" -> IdeControl.setTheme(robot, dark = true)
        "light" -> IdeControl.setTheme(robot, dark = false)
        else -> throw IllegalArgumentException("theme takes 'light' or 'dark', got '$name'")
    }

    /**
     * Locale-proof text entry: put the text on the IDE-side clipboard and paste with Ctrl+V.
     *
     * `keys` (Keyboard.enterText) presses raw key codes, which go through the OS keyboard
     * layout — on a Russian layout `hello @` arrives as `руддщ "` and nothing that watches for
     * `@` ever fires. Ctrl+V is layout-independent (same physical key everywhere). Use `keys`
     * only when a test needs true per-keystroke typing, and then only for layout-neutral keys.
     */
    private fun typeText(text: String) {
        val escaped = text.replace("\\", "\\\\").replace("'", "\\'")
        js(
            """
            var selection = new java.awt.datatransfer.StringSelection('$escaped');
            com.intellij.openapi.ide.CopyPasteManager.getInstance().setContents(selection);
            """.trimIndent()
        )
        keyboard.hotKey(KeyEvent.VK_CONTROL, KeyEvent.VK_V)
    }

    private fun pressKeys(spec: String) {
        val codes = spec.split('+').map { keyCode(it.trim()) }
        if (codes.size == 1) keyboard.key(codes[0]) else keyboard.hotKey(*codes.toIntArray())
    }

    /** Maps `ENTER`, `CTRL`, `A`, `F4` … onto `KeyEvent.VK_*` without a lookup table. */
    private fun keyCode(name: String): Int {
        // The handful of names people actually type that differ from the VK_ constant.
        val canonical = when (name.uppercase()) {
            "CTRL" -> "CONTROL"
            "ESC" -> "ESCAPE"
            "DEL" -> "DELETE"
            "BACKSPACE" -> "BACK_SPACE"
            "WIN", "META", "CMD" -> "WINDOWS"
            else -> name.uppercase()
        }
        val field = "VK_$canonical"
        try {
            return KeyEvent::class.java.getField(field).getInt(null)
        } catch (e: NoSuchFieldException) {
            throw IllegalArgumentException("unknown key '$name' (no KeyEvent.$field)")
        }
    }

    // ---------------------------------------------------------------- output

    private fun shot(command: String) = screenshot(command)

    private fun screenshot(name: String) {
        val safe = name.replace(Regex("[^A-Za-z0-9._-]"), "_").take(40)
        val file = File(outputDir, "%02d-%s.png".format(step, safe))
        ImageIO.write(cropToIde(robot.getScreenshot()), "png", file)
        say("    shot -> ${file.name}")
    }

    /**
     * Trims the full-screen grab down to the IDE's own windows.
     *
     * `getScreenshot()` captures the whole desktop — on a developer machine that is mostly the
     * browser and chat windows the sandbox happens to be sitting on top of, which is both noise
     * and someone's private screen. Cropping to the union of the IDE's showing windows keeps
     * popups and dialogs (they are separate windows) while dropping everything else.
     *
     * Falls back to the untouched image if the bounds cannot be read or do not fit — a slightly
     * noisy screenshot beats no screenshot.
     */
    private fun cropToIde(image: BufferedImage): BufferedImage {
        try {
            val parts = jsString(WINDOW_BOUNDS_SCRIPT).split(',').map { it.trim().toInt() }
            if (parts.size != 5 || parts[2] <= 0 || parts[3] <= 0) return image
            // A HiDPI desktop reports scaled AWT coordinates while the grab is in device pixels.
            val scale = image.width.toDouble() / parts[4]
            val cx = (parts[0] * scale).toInt().coerceIn(0, image.width - 1)
            val cy = (parts[1] * scale).toInt().coerceIn(0, image.height - 1)
            val cw = (parts[2] * scale).toInt().coerceAtMost(image.width - cx)
            val ch = (parts[3] * scale).toInt().coerceAtMost(image.height - cy)
            if (cw <= 0 || ch <= 0) return image
            return image.getSubimage(cx, cy, cw, ch)
        } catch (e: Exception) {
            say("    (screenshot not cropped: ${e.message})")
            return image
        }
    }

    private fun say(message: String) {
        println(message)
        log.append(message).append('\n')
    }

    private fun indent(text: String): String =
        text.lines().joinToString("\n") { "    $it" }

    private companion object {
        val DEFAULT_TIMEOUT: Duration = Duration.ofSeconds(15)

        /** Everything visible lives under the IDE frame; text and asserts scan from there. */
        const val DEFAULT_ROOT = "//div[@class='IdeFrameImpl']"

        /** Returns "x,y,width,height,screenWidth" for the union of the IDE's showing windows. */
        val WINDOW_BOUNDS_SCRIPT = """
            var windows = java.awt.Window.getWindows();
            var union = null;
            for (var i = 0; i < windows.length; i++) {
                var w = windows[i];
                if (!w.isShowing()) continue;
                var b = w.getBounds();
                if (b.width <= 0 || b.height <= 0) continue;
                union = (union == null) ? b : union.union(b);
            }
            var screen = java.awt.Toolkit.getDefaultToolkit().getScreenSize();
            if (union == null) '0,0,0,0,' + screen.width;
            else union.x + ',' + union.y + ',' + union.width + ',' + union.height + ',' + screen.width;
        """.trimIndent()

        /**
         * Rhino is ES5 — `var` only, no arrow functions, no template literals.
         *
         * `filter` keeps a node (and everything below it) when its class or accessible name
         * contains the string; empty means keep everything.
         */
        fun treeScript(filter: String, maxDepth: Int): String = """
            var sb = new java.lang.StringBuilder();
            var filter = '${filter.replace("'", "\\'")}'.toLowerCase();
            function attr(c) {
                var out = '';
                try {
                    var ac = c.getAccessibleContext();
                    if (ac != null && ac.getAccessibleName() != null) {
                        out += ' name="' + ac.getAccessibleName() + '"';
                    }
                } catch (e) {}
                try {
                    if (c instanceof javax.swing.AbstractButton ||
                        c instanceof javax.swing.JLabel ||
                        c instanceof javax.swing.text.JTextComponent) {
                        var t = c.getText();
                        if (t != null && t.length() > 0) out += ' text="' + t + '"';
                    }
                } catch (e) {}
                return out;
            }
            // Anonymous classes have an empty simple name, and IntelliJ's toolbar buttons are
            // exactly that — so without this the most clickable rows print as blanks. Fall back
            // to the nearest named superclass, marked with '~'.
            function classOf(c) {
                var k = c.getClass();
                if (k.getSimpleName().length() > 0) return k.getSimpleName();
                while (k != null && k.getSimpleName().length() == 0) k = k.getSuperclass();
                return k == null ? c.getClass().getName() : '~' + k.getSimpleName();
            }
            function walk(c, depth, matched) {
                if (depth > $maxDepth) return;
                try { if (!c.isShowing()) return; } catch (e) { return; }
                var line = classOf(c) + attr(c);
                var hit = matched || filter.length == 0 || line.toLowerCase().indexOf(filter) >= 0;
                if (hit) {
                    var pad = '';
                    for (var i = 0; i < depth; i++) pad += '  ';
                    var b = c.getBounds();
                    sb.append(pad).append(line)
                      .append(' [').append(b.width).append('x').append(b.height).append(']')
                      .append('\n');
                }
                if (c instanceof java.awt.Container) {
                    var kids = c.getComponents();
                    for (var j = 0; j < kids.length; j++) walk(kids[j], depth + 1, hit);
                }
            }
            var windows = java.awt.Window.getWindows();
            for (var k = 0; k < windows.length; k++) walk(windows[k], 0, false);
            sb.toString();
        """.trimIndent()
    }
}
