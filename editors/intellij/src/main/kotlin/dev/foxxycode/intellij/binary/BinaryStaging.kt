package dev.foxxycode.intellij.binary

import java.io.IOException
import java.nio.file.AtomicMoveNotSupportedException
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.StandardCopyOption
import java.nio.file.attribute.FileTime
import java.security.MessageDigest
import java.util.Comparator
import java.util.UUID
import java.util.stream.Collectors
import java.util.stream.Stream

/**
 * Copies the bundled foxxycode binary out of the plugin directory before it is executed.
 *
 * Windows keeps a mandatory lock on the image file of a running process, so a backend started
 * from `<plugin>/foxxycode-bin/...` makes the plugin directory undeletable — and a plugin update
 * fails with `AccessDeniedException` on exactly that path, leaving the user on the old version.
 * Running a copy from outside the plugin keeps the plugin directory free of running images.
 *
 * A hard link would not do: on Windows the loader locks the file *object*, so another name for
 * the same bytes stays just as undeletable. It has to be a real copy.
 *
 * Deliberately JDK-only — no IntelliJ platform types — so the whole policy is unit-testable
 * without an IDE fixture. [FoxxyCodeBinaryResolver] supplies the platform-specific roots.
 */
object BinaryStaging {

    /** Prefix of the half-built directories [stage] moves into place; also what [prune] sweeps. */
    const val TEMP_PREFIX = ".staging-"

    /** How many staged builds survive a [prune], the current one included. */
    const val KEEP = 2

    /** Free space demanded on top of the payload before a copy is attempted. */
    const val FREE_SPACE_HEADROOM = 64L * 1024 * 1024

    /**
     * Identity of one bundled build: the plugin version plus the exact bytes on disk, derived
     * from the (relative name, size, last-modified) triple of every file under [sourceDir].
     *
     * Deliberately not a content hash. Digesting ~30 MB costs a full cold read on every IDE
     * start — the one moment this code must not add latency — while size+mtime is a stat per
     * file. A false miss costs one extra copy; a false hit would need the same plugin version,
     * the same size to the byte and the same mtime to the millisecond, and `go build` rewrites
     * the file whenever it changes. The mtime is what keeps dev builds honest: a sandbox plugin
     * reports the same version string after every rebuild.
     */
    fun cacheKey(sourceDir: Path, pluginVersion: String, platformDir: String): String {
        val digest = MessageDigest.getInstance("SHA-256")
        digest.update(pluginVersion.toByteArray(Charsets.UTF_8))
        digest.update(platformDir.toByteArray(Charsets.UTF_8))
        for (file in listFiles(sourceDir)) {
            val stamp = relative(sourceDir, file) +
                ":" + Files.size(file) +
                ":" + Files.getLastModifiedTime(file).toMillis()
            digest.update(stamp.toByteArray(Charsets.UTF_8))
        }
        val bytes = digest.digest()
        return (0 until KEY_LENGTH_BYTES).joinToString("") { String.format("%02x", bytes[it]) }
    }

    /** Directory name a key maps to, e.g. `windows-amd64-0.2.27-3f9a1c04`. Filesystem-safe. */
    fun stagedDirName(platformDir: String, pluginVersion: String, key: String): String =
        sanitize(platformDir) + "-" + sanitize(pluginVersion) + "-" + key

    /**
     * Returns the staged copy of [binaryName], copying [sourceDir] under [root] first when this
     * build is not staged yet.
     *
     * The copy lands in a `.staging-<uuid>` sibling and is moved into place as a whole directory,
     * so "the final name exists" and "the copy is complete" are the same fact, and a directory
     * another process is currently executing from is never written into.
     *
     * @param makeExecutable set the executable bit on the copied files (pointless on Windows)
     * @throws IOException when the source is missing, the root cannot be created, disk space is
     *   short, or the copy fails — the caller is expected to fall back to the in-plugin binary.
     */
    @Throws(IOException::class)
    fun stage(
        sourceDir: Path,
        root: Path,
        pluginVersion: String,
        platformDir: String,
        binaryName: String,
        makeExecutable: Boolean,
        minFreeBytes: Long = FREE_SPACE_HEADROOM,
    ): Path {
        val source = sourceDir.resolve(binaryName)
        if (!Files.isRegularFile(source)) throw IOException("No bundled binary at $source")
        val sourceSize = Files.size(source)

        Files.createDirectories(root)
        val target = root.resolve(
            stagedDirName(platformDir, pluginVersion, cacheKey(sourceDir, pluginVersion, platformDir))
        )
        reuse(target, binaryName, sourceSize)?.let { return it }
        // Whether the name was taken *before* we started copying. Re-checking it later would be
        // a race: a sibling process that stages the same build while we copy would have its
        // finished directory deleted out from under the path it already handed to its caller.
        val stale = Files.exists(target)

        val files = listFiles(sourceDir)
        val payload = files.sumOf { Files.size(it) }
        val usable = Files.getFileStore(root).usableSpace
        // Subtract rather than add: `payload + minFreeBytes` can overflow, and an overflowed
        // comparison would wave through exactly the case this guard exists for.
        if (usable - payload < minFreeBytes) {
            throw IOException("Not enough free space in $root: $usable available, $payload needed plus $minFreeBytes headroom")
        }

        // A staged directory that was already there and holds no usable binary is a crashed or
        // truncated copy; drop it so the fresh one can take the name.
        if (stale) deleteRecursively(target)

        val tmp = Files.createDirectory(root.resolve(TEMP_PREFIX + UUID.randomUUID()))
        try {
            for (file in files) {
                val destination = tmp.resolve(relative(sourceDir, file))
                destination.parent?.let { Files.createDirectories(it) }
                Files.copy(file, destination, StandardCopyOption.REPLACE_EXISTING)
                if (makeExecutable) destination.toFile().setExecutable(true, false)
            }
            try {
                Files.move(tmp, target, StandardCopyOption.ATOMIC_MOVE)
            } catch (e: AtomicMoveNotSupportedException) {
                Files.move(tmp, target)
            } catch (e: IOException) {
                // Another IDE window - or another project in this one - staged the same build
                // while we were copying. Renaming onto an existing directory fails differently
                // on every OS (FileAlreadyExistsException on Windows, DirectoryNotEmptyException
                // on Unix), so the target's own state decides whether this was a harmless race
                // or a real failure.
                deleteRecursively(tmp)
                return reuse(target, binaryName, sourceSize) ?: throw e
            }
        } catch (e: Throwable) {
            deleteRecursively(tmp)
            throw e
        }
        return target.resolve(binaryName)
    }

    /**
     * Deletes `.staging-*` leftovers and all but the [keep] most recently used staged builds,
     * never [keepDirName].
     *
     * Returns the entries it could not delete. Failure is expected rather than exceptional: on
     * Windows a sibling IDE window still running the previous build holds its image locked. The
     * next launch tries again.
     */
    fun prune(root: Path, keepDirName: String, keep: Int = KEEP): List<Path> {
        if (!Files.isDirectory(root)) return emptyList()
        val entries = listDirectories(root)
        val leftovers = entries.filter { it.fileName.toString().startsWith(TEMP_PREFIX) }
        val doomed = entries
            .filter { !it.fileName.toString().startsWith(TEMP_PREFIX) }
            .filter { it.fileName.toString() != keepDirName }
            .sortedByDescending { lastModified(it) }
            .drop((keep - 1).coerceAtLeast(0))

        val failed = mutableListOf<Path>()
        for (path in leftovers + doomed) {
            if (!deleteRecursively(path)) failed.add(path)
        }
        return failed
    }

    /**
     * The staged binary when it is already there and the right size, marking its directory as
     * most recently used so [prune] treats it as live. Null means "copy it".
     */
    private fun reuse(target: Path, binaryName: String, expectedSize: Long): Path? {
        val binary = target.resolve(binaryName)
        if (!Files.isRegularFile(binary)) return null
        if (Files.size(binary) != expectedSize) return null
        try {
            Files.setLastModifiedTime(target, FileTime.fromMillis(System.currentTimeMillis()))
        } catch (e: IOException) {
            // A read-only staging root only costs pruning accuracy, never a launch.
        }
        return binary
    }

    /** Regular files under [dir], sorted by relative path so the digest is order-independent. */
    private fun listFiles(dir: Path): List<Path> {
        if (!Files.isDirectory(dir)) return emptyList()
        return walk(dir) { stream ->
            stream.filter { Files.isRegularFile(it) }
                .collect(Collectors.toList())
                .sortedBy { relative(dir, it) }
        }
    }

    private fun listDirectories(root: Path): List<Path> {
        val stream = Files.list(root)
        try {
            return stream.filter { Files.isDirectory(it) }.collect(Collectors.toList())
        } finally {
            stream.close()
        }
    }

    private fun lastModified(path: Path): Long =
        try {
            Files.getLastModifiedTime(path).toMillis()
        } catch (e: IOException) {
            0L
        }

    /** Best-effort recursive delete; false when anything survived (a locked image, typically). */
    private fun deleteRecursively(path: Path): Boolean {
        if (!Files.exists(path)) return true
        return try {
            walk(path) { stream ->
                stream.sorted(Comparator.reverseOrder()).forEach { Files.deleteIfExists(it) }
            }
            !Files.exists(path)
        } catch (e: Exception) {
            false
        }
    }

    private fun <T> walk(dir: Path, body: (Stream<Path>) -> T): T {
        val stream = Files.walk(dir)
        try {
            return body(stream)
        } finally {
            stream.close()
        }
    }

    /** Relative path with forward slashes, so a digest computed on Windows matches elsewhere. */
    private fun relative(root: Path, file: Path): String =
        root.relativize(file).toString().replace('\\', '/')

    /**
     * Keeps a directory name to characters every filesystem accepts. Plugin versions may carry
     * `+build` metadata or a branch name with a slash.
     */
    private fun sanitize(value: String): String {
        val cleaned = value.trim().map { c ->
            if (c.isLetterOrDigit() || c == '.' || c == '-' || c == '_') c else '_'
        }.joinToString("")
        return cleaned.ifEmpty { "unknown" }.take(MAX_NAME_PART)
    }

    /** Four bytes of the digest: enough to separate builds, short enough to keep paths sane. */
    private const val KEY_LENGTH_BYTES = 4

    /** Upper bound on each name component, so a wild version string cannot blow the path limit. */
    private const val MAX_NAME_PART = 48
}
