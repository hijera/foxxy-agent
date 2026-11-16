package dev.foxxycode.intellij.process

import org.junit.Assert.assertEquals
import org.junit.Test

class BackendKindTest {
    @Test
    fun `detects coddy from its http usage line`() {
        assertEquals(BackendKind.CODDY, BackendKind.detect("Usage:\n  coddy http [flags]\n"))
        assertEquals(BackendKind.CODDY, BackendKind.detect("  C:\\Tools\\coddy.exe http [flags]\n"))
        assertEquals(BackendKind.CODDY, BackendKind.detect("Usage:\n  C:\\Tools\\coddy.exe serve [flags]\n"))
    }

    @Test
    fun `foxxycode remains the default and a prose mention is not detection`() {
        assertEquals(BackendKind.FOXXYCODE, BackendKind.detect("Usage:\n  foxxycode http [flags]\n"))
        assertEquals(BackendKind.FOXXYCODE, BackendKind.detect("Compatible with coddy-agent\n"))
    }
}
