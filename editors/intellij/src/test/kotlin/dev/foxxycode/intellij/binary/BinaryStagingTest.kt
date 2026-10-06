package dev.foxxycode.intellij.binary

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeFalse
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.IOException
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.attribute.FileTime
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit

/**
 * Unit tests for the staging that keeps the running binary out of the plugin directory.
 * Pure filesystem work — no IntelliJ platform, so these run in the plain JUnit `test` task.
 */
class BinaryStagingTest {

    @get:Rule
    val tmp = TemporaryFolder()

    private val binaryName = "foxxycode"
    private val platform = "windows-amd64"
    private val version = "0.2.27"

    private lateinit var source: Path
    private lateinit var root: Path

    private fun setUpDirs(content: String = "binary-bytes") {
        source = tmp.newFolder("plugin", "foxxycode-bin", platform).toPath()
        root = tmp.newFolder("system").toPath().resolve("staged")
        Files.write(source.resolve(binaryName), content.toByteArray())
    }

    private fun stage(
        pluginVersion: String = version,
        platformDir: String = platform,
        makeExecutable: Boolean = false,
        minFreeBytes: Long = 0,
    ): Path = BinaryStaging.stage(
        sourceDir = source,
        root = root,
        pluginVersion = pluginVersion,
        platformDir = platformDir,
        binaryName = binaryName,
        makeExecutable = makeExecutable,
        minFreeBytes = minFreeBytes,
    )

    private fun key(pluginVersion: String = version, platformDir: String = platform): String =
        BinaryStaging.cacheKey(source, pluginVersion, platformDir)

    private fun stagedDirs(): List<String> =
        root.toFile().listFiles().orEmpty().filter { it.isDirectory }.map { it.name }.sorted()

    // ---- cache key -----------------------------------------------------------------------

    @Test
    fun `key is stable while nothing changes`() {
        setUpDirs()
        assertEquals(key(), key())
    }

    @Test
    fun `key changes when the binary size changes`() {
        setUpDirs()
        val before = key()
        Files.write(source.resolve(binaryName), "binary-bytes-and-more".toByteArray())
        assertNotEquals(before, key())
    }

    @Test
    fun `key changes when only the timestamp moves`() {
        setUpDirs()
        val before = key()
        // A dev build keeps the same version string across rebuilds; without the timestamp the
        // cache would pin the first binary ever staged and never notice a recompile.
        Files.setLastModifiedTime(source.resolve(binaryName), FileTime.fromMillis(123_456_789L))
        assertNotEquals(before, key())
    }

    @Test
    fun `key changes with the plugin version and with the platform`() {
        setUpDirs()
        assertNotEquals(key(), key(pluginVersion = "0.2.28"))
        assertNotEquals(key(), key(platformDir = "linux-arm64"))
    }

    @Test
    fun `directory name survives a hostile version string`() {
        val name = BinaryStaging.stagedDirName(platform, "0.0.0-dev+feat/x:1", "deadbeef")
        assertFalse(name.contains('/'))
        assertFalse(name.contains('+'))
        assertFalse(name.contains(':'))
        assertTrue(name.startsWith("windows-amd64-"))
        assertTrue(name.endsWith("-deadbeef"))
    }

    // ---- staging -------------------------------------------------------------------------

    @Test
    fun `staged binary is a copy living outside the source directory`() {
        setUpDirs()
        val staged = stage()

        assertTrue(Files.isRegularFile(staged))
        assertEquals("binary-bytes", String(Files.readAllBytes(staged)))
        assertTrue(staged.startsWith(root))
        assertFalse(staged.startsWith(source))
        // The bundled original is left where it was; only the plugin's own updater touches it.
        assertTrue(Files.isRegularFile(source.resolve(binaryName)))
    }

    @Test
    fun `an unchanged source is not copied a second time`() {
        setUpDirs()
        val first = stage()
        // Same length, different bytes: the fast path would keep this, a fresh copy would not.
        Files.write(first, "BINARY-BYTES".toByteArray())

        val second = stage()

        assertEquals(first, second)
        assertEquals("BINARY-BYTES", String(Files.readAllBytes(second)))
    }

    @Test
    fun `a changed source is staged next to the old build, not over it`() {
        setUpDirs()
        val first = stage()
        Files.write(source.resolve(binaryName), "binary-bytes-v2".toByteArray())

        val second = stage()

        assertNotEquals(first, second)
        // The previous build stays readable: another IDE window may still be running it.
        assertTrue(Files.isRegularFile(first))
        assertEquals("binary-bytes", String(Files.readAllBytes(first)))
        assertEquals("binary-bytes-v2", String(Files.readAllBytes(second)))
        assertEquals(2, stagedDirs().size)
    }

    @Test
    fun `a truncated staged binary is replaced, not reused`() {
        setUpDirs()
        val first = stage()
        Files.write(first, "trunc".toByteArray())

        val second = stage()

        assertEquals(first, second)
        assertEquals("binary-bytes", String(Files.readAllBytes(second)))
    }

    @Test
    fun `leftover temp directories are ignored by stage and swept by prune`() {
        setUpDirs()
        val leftover = Files.createDirectories(root.resolve(BinaryStaging.TEMP_PREFIX + "crashed"))
        Files.write(leftover.resolve(binaryName), "garbage".toByteArray())

        val staged = stage()
        assertEquals("binary-bytes", String(Files.readAllBytes(staged)))

        val failed = BinaryStaging.prune(root, staged.parent.fileName.toString())
        assertTrue(failed.isEmpty())
        assertFalse(Files.exists(leftover))
        assertTrue(Files.isRegularFile(staged))
    }

    @Test
    fun `the staged copy is executable even when the bundled one is not`() {
        assumeFalse(System.getProperty("os.name").startsWith("Windows"))
        setUpDirs()
        source.resolve(binaryName).toFile().setExecutable(false, false)

        val staged = stage(makeExecutable = true)

        assertTrue(Files.isExecutable(staged))
    }

    @Test(expected = IOException::class)
    fun `staging fails when the root cannot be created`() {
        setUpDirs()
        val file = tmp.newFile("not-a-directory").toPath()
        root = file.resolve("staged")
        stage()
    }

    @Test
    fun `staging refuses to start when free space is short`() {
        setUpDirs()
        var failed = false
        try {
            stage(minFreeBytes = Long.MAX_VALUE)
        } catch (e: IOException) {
            failed = true
        }
        assertTrue("expected staging to refuse", failed)
        // It gave up before writing anything, so there is nothing to clean up later.
        assertTrue(root.toFile().listFiles().orEmpty().none { it.name.startsWith(BinaryStaging.TEMP_PREFIX) })
    }

    // ---- pruning -------------------------------------------------------------------------

    @Test
    fun `prune keeps the current build and the most recent other one`() {
        setUpDirs()
        root = tmp.newFolder("prune-root").toPath()
        val current = makeStagedDir("current", 3_000)
        val recent = makeStagedDir("recent", 2_000)
        val older = makeStagedDir("older", 1_000)
        val oldest = makeStagedDir("oldest", 500)

        val failed = BinaryStaging.prune(root, "current")

        assertTrue(failed.isEmpty())
        assertTrue(Files.exists(current))
        assertTrue(Files.exists(recent))
        assertFalse(Files.exists(older))
        assertFalse(Files.exists(oldest))
    }

    @Test
    fun `prune never deletes the current build, however old it looks`() {
        setUpDirs()
        root = tmp.newFolder("prune-root").toPath()
        val current = makeStagedDir("current", 1)
        makeStagedDir("a", 3_000)
        makeStagedDir("b", 2_000)

        BinaryStaging.prune(root, "current")

        assertTrue(Files.exists(current))
    }

    @Test
    fun `prune on a missing root is a no-op`() {
        setUpDirs()
        assertTrue(BinaryStaging.prune(tmp.root.toPath().resolve("never-created"), "current").isEmpty())
    }

    // ---- concurrency ---------------------------------------------------------------------

    @Test
    fun `two threads staging the same build agree on one copy`() {
        setUpDirs()
        val start = CountDownLatch(1)
        val done = CountDownLatch(2)
        val results = arrayOfNulls<Path>(2)
        val failures = arrayOfNulls<Throwable>(2)

        for (i in 0..1) {
            Thread {
                try {
                    start.await()
                    results[i] = stage()
                } catch (e: Throwable) {
                    failures[i] = e
                } finally {
                    done.countDown()
                }
            }.start()
        }
        start.countDown()
        assertTrue(done.await(30, TimeUnit.SECONDS))

        assertNull("thread 0 failed: ${failures[0]}", failures[0])
        assertNull("thread 1 failed: ${failures[1]}", failures[1])
        assertEquals(results[0], results[1])
        assertEquals("binary-bytes", String(Files.readAllBytes(results[0]!!)))
        assertTrue(root.toFile().listFiles().orEmpty().none { it.name.startsWith(BinaryStaging.TEMP_PREFIX) })
    }

    private fun makeStagedDir(name: String, mtime: Long): Path {
        val dir = Files.createDirectories(root.resolve(name))
        Files.write(dir.resolve(binaryName), "x".toByteArray())
        Files.setLastModifiedTime(dir, FileTime.fromMillis(mtime))
        return dir
    }
}
