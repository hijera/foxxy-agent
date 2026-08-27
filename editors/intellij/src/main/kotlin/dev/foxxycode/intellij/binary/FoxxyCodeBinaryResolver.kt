package dev.foxxycode.intellij.binary

import com.intellij.ide.plugins.IdeaPluginDescriptor
import com.intellij.ide.plugins.PluginManagerCore
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.application.PathManager
import com.intellij.openapi.diagnostic.logger
import com.intellij.openapi.extensions.PluginId
import com.intellij.openapi.util.SystemInfo
import com.intellij.util.concurrency.annotations.RequiresBackgroundThread
import dev.foxxycode.intellij.FoxxyCodeBundle
import dev.foxxycode.intellij.settings.FoxxyCodeSettings
import java.io.File
import java.io.IOException
import java.nio.file.Path
import java.nio.file.Paths

/**
 * Resolves and validates the foxxycode binary to use.
 *
 * Resolution order:
 *  1. An explicit override path set in Settings | Tools | FoxxyCode (optional).
 *  2. The binary bundled with the plugin under `<plugin>/foxxycode-bin/<goos>-<goarch>/foxxycode[.exe]`,
 *     cross-compiled from the repo root at plugin build time, staged out of the plugin directory
 *     before it is launched — see [resolveForLaunch].
 */
object FoxxyCodeBinaryResolver {
    private val LOG = logger<FoxxyCodeBinaryResolver>()
    private const val PLUGIN_ID = "dev.foxxycode.intellij"
    private const val BUNDLED_DIR = "foxxycode-bin"

    /**
     * The binary the backend is actually started from.
     *
     * An explicit `binaryPath` override is returned verbatim — it exists so a developer can point
     * the plugin at their own `go build` output, and copying that would serve stale bytes after
     * every rebuild. The bundled binary, in contrast, is staged into [BinaryStaging]'s cache
     * outside the plugin directory, because a running image makes the plugin directory
     * undeletable on Windows and the next plugin update fails with `AccessDeniedException`.
     *
     * Blocking: the first call after a plugin update copies the binary (tens of megabytes).
     * Falls back to the in-plugin binary when staging is impossible — a full disk should cost
     * the update guarantee, not the plugin.
     */
    @RequiresBackgroundThread
    fun resolveForLaunch(): File? {
        val s = FoxxyCodeSettings.getInstance().state
        if (s.binaryPath.isNotBlank()) {
            val f = File(s.binaryPath)
            if (f.isFile) return f
            LOG.warn("Configured binary path does not point to a file: ${s.binaryPath}")
        }

        val bundled = bundledBinary()?.takeIf { it.isFile } ?: return null
        if (ApplicationManager.getApplication()?.isDispatchThread == true) {
            LOG.warn("resolveForLaunch() called on the EDT; staging can take seconds", Throwable())
        }
        return stage(bundled) ?: bundled
    }

    /** Copies the bundled binary out of the plugin directory, or null when that failed. */
    private fun stage(bundled: File): File? {
        // Two projects opening at once must not each copy the same tens of megabytes. Across
        // processes no lock is needed: the staged directory is moved into place atomically.
        synchronized(this) {
            val sourceDir = bundled.toPath().parent ?: return null
            val root = stagingRoot()
            return stageUnderLock(sourceDir, root)
        }
    }

    private fun stageUnderLock(sourceDir: Path, root: Path): File? {
        return try {
            val staged = BinaryStaging.stage(
                sourceDir = sourceDir,
                root = root,
                pluginVersion = pluginVersion(),
                platformDir = Platform.platformDirName(),
                binaryName = Platform.binaryFileName(),
                makeExecutable = !SystemInfo.isWindows,
            )
            staged.parent?.fileName?.let { current ->
                val kept = BinaryStaging.prune(root, current.toString())
                // A staged build another IDE window still runs cannot be deleted on Windows.
                // Expected, not an error: the next launch tries again.
                if (kept.isNotEmpty()) LOG.info("Staged builds still in use, left in place: $kept")
            }
            staged.toFile()
        } catch (e: IOException) {
            LOG.warn(
                "Could not stage the foxxycode binary under $root; running it from the plugin " +
                    "directory instead. Updating the plugin may fail on Windows while it runs.",
                e
            )
            null
        }
    }

    /** `<plugin>/foxxycode-bin/<goos>-<goarch>/foxxycode[.exe]`, or null when the plugin is gone. */
    private fun bundledBinary(): File? {
        val descriptor = descriptor() ?: return null
        return descriptor.pluginPath.resolve(BUNDLED_DIR).resolve(Platform.bundledRelativePath()).toFile()
    }

    private fun descriptor(): IdeaPluginDescriptor? = PluginManagerCore.getPlugin(PluginId.getId(PLUGIN_ID))

    private fun pluginVersion(): String = descriptor()?.version?.takeIf { it.isNotBlank() } ?: "unknown"

    /**
     * Where staged copies live: per product and per major IDE version, so two IDEs never contend
     * for the same directory. One staged build is tens of megabytes and [BinaryStaging.prune]
     * keeps at most [BinaryStaging.KEEP] of them.
     */
    private fun stagingRoot(): Path = Paths.get(PathManager.getSystemPath(), BUNDLED_DIR)

    data class Validation(val ok: Boolean, val version: String?, val message: String)

    /**
     * Confirms the binary runs and is a full-feature build that supports `foxxycode http`.
     * A lean build prints "http support is not built in" (see cmd/foxxycode/http_stub.go).
     * Blocking; call off the EDT.
     */
    @RequiresBackgroundThread
    fun validate(binary: File): Validation {
        if (!binary.isFile) return Validation(false, null, FoxxyCodeBundle.message("binary.error.notFound", binary.path))

        val version = runCapture(binary, listOf("-v"))?.trim()
            ?: return Validation(false, null, FoxxyCodeBundle.message("binary.error.executeVersion", binary.path))

        val help = runCapture(binary, listOf("http", "--help"))
            ?: return Validation(false, version, FoxxyCodeBundle.message("binary.error.executeHelp", binary.path))

        if (help.contains("not built", ignoreCase = true) ||
            help.contains("http support is not", ignoreCase = true)
        ) {
            return Validation(
                false, version,
                FoxxyCodeBundle.message("binary.error.leanBuild")
            )
        }
        return Validation(true, version, FoxxyCodeBundle.message("binary.ok.fullBuild", version))
    }

    /** Runs the binary, merging stderr into stdout; returns combined output or null on failure. */
    private fun runCapture(binary: File, args: List<String>): String? {
        return try {
            val proc = ProcessBuilder(listOf(binary.absolutePath) + args)
                .redirectErrorStream(true)
                .start()
            val out = proc.inputStream.bufferedReader().readText()
            proc.waitFor()
            out
        } catch (e: Exception) {
            null
        }
    }
}
