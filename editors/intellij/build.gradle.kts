plugins {
    id("java")
    id("org.jetbrains.kotlin.jvm") version "1.9.25"
    // Legacy Gradle IntelliJ Plugin (1.x) — required to target old platforms like 2021.2 (212).
    id("org.jetbrains.intellij") version "1.17.4"
}

group = "dev.foxxycode"
// Overridable from CI: ./gradlew buildPlugin -PpluginVersion=1.2.3
version = (findProperty("pluginVersion") as String?)?.takeIf { it.isNotBlank() } ?: "0.1.1"

repositories {
    mavenCentral()
}

// ----------------------------------------------------------------------------------
// foxxycode-agent: build the bundled `foxxycode` binary from source on every plugin build.
// Mirrors the root `Makefile`: `npm --prefix external/ui run build:go` (SPA for
// go:embed, tag `ui`) then `go build -tags "http ui scheduler memory"`.
//
// The Go source is the repo root: this plugin lives at editors/intellij, so the root
// is two levels up. There is no nested clone.
//
// Prerequisites for `buildPlugin`/`runIde`: Go (see root go.mod) and Node/npm.
//
// Binary layout inside the plugin: foxxycode-bin/<os>-<arch>/foxxycode[.exe], resolved at
// runtime by FoxxyCodeBinaryResolver. Dev builds (buildPlugin/runIde) compile only the
// host target for speed; production builds (-Pproduction) cross-compile every target
// so a single plugin zip runs on all desktop platforms.
// ----------------------------------------------------------------------------------
val foxxycodeDir = layout.projectDirectory.dir("../..")
val production = project.hasProperty("production")

// Release targets, mirroring .github/workflows/release-binaries.yaml.
data class BinTarget(val goos: String, val goarch: String) {
    val binName: String get() = if (goos == "windows") "foxxycode.exe" else "foxxycode"
    val dirName: String get() = "$goos-$goarch"
}

val binTargets = listOf(
    BinTarget("linux", "amd64"),
    BinTarget("linux", "arm64"),
    BinTarget("darwin", "amd64"),
    BinTarget("darwin", "arm64"),
    BinTarget("windows", "amd64"),
)

// Map the host JVM's os/arch onto a Go target so dev builds pick the right one.
val hostGoos: String = when {
    System.getProperty("os.name").startsWith("Windows") -> "windows"
    System.getProperty("os.name").startsWith("Mac") -> "darwin"
    else -> "linux"
}
val hostGoarch: String = when (val a = System.getProperty("os.arch")) {
    "amd64", "x86_64" -> "amd64"
    "aarch64", "arm64" -> "arm64"
    else -> a
}
val hostTarget = binTargets.firstOrNull { it.goos == hostGoos && it.goarch == hostGoarch }
    ?: BinTarget(hostGoos, hostGoarch)

val foxxycodeBinRoot = layout.buildDirectory.dir("foxxycode-bin")

// On Windows `npm` is a batch file (npm.cmd); Gradle Exec needs the .cmd extension to launch it.
val npmCmd = if (System.getProperty("os.name").startsWith("Windows")) "npm.cmd" else "npm"

val foxxycodeNpmInstall by tasks.registering(Exec::class) {
    group = "foxxycode"
    description = "Install npm dependencies for the foxxycode-agent embedded UI."
    workingDir(foxxycodeDir.dir("external/ui"))
    commandLine(npmCmd, "install", "--no-fund", "--no-audit")
    // Re-run when the lockfile changes; otherwise Gradle up-to-date cache applies.
    inputs.file(foxxycodeDir.file("external/ui/package.json"))
    inputs.file(foxxycodeDir.file("external/ui/package-lock.json"))
    outputs.upToDateWhen { true }
}

val foxxycodeUiBuild by tasks.registering(Exec::class) {
    group = "foxxycode"
    description = "Build the foxxycode-agent SPA (vite + chromium-104 compat + sync to go:embed)."
    dependsOn(foxxycodeNpmInstall)
    workingDir(foxxycodeDir)
    commandLine(npmCmd, "--prefix", "external/ui", "run", "build:go")
    // The build writes into external/ui/dist and syncs into the Go tree for go:embed.
    inputs.dir(foxxycodeDir.dir("external/ui/src"))
    inputs.file(foxxycodeDir.file("external/ui/vite.config.ts"))
    inputs.file(foxxycodeDir.file("external/ui/package.json"))
    outputs.upToDateWhen { true }
}

// One cross-compile task per target. Each writes build/foxxycode-bin/<os>-<arch>/foxxycode[.exe].
// Always include the host target so dev builds work even on a host arch that is not a
// release target (e.g. a windows/arm64 dev box).
val buildTargets = (binTargets + hostTarget).distinct()
val foxxycodeBuildTasks = buildTargets.associateWith { t ->
    tasks.register<Exec>("foxxycodeGoBuild_${t.goos}_${t.goarch}") {
        group = "foxxycode"
        description = "Build the foxxycode binary for ${t.dirName} (http/ui/scheduler/memory)."
        dependsOn(foxxycodeUiBuild)
        workingDir(foxxycodeDir)
        val outFile = foxxycodeBinRoot.get().dir(t.dirName).file(t.binName)
        environment("GOOS", t.goos)
        environment("GOARCH", t.goarch)
        environment("CGO_ENABLED", "0")
        commandLine(
            "go", "build",
            "-tags", "http ui scheduler memory",
            "-trimpath",
            "-ldflags", "-s -w -X github.com/hijera/foxxycode-agent/internal/version.Version=${project.version}",
            "-o", outFile.asFile.absolutePath,
            "./cmd/foxxycode/"
        )
        inputs.dir(foxxycodeDir.dir("cmd"))
        inputs.dir(foxxycodeDir.dir("internal"))
        inputs.dir(foxxycodeDir.dir("external"))
        inputs.file(foxxycodeDir.file("go.mod"))
        outputs.file(outFile)
    }
}

// Fail fast in production if any target binary is missing before it gets packaged.
val foxxycodeVerifyBinaries by tasks.registering {
    group = "foxxycode"
    description = "Verify every release-target foxxycode binary is present (production only)."
    dependsOn(binTargets.map { foxxycodeBuildTasks.getValue(it) })
    doLast {
        val missing = binTargets.filter {
            !foxxycodeBinRoot.get().dir(it.dirName).file(it.binName).asFile.isFile
        }
        require(missing.isEmpty()) {
            "Missing foxxycode binaries for: ${missing.joinToString { it.dirName }}"
        }
    }
}

// Compile against IntelliJ IDEA Community 2022.3 so the plugin runs on build 223 and newer.
intellij {
    version.set("2022.3")
    type.set("IC")
    downloadSources.set(false)
    // Pure-Kotlin plugin, no @NotNull/form instrumentation needed.
    instrumentCode.set(false)
}

java {
    sourceCompatibility = JavaVersion.VERSION_17
    targetCompatibility = JavaVersion.VERSION_17
}

tasks {
    patchPluginXml {
        sinceBuild.set("223")
        // Empty upper bound: keep the plugin compatible with newer IDE builds.
        untilBuild.set("")
    }

    withType<org.jetbrains.kotlin.gradle.tasks.KotlinCompile> {
        kotlinOptions {
            // 2022.3 runs on JBR 17 and bundles Kotlin 1.7 — stay within both.
            jvmTarget = "17"
            apiVersion = "1.7"
        }
    }

    // Skips launching the (old) target IDE headlessly just to index settings.
    buildSearchableOptions {
        enabled = false
    }

    // Bundle the locally-built foxxycode binaries into the plugin distribution under foxxycode-bin/.
    // production: all targets (single cross-platform zip). dev: host target only (fast loop).
    prepareSandbox {
        if (production) {
            dependsOn(foxxycodeVerifyBinaries)
        } else {
            dependsOn(foxxycodeBuildTasks.getValue(hostTarget))
        }
        from(foxxycodeBinRoot) {
            into("${intellij.pluginName.get()}/foxxycode-bin")
        }
    }
}
