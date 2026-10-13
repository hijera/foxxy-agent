package dev.foxxycode.intellij.ui

import com.google.gson.JsonParser
import com.intellij.notification.Notification
import com.intellij.notification.NotificationAction
import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.project.Project
import com.intellij.openapi.wm.ToolWindowManager
import com.intellij.openapi.wm.WindowManager

/** Chat alerts carry text and opaque action IDs. The live SPA owns all decisions. */
class ChatNotificationBridge(private val project: Project, private val activate: (String, String) -> Unit) {
    private val live = LinkedHashMap<String, Notification>()

    fun receive(payload: String) {
        if (payload.length > 16384) return
        val data = try { JsonParser.parseString(payload).asJsonObject } catch (_: Exception) { return }
        ApplicationManager.getApplication().invokeLater {
            if (project.isDisposed) return@invokeLater
            if (data.has("dismiss")) {
                val id = runCatching { data.get("dismiss").asString }.getOrNull() ?: return@invokeLater
                live.remove(id)?.expire()
                return@invokeLater
            }
            val n = runCatching { ChatNotificationPayload.parse(payload) }.getOrNull() ?: return@invokeLater
            if (live.containsKey(n.id)) return@invokeLater
            val notification = NotificationGroupManager.getInstance().getNotificationGroup("FoxxyCode")
                .createNotification(escapeHtml(n.title), escapeHtml(n.body).replace("\n", "<br>"), NotificationType.INFORMATION)
            for (action in n.actions) {
                notification.addAction(NotificationAction.createSimple(action.label) {
                    if (live.remove(n.id) !== notification) return@createSimple
                    notification.expire()
                    ToolWindowManager.getInstance(project).getToolWindow("FoxxyCode")?.activate(null)
                    WindowManager.getInstance().getFrame(project)?.toFront()
                    activate(n.id, action.id)
                })
            }
            live[n.id] = notification
            if (live.size > 128) live.remove(live.keys.first())?.expire()
            notification.notify(project)
        }
    }

    fun clear() {
        for (notification in live.values) notification.expire()
        live.clear()
    }

    private fun escapeHtml(value: String): String = value.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")
}

data class ChatNotificationAction(val id: String, val label: String)
data class ChatNotificationPayload(val id: String, val title: String, val body: String, val actions: List<ChatNotificationAction>) {
    companion object {
        fun parse(payload: String): ChatNotificationPayload? {
            val data = JsonParser.parseString(payload).asJsonObject
            val id = data.get("id").asString
            if (id.isEmpty() || id.length > 256) return null
            val actions = data.getAsJsonArray("actions").map {
                val a = it.asJsonObject
                ChatNotificationAction(a.get("id").asString, a.get("label").asString.take(120))
            }
            if (actions.size > 5 || actions.any { it.id.isEmpty() } || actions.map { it.id }.distinct().size != actions.size) return null
            return ChatNotificationPayload(id, data.get("title").asString.take(256), data.get("body").asString.take(500), actions)
        }
    }
}
