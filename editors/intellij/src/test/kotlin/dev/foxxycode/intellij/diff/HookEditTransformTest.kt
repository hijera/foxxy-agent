package dev.foxxycode.intellij.diff

import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test

class HookEditTransformTest {
    @Test
    fun `edit matches across line endings and preserves the file style`() {
        val args = """{"oldString":"two\nthree","newString":"2\n3"}"""
        assertEquals("one\r\n2\r\n3\r\nfour\r\n", HookEditTransform.apply("edit", args, "one\r\ntwo\r\nthree\r\nfour\r\n"))
    }

    @Test
    fun `unified diff preserves crlf and the final newline`() {
        val patch = """--- a/a.txt
+++ b/a.txt
@@ -1,3 +1,3 @@
 alpha
-beta
+two
 gamma
"""
        assertEquals("alpha\r\ntwo\r\ngamma\r\n", HookEditTransform.apply("apply_patch", """{"patch":${gsonString(patch)}}""", "alpha\r\nbeta\r\ngamma\r\n"))
    }

    @Test
    fun `v4a patch applies context and additions`() {
        val patch = """*** Begin Patch
*** Update File: a.txt
@@
 alpha
-beta
+two
 gamma
*** End Patch
"""
        assertEquals("alpha\ntwo\ngamma\n", HookEditTransform.apply("apply_patch", """{"patch":${gsonString(patch)}}""", "alpha\nbeta\ngamma\n"))
    }

    @Test
    fun `bare v4a hunk preserves crlf and a trailing blank line`() {
        val patch = "@@\n-beta\n+BETA\n"
        assertEquals(
            "alpha\r\nBETA\r\n\r\n",
            HookEditTransform.apply("apply_patch", """{"patch":${gsonString(patch)}}""", "alpha\r\nbeta\r\n\r\n"),
        )
    }

    @Test
    fun `replace all uses literal text and the file line endings`() {
        val args = """{"oldString":"alpha\nbeta","newString":"alpha\nBETA","replaceAll":true}"""
        assertEquals(
            "alpha\r\nBETA\r\nalpha\r\nBETA\r\n",
            HookEditTransform.apply("edit", args, "alpha\r\nbeta\r\nalpha\r\nbeta\r\n"),
        )
    }

    @Test
    fun `a stale patch is an error rather than a guessed edit`() {
        val patch = "@@ -1 +1 @@\n-old\n+new\n"
        assertThrows(IllegalArgumentException::class.java) {
            HookEditTransform.apply("apply_patch", """{"patch":${gsonString(patch)}}""", "different\n")
        }
    }

    private fun gsonString(value: String): String = com.google.gson.JsonPrimitive(value).toString()
}
