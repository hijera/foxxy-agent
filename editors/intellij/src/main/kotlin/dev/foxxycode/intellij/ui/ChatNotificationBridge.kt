package dev.foxxycode.intellij.ui

import com.google.gson.JsonObject
import com.google.gson.JsonParser
import com.intellij.notification.Notification
import com.intellij.notification.NotificationAction
import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.project.Project
import com.intellij.openapi.wm.ToolWindowManager
import com.intellij.openapi.wm.WindowManager
import java.awt.KeyboardFocusManager
import java.awt.Toolkit

/** Chat alerts carry text and opaque action IDs. The live SPA owns all decisions. */
class ChatNotificationBridge(private val project: Project, private val activate: (String, String) -> Unit) {
    private val live = LinkedHashMap<String, Notification>()

    fun receive(payload: String) {
        if (payload.length > 16384) return
        // One parse for both message shapes; malformed input drops silently.
        val message = parseMessage(payload) ?: return
        ApplicationManager.getApplication().invokeLater {
            if (project.isDisposed) return@invokeLater
            when (message) {
                is ChatBridgeDismiss -> live.remove(message.id)?.expire()
                is ChatBridgeShow -> show(message.payload)
            }
        }
    }

    private fun show(n: ChatNotificationPayload) {
        if (live.containsKey(n.id)) return
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
        // JCEF keeps its audio context suspended until an in-page gesture, so
        // the SPA cannot reliably play the alert itself: the IDE plays the
        // system sound for background alerts, like the Windows shell does.
        if (!n.silent && (n.whenFocused || !ideWindowActive())) {
            runCatching { Toolkit.getDefaultToolkit().beep() }
        }
    }

    private fun ideWindowActive(): Boolean =
        KeyboardFocusManager.getCurrentKeyboardFocusManager().activeWindow != null

    fun clear() {
        for (notification in live.values) notification.expire()
        live.clear()
    }

    private fun escapeHtml(value: String): String = value.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")
}

sealed interface ChatBridgeMessage

data class ChatBridgeDismiss(val id: String) : ChatBridgeMessage
data class ChatBridgeShow(val payload: ChatNotificationPayload) : ChatBridgeMessage

data class ChatNotificationAction(val id: String, val label: String)
data class ChatNotificationPayload(
    val id: String,
    val title: String,
    val body: String,
    val silent: Boolean,
    val whenFocused: Boolean,
    val actions: List<ChatNotificationAction>,
) {
    companion object {
        /** Never throws: anything malformed or oversized returns null. */
        fun parse(payload: String): ChatNotificationPayload? {
            val data = try {
                JsonParser.parseString(payload).takeIf { it.isJsonObject }?.asJsonObject ?: return null
            } catch (_: Exception) {
                return null
            }
            return from(data)
        }

        fun from(data: JsonObject): ChatNotificationPayload? {
            val id = data.string("id")?.takeIf { it.isNotEmpty() && it.length <= 256 } ?: return null
            // gson's getAsJsonArray throws on a non-array field; check the shape.
            val actionsElement = data.get("actions")?.takeIf { it.isJsonArray } ?: return null
            val actions = actionsElement.asJsonArray.map { element ->
                // A malformed action invalidates the whole payload: dropping it
                // silently would change the balloon's choices.
                val action = element.takeIf { it.isJsonObject }?.asJsonObject
                val actionId = action?.string("id")?.take(256)
                val label = action?.string("label")?.take(120)
                if (actionId.isNullOrEmpty() || label == null) return null
                ChatNotificationAction(actionId, label)
            }
            if (actions.size > 5 || actions.any { it.id.isEmpty() } || actions.map { it.id }.distinct().size != actions.size) return null
            return ChatNotificationPayload(
                id,
                data.string("title")?.take(256) ?: return null,
                data.string("body")?.take(500) ?: return null,
                silent = data.bool("silent") ?: false,
                whenFocused = data.bool("whenFocused") ?: false,
                actions = actions,
            )
        }
    }
}

private fun JsonObject.string(name: String): String? =
    runCatching { get(name)?.takeIf { it.isJsonPrimitive }?.asJsonPrimitive?.takeIf { it.isString }?.asString }.getOrNull()

private fun JsonObject.bool(name: String): Boolean? =
    runCatching { get(name)?.takeIf { it.isJsonPrimitive }?.asJsonPrimitive?.takeIf { it.isBoolean }?.asBoolean }.getOrNull()

/** Parses both bridge shapes once: show requests and dismiss notices. */
internal fun parseMessage(payload: String): ChatBridgeMessage? {
    val data = try {
        JsonParser.parseString(payload).takeIf { it.isJsonObject }?.asJsonObject
    } catch (_: Exception) {
        null
    } ?: return null
    if (data.has("dismiss")) {
        return data.string("dismiss")?.let { ChatBridgeDismiss(it) }
    }
    return ChatNotificationPayload.from(data)?.let { ChatBridgeShow(it) }
}
