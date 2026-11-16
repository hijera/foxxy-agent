package dev.foxxycode.intellij.uitest

import com.google.gson.Gson
import com.intellij.remoterobot.RemoteRobot
import com.intellij.remoterobot.utils.waitFor
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File
import java.time.Duration

class ArtifactMenuUiTest {
    @Test
    fun fileActionsRemainReachableOutsideTheCard() {
        val robot = RemoteRobot(System.getProperty("robot-server.url", "http://127.0.0.1:8580"))
        IdeControl.dismissPopups(robot)
        IdeControl.activateToolWindow(robot, "FoxxyCode")
        val home = File(System.getProperty("foxxycode.uitest.home", "build/uitest-foxxycode-home"))
        val id = "sess_artifactmenu" + System.currentTimeMillis()
        val dir = File(home, "sessions/$id")
        assertTrue(dir.mkdirs())
        val gson = Gson()
        File(dir, "session.json").writeText(gson.toJson(mapOf(
            "version" to 1, "id" to id, "cwd" to File(".").absolutePath,
            "mode" to "agent", "title" to "File actions fixture",
            "updatedAt" to "2026-11-05T10:00:00Z",
        )))
        val artifact = mapOf("id" to "a".repeat(24), "name" to "qa-report.txt",
            "sha256" to "b".repeat(64), "size" to 8, "source_relative_path" to "qa-report.txt")
        File(dir, "messages.json").writeText(gson.toJson(mapOf("version" to 1, "messages" to listOf(
            mapOf("role" to "user", "content" to "Share the QA report"),
            mapOf("role" to "assistant", "content" to "Sharing the report", "tool_calls" to listOf(
                mapOf("id" to "call_share", "name" to "share_file", "input" to "{\"path\":\"qa-report.txt\"}"))),
            mapOf("role" to "tool", "tool_call_id" to "call_share", "content" to "Report ready", "artifacts" to listOf(artifact)),
            mapOf("role" to "assistant", "content" to "QA report is ready."),
        ))))
        CefChat.connectWithRetry().use { chat ->
            chat.eval("location.hash='#/s/$id'; 'opening fixture'")
            waitFor(Duration.ofSeconds(20), Duration.ofMillis(100), "file card") {
                chat.eval("!!document.querySelector('button[aria-label=\"Actions for qa-report.txt\"]')") == "true"
            }
            for (width in listOf(360, 640)) {
                IdeControl.setToolWindowWidth(robot, "FoxxyCode", width)
                chat.eval("new Promise(function(resolve){requestAnimationFrame(function(){requestAnimationFrame(resolve);});})")
                chat.eval("document.querySelector('button[aria-label=\"Actions for qa-report.txt\"]').scrollIntoView({block:'center'})")
                chat.eval("new Promise(function(resolve){requestAnimationFrame(function(){requestAnimationFrame(resolve);});})")
                chat.click("button[aria-label='Actions for qa-report.txt']")
                waitFor(Duration.ofSeconds(5), Duration.ofMillis(100), "file actions remain reachable at $width px") {
                    chat.eval("""
                    (function(){var m=document.querySelector('.inline-artifact-menu');
                      if(!m||m.closest('.tool-artifact-card'))return false;
                      var b=m.querySelector('button').getBoundingClientRect();
                      var hit=document.elementFromPoint(b.x+b.width/2,b.y+b.height/2);
                      var r=m.getBoundingClientRect();
                      return !!hit&&m.contains(hit)&&r.top>=0&&r.left>=0&&r.right<=innerWidth&&r.bottom<=innerHeight;})()
                    """.trimIndent()) == "true"
                }
                IdeControl.setToolWindowWidth(robot, "FoxxyCode", width + 20)
                waitFor(Duration.ofSeconds(5), Duration.ofMillis(100), "resize closes file actions") {
                    chat.eval("!document.querySelector('.inline-artifact-menu')") == "true"
                }
                assertTrue("resize raised the FoxxyCode error overlay",
                    chat.eval("!document.getElementById('foxxycode-err-overlay')") == "true")
                chat.eval("new Promise(function(resolve){requestAnimationFrame(function(){requestAnimationFrame(resolve);});})")
                chat.click("button[aria-label='Actions for qa-report.txt']")
                chat.pressKey("ESC")
                assertTrue("Escape did not close file actions", chat.eval("!document.querySelector('.inline-artifact-menu')") == "true")
            }
        }
    }
}
