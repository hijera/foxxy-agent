package dev.foxxycode.intellij.ui

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class ChatNotificationPayloadTest {
    @Test
    fun `chat alerts retain action ids and literal text`() {
        val n = ChatNotificationPayload.parse("""{"id":"p1","title":"FoxxyCode — chat","body":"Allow <command>?","actions":[{"id":"allow","label":"Allow"}]}""")!!
        assertEquals("Allow <command>?", n.body)
        assertEquals("allow", n.actions.single().id)
    }

    @Test
    fun `duplicate action ids are rejected`() {
        assertNull(ChatNotificationPayload.parse("""{"id":"p1","title":"Chat","body":"Allow?","actions":[{"id":"allow","label":"Allow"},{"id":"allow","label":"Again"}]}"""))
    }
}
