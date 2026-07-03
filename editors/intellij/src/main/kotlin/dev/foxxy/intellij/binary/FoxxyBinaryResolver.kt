package dev.foxxy.intellij.binary

import com.intellij.ide.plugins.PluginManagerCore
import com.intellij.openapi.diagnostic.logger
import com.intellij.openapi.extensions.PluginId
import dev.foxxy.intellij.FoxxyBundle
import dev.foxxy.intellij.settings.FoxxySettings
import java.io.File

/**
 * Resolves and validates the foxxy binary to use.
 *
 * Resolution order:
 *  1. An explicit override path set in Settings | Tools | Foxxy (optional).
 *  2. The binary bundled with the plugin under `<plugin>/foxxy-bin/<goos>-<goarch>/foxxy[.exe]`,
 *     cross-compiled from the repo root at plugin build time.
 */
object FoxxyBinaryResolver {
    private val LOG = logger<FoxxyBinaryResolver>()
    private const val PLUGIN_ID = "dev.foxxy.intellij"
    private const val BUNDLED_DIR = "foxxy-bin"

    /** Returns the resolved binary without triggering any download, or null if unavailable. */
    fun resolveExisting(): File? {
        val s = FoxxySettings.getInstance().state

        // 1. Explicit override from settings.
        if (s.binaryPath.isNotBlank()) {
            val f = File(s.binaryPath)
            if (f.isFile) return f
            LOG.warn("Configured binary path does not point to a file: ${s.binaryPath}")
        }

        // 2. Bundled binary shipped with the plugin, matching the running IDE's os/arch.
        val descriptor = PluginManagerCore.getPlugin(PluginId.getId(PLUGIN_ID)) ?: return null
        val bin = descriptor.pluginPath.resolve(BUNDLED_DIR).resolve(Platform.bundledRelativePath()).toFile()
        return if (bin.isFile) bin else null
    }

    data class Validation(val ok: Boolean, val version: String?, val message: String)

    /**
     * Confirms the binary runs and is a full-feature build that supports `foxxy http`.
     * A lean build prints "http support is not built in" (see cmd/coddy/http_stub.go).
     * Blocking; call off the EDT.
     */
    fun validate(binary: File): Validation {
        if (!binary.isFile) return Validation(false, null, FoxxyBundle.message("binary.error.notFound", binary.path))

        val version = runCapture(binary, listOf("-v"))?.trim()
            ?: return Validation(false, null, FoxxyBundle.message("binary.error.executeVersion", binary.path))

        val help = runCapture(binary, listOf("http", "--help"))
            ?: return Validation(false, version, FoxxyBundle.message("binary.error.executeHelp", binary.path))

        if (help.contains("not built", ignoreCase = true) ||
            help.contains("http support is not", ignoreCase = true)
        ) {
            return Validation(
                false, version,
                FoxxyBundle.message("binary.error.leanBuild")
            )
        }
        return Validation(true, version, FoxxyBundle.message("binary.ok.fullBuild", version))
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
