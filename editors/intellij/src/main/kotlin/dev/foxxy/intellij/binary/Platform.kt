package dev.foxxy.intellij.binary

import com.intellij.openapi.util.SystemInfo

/**
 * Resolves the bundled foxxy binary location for the current IDE runtime platform.
 *
 * The plugin bundles one binary per desktop target under
 * `<plugin>/foxxy-bin/<goos>-<goarch>/foxxy[.exe]` (built by the `foxxyGoBuild_*` Gradle
 * tasks). [bundledRelativePath] picks the entry matching the running IDE.
 */
object Platform {
    fun goos(): String = when {
        SystemInfo.isWindows -> "windows"
        SystemInfo.isMac -> "darwin"
        else -> "linux"
    }

    fun goarch(): String = when (System.getProperty("os.arch").lowercase()) {
        "amd64", "x86_64", "x64" -> "amd64"
        "aarch64", "arm64" -> "arm64"
        else -> "amd64"
    }

    fun binaryFileName(): String = if (SystemInfo.isWindows) "foxxy.exe" else "foxxy"

    /** Relative path under `foxxy-bin/` for the current platform, e.g. `linux-amd64/foxxy`. */
    fun bundledRelativePath(): String = "${goos()}-${goarch()}/${binaryFileName()}"
}
