package dev.foxxycode.intellij.ui

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ChatNotificationPayloadTest {
    @Test
    fun `chat alerts retain action ids and literal text`() {
        val n = ChatNotificationPayload.parse("""{"id":"p1","title":"FoxxyCode — chat","body":"Allow <command>?","actions":[{"id":"allow","label":"Allow"}]}""")!!
        assertEquals("Allow <command>?", n.body)
        assertEquals("allow", n.actions.single().id)
        assertFalse(n.silent)
        assertFalse(n.whenFocused)
    }

    @Test
    fun `duplicate action ids are rejected`() {
        assertNull(ChatNotificationPayload.parse("""{"id":"p1","title":"Chat","body":"Allow?","actions":[{"id":"allow","label":"Allow"},{"id":"allow","label":"Again"}]}"""))
    }

    @Test
    fun `silent and whenFocused flags survive parsing`() {
        val n = ChatNotificationPayload.parse("""{"id":"p1","title":"Chat","body":"Allow?","silent":true,"whenFocused":true,"actions":[]}""")!!
        assertTrue(n.silent)
        assertTrue(n.whenFocused)
    }

    @Test
    fun `oversized ids bodies and action lists are rejected or capped`() {
        val longId = "x".repeat(257)
        assertNull(ChatNotificationPayload.parse("""{"id":"$longId","title":"Chat","body":"Allow?","actions":[]}"""))
        val sixActions = (1..6).joinToString(",") { """{"id":"a$it","label":"A$it"}""" }
        assertNull(ChatNotificationPayload.parse("""{"id":"p1","title":"Chat","body":"Allow?","actions":[$sixActions]}"""))
        val n = ChatNotificationPayload.parse("""{"id":"p1","title":"${"t".repeat(300)}","body":"${"b".repeat(600)}","actions":[{"id":"allow","label":"${"l".repeat(200)}"}]}""")!!
        assertEquals(256, n.title.length)
        assertEquals(500, n.body.length)
        assertEquals(120, n.actions.single().label.length)
    }

    @Test
    fun `malformed payloads return null instead of throwing`() {
        assertNull(ChatNotificationPayload.parse("not json"))
        assertNull(ChatNotificationPayload.parse("""[1,2,3]"""))
        assertNull(ChatNotificationPayload.parse("""{"title":"Chat","body":"Allow?","actions":[]}"""))
        assertNull(ChatNotificationPayload.parse("""{"id":"p1","title":"Chat","body":"Allow?","actions":[{"id":"allow"}]}"""))
        assertNull(ChatNotificationPayload.parse("""{"id":"p1","title":"Chat","body":"Allow?","actions":"nope"}"""))
    }

    @Test
    fun `dismiss notices parse alongside show requests`() {
        val dismiss = parseMessage("""{"dismiss":"p1"}""") as ChatBridgeDismiss
        assertEquals("p1", dismiss.id)
        val show = parseMessage("""{"id":"p1","title":"Chat","body":"Allow?","actions":[]}""") as ChatBridgeShow
        assertEquals("p1", show.payload.id)
        assertNull(parseMessage("not json"))
        assertNull(parseMessage("""{"dismiss":42}"""))
    }
}
