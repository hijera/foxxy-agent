plugins {
    id("java")
    id("org.jetbrains.kotlin.jvm") version "1.9.25"
    // Legacy Gradle IntelliJ Plugin (1.x) — required to target old platforms like 2021.2 (212).
    id("org.jetbrains.intellij") version "1.17.4"
}

group = "dev.foxxycode"
// Overridable from CI: ./gradlew buildPlugin -PpluginVersion=1.2.3
version = (findProperty("pluginVersion") as String?)?.takeIf { it.isNotBlank() } ?: "0.1.3"

repositories {
    mavenCentral()
    // Remote Robot (remote-robot, remote-fixtures, robot-server-plugin) is not on Maven Central.
    maven("https://packages.jetbrains.team/maven/p/ij/intellij-dependencies")
}

// Remote Robot: the UI test client and the matching robot-server plugin must be the same version.
val remoteRobotVersion = "0.11.23"

// Gson (remote-robot's response decoder) reflects over java.lang internals; JDK 17 needs to be
// told that is allowed. See the comment on the UI testing task block.
val remoteRobotJvmArgs = listOf("--add-opens", "java.base/java.lang=ALL-UNNAMED")

// Where the sandbox IDE's throwaway project is materialised.
val uiTestProjectDir = layout.buildDirectory.dir("uitest-project")

// The port robot-server listens on inside the sandbox IDE. `uiConsole`, `uiTest` and the plain
// `curl http://127.0.0.1:8580/` component-tree viewer all talk to it.
val robotServerPort = "8580"

// JCEF's Chrome-DevTools-Protocol port inside the sandbox. Remote Robot stops at the Swing
// boundary; everything the chat SPA renders is only reachable through this (CefChat.kt, the
// `cef-*` uiConsole commands). `curl http://127.0.0.1:8581/json` lists the live page targets.
val cefDebugPort = "8581"

dependencies {
    // Plain JUnit4 unit tests (e.g. ProxyEnvironmentTest) — no IntelliJ platform needed.
    testImplementation("junit:junit:4.13.2")
}

// Remote Robot UI tests and the `uiConsole` script runner. These drive a *separate* IDE process
// (the `runIdeForUiTests` sandbox) over HTTP, so this source set never sees the IntelliJ platform
// on its own classpath — only the remote-robot client.
val uiTest: SourceSet = sourceSets.create("uiTest") {
    compileClasspath += sourceSets["main"].output
    runtimeClasspath += sourceSets["main"].output
}

configurations["uiTestImplementation"].extendsFrom(configurations["testImplementation"])
configurations["uiTestRuntimeOnly"].extendsFrom(configurations["testRuntimeOnly"])

dependencies {
    "uiTestImplementation"("com.intellij.remoterobot:remote-robot:$remoteRobotVersion")
    "uiTestImplementation"("com.intellij.remoterobot:remote-fixtures:$remoteRobotVersion")
    // RemoteRobot's constructor takes an okhttp3.OkHttpClient, but remote-robot declares its
    // transport (retrofit, which brings okhttp) as runtime-only — so without this the compiler
    // reports "Cannot access class 'okhttp3.OkHttpClient'". Pulling retrofit at the version
    // remote-robot asks for keeps the compile and runtime okhttp identical.
    "uiTestImplementation"("com.squareup.retrofit2:retrofit:2.11.0")
}

/**
 * Undoes the IDE-run configuration that the IntelliJ Gradle plugin applies to every [Test] task.
 *
 * `test` needs this (see the long comment on that task below) and so does `uiTest`: both run in a
 * plain JVM with no platform on the classpath, so `-Djava.system.class.loader=…PathClassLoader`
 * and the coroutines javaagent kill the worker before a single test runs.
 */
fun Test.stripIntellijPlatformJvmArguments() {
    doFirst {
        systemProperties.remove("java.system.class.loader")
        // The agent comes from the plugin's IntelliJPlatformArgumentProvider, which is reachable
        // neither through jvmArgs nor systemProperties (and assigning allJvmArgs fails: its setter
        // touches the already-finalized debug options). Drop that whole provider — its other
        // arguments only configure an IDE sandbox these tests never open.
        jvmArgumentProviders.removeIf { provider ->
            provider.asArguments().any {
                it.startsWith("-javaagent:") && it.contains("coroutines-javaagent")
            }
        }
    }
}

// ----------------------------------------------------------------------------------
// foxxycode-agent: build the bundled `foxxycode` binary from source on every plugin build.
// Mirrors the root `Makefile`: `npm --prefix external/ui run build:go` (SPA for
// go:embed, tag `ui`) then `go build -tags "http ui scheduler memory browser"`.
// Tags track the Makefile FULL_TAGS set minus `cli` (the plugin speaks ACP, never the
// console TUI); TestBundledBinaryTagsMatchShippedTagSet keeps the two from drifting.
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
        description = "Build the foxxycode binary for ${t.dirName} (http/ui/scheduler/memory/browser)."
        dependsOn(foxxycodeUiBuild)
        workingDir(foxxycodeDir)
        val outFile = foxxycodeBinRoot.get().dir(t.dirName).file(t.binName)
        environment("GOOS", t.goos)
        environment("GOARCH", t.goarch)
        environment("CGO_ENABLED", "0")
        commandLine(
            "go", "build",
            "-tags", "http ui scheduler memory browser",
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

// Compile against IntelliJ IDEA Community 2022.2.1 so the plugin runs on build 222 and newer
// (PhpStorm / any IntelliJ-based IDE 2022.2.1+). Building against the floor makes the compiler
// reject any accidental use of a 223+ API.
intellij {
    version.set("2022.2.1")
    type.set("IC")
    downloadSources.set(false)
    // Pure-Kotlin plugin, no @NotNull/form instrumentation needed.
    instrumentCode.set(false)
}

java {
    sourceCompatibility = JavaVersion.VERSION_17
    targetCompatibility = JavaVersion.VERSION_17
}

// ----------------------------------------------------------------------------------
// Change notes: CHANGELOG.md is the single source for the "Change Notes" tab the IDE
// shows in Settings | Plugins. It is written in Russian for a human reader, not as a
// commit list, and its sections are versioned `## X.Y.Z — YYYY-MM-DD`, newest first.
//
// The markdown is rendered here rather than through the org.jetbrains.changelog plugin
// so the build needs no extra dependency. Only the subset the file actually uses is
// supported: level-2 version headings, paragraphs, `**bold**`, and `code`.
// ----------------------------------------------------------------------------------
val changelogFile = layout.projectDirectory.file("CHANGELOG.md").asFile
val changeNotesSections = 3

/** One `## X.Y.Z — date` section with its body lines. */
data class ChangelogEntry(val version: String, val heading: String, val body: List<String>)

fun parseChangelog(text: String): List<ChangelogEntry> {
    val headingRe = Regex("""^##\s+(\d+\.\d+\.\d+)\s*[—-]\s*(.+)$""")
    val entries = mutableListOf<ChangelogEntry>()
    var current: ChangelogEntry? = null
    for (line in text.lines()) {
        val m = headingRe.find(line.trim())
        if (m != null) {
            current?.let { entries += it }
            current = ChangelogEntry(m.groupValues[1], line.trim().removePrefix("##").trim(), mutableListOf())
        } else if (current != null) {
            (current.body as MutableList<String>) += line
        }
    }
    current?.let { entries += it }
    return entries
}

fun escapeHtml(s: String): String =
    s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")

/** Inline markdown -> HTML for the small subset the changelog uses. */
fun inlineHtml(s: String): String =
    Regex("""\*\*(.+?)\*\*""").replace(
        Regex("""`([^`]+)`""").replace(escapeHtml(s)) { "<code>${it.groupValues[1]}</code>" },
    ) { "<b>${it.groupValues[1]}</b>" }

fun renderChangeNotes(entries: List<ChangelogEntry>): String = buildString {
    for (entry in entries) {
        append("<h3>").append(escapeHtml(entry.heading)).append("</h3>\n")
        val paragraph = StringBuilder()
        fun flush() {
            if (paragraph.isNotBlank()) append("<p>").append(inlineHtml(paragraph.toString().trim())).append("</p>\n")
            paragraph.setLength(0)
        }
        for (line in entry.body) {
            if (line.isBlank()) flush() else paragraph.append(line.trim()).append(' ')
        }
        flush()
    }
}

/**
 * changeNotesHtml renders the newest sections, putting the version being built first when
 * the changelog knows it. A dev build (`0.0.0-dev-<sha>`) has no section of its own, so it
 * simply shows the latest released ones; a missing or unreadable file leaves the notes
 * empty rather than failing the build.
 */
val changeNotesHtml: String by lazy {
    if (!changelogFile.isFile) {
        logger.warn("CHANGELOG.md not found at ${changelogFile.path}; the plugin ships without change notes.")
        return@lazy ""
    }
    val entries = parseChangelog(changelogFile.readText(Charsets.UTF_8))
    if (entries.isEmpty()) {
        logger.warn("CHANGELOG.md has no `## X.Y.Z — date` sections; the plugin ships without change notes.")
        return@lazy ""
    }
    val built = version.toString()
    if (entries.none { it.version == built } && !built.contains("dev")) {
        logger.warn("CHANGELOG.md has no section for version $built — the IDE will show older notes instead.")
    }
    val ordered = entries.sortedByDescending { it.version == built }
    renderChangeNotes(ordered.take(changeNotesSections))
}

tasks {
    patchPluginXml {
        sinceBuild.set("222")
        // Empty upper bound: keep the plugin compatible with newer IDE builds.
        untilBuild.set("")
        changeNotes.set(provider { changeNotesHtml })
    }

    withType<org.jetbrains.kotlin.gradle.tasks.KotlinCompile> {
        kotlinOptions {
            // 2022.2 runs on JBR 17 and bundles Kotlin 1.6 — stay within both so the
            // plugin doesn't reference stdlib APIs absent from the 2022.2 runtime.
            jvmTarget = "17"
            apiVersion = "1.6"
        }
    }

    // Skips launching the (old) target IDE headlessly just to index settings.
    buildSearchableOptions {
        enabled = false
    }

    // Bundle the locally-built foxxycode binaries into the plugin distribution under foxxycode-bin/.
    // production: all targets (single cross-platform zip). dev: host target only (fast loop).
    //
    // Applied to the UI-testing sandbox as well: without the binary the plugin renders its
    // "foxxycode binary not found" error card instead of the SPA, so every UI check would look
    // at a dead panel. Deliberately NOT applied to prepareTestingSandbox: `test` must keep
    // working without Go and Node.
    val bundleFoxxycodeBinaries: org.jetbrains.intellij.tasks.PrepareSandboxTask.() -> Unit = {
        if (production) {
            dependsOn(foxxycodeVerifyBinaries)
        } else {
            dependsOn(foxxycodeBuildTasks.getValue(hostTarget))
        }
        from(foxxycodeBinRoot) {
            into("${intellij.pluginName.get()}/foxxycode-bin")
        }
    }
    prepareSandbox(bundleFoxxycodeBinaries)
    named<org.jetbrains.intellij.tasks.PrepareSandboxTask>("prepareUiTestingSandbox", bundleFoxxycodeBinaries)

    // The unit tests here are plain JUnit4 ones that never touch the IntelliJ platform
    // (ProxyEnvironmentTest drives a fake configurable through reflection;
    // ProjectRelativePathsTest is pure path math). The IntelliJ Gradle plugin nevertheless
    // configures every Test task for an IDE run, which breaks the test JVM twice over:
    //
    //  1. -Djava.system.class.loader=com.intellij.util.lang.PathClassLoader — that class ships
    //     in the IDE distribution, not on a unit test's runtime classpath, so the VM dies with
    //     "Error occurred during initialization of VM ... ClassNotFoundException:
    //      com.intellij.util.lang.PathClassLoader".
    //  2. the kotlinx-coroutines debug javaagent — a manifest-only jar whose Premain-Class
    //     resolves through that same platform classpath, otherwise
    //     "Exception in thread \"main\" java.lang.ClassNotFoundException:
    //      kotlinx.coroutines.debug.AgentPremain".
    //
    // Both are stripped by `stripIntellijPlatformJvmArguments()` (in doFirst, so it also clears
    // whatever the plugin added during lazy configuration). If a test ever does need the IntelliJ
    // platform, give it its own task rather than putting these back here.
    //
    // KNOWN ENVIRONMENT LIMIT: this task still cannot fork a worker on a machine whose
    // GRADLE_USER_HOME contains non-ASCII characters (e.g. C:\Users\<cyrillic>\.gradle).
    // Gradle writes the worker classpath argfile as UTF-8 while the JVM parses argfiles using
    // sun.jnu.encoding (Cp1251 there), so every cached jar path turns to mojibake and the VM
    // reports "Could not find or load main class
    // worker.org.gradle.process.internal.worker.GradleWorkerMain". That is upstream Gradle
    // behaviour, unrelated to the two workarounds above; the fix is an ASCII Gradle home.
    test {
        useJUnit()
        stripIntellijPlatformJvmArguments()
    }

    // ------------------------------------------------------------------------------------
    // Remote Robot UI testing. See .claude/skills/intellij-plugin-uitest/SKILL.md.
    //
    // Unlike `test`, these drive a real IDE: `runIdeForUiTests` launches the sandbox with the
    // plugin plus the robot-server plugin listening on 8580, and `uiConsole` / `uiTest` connect
    // to it from a separate JVM. The sandbox must stay running while they do.
    //
    // Both need --add-opens: remote-robot decodes responses with Gson, which reflects over
    // java.lang.Throwable's private fields to deserialize IDE-side errors. On JDK 17 that throws
    // InaccessibleObjectException, and retrofit surfaces it as the misleading
    // "Unable to create converter for class …RetrieveResponse".
    // ------------------------------------------------------------------------------------

    downloadRobotServerPlugin {
        version.set(remoteRobotVersion)
    }

    // The IDE opens a *copy* of uitest-project/ so the .idea/ directory it writes there — and any
    // damage a test does — never lands in the repository.
    val syncUiTestProject by registering(Copy::class) {
        from(layout.projectDirectory.dir("uitest-project"))
        into(uiTestProjectDir)
    }

    runIdeForUiTests {
        // Without a project argument the IDE stops at the Welcome frame, where a project tool
        // window like FoxxyCode cannot exist at all.
        dependsOn(syncUiTestProject)
        args = listOf(uiTestProjectDir.get().asFile.absolutePath)

        systemProperty("robot-server.port", robotServerPort)
        // Registry values can be overridden by a same-named system property; this makes JBCefApp
        // start Chromium with --remote-debugging-port so tests can reach inside the chat SPA.
        systemProperty("ide.browser.jcef.debug.port", cefDebugPort)
        // A fresh sandbox otherwise blocks on modal startup dialogs, and a modal dialog stops
        // robot-server from reaching anything behind it.
        systemProperty("jb.privacy.policy.text", "<!--999.999-->")
        systemProperty("jb.consents.confirmation.enabled", "false")
        systemProperty("ide.show.tips.on.startup.default.value", "false")
        // Without this the modal "Trust and Open Project?" dialog blocks startup and the project
        // never opens (waitForProject then times out at 2 minutes).
        systemProperty("idea.trust.all.projects", "true")
        // Pin the locale. FoxxyCodeBundle resolves against the IDE's locale, so on a Russian
        // Windows the toolbar tooltips come up in Russian and every locator written against the
        // English bundle misses. Tests and CI both assume English.
        systemProperty("user.language", "en")
        systemProperty("user.country", "US")

        // FoxxyCodeToolWindowFactory opens FirstRunDialog whenever firstRunCompleted is false —
        // a modal that nothing behind it can be reached through, and a fresh sandbox always hits
        // it. Seed the setting so the tool window opens straight into the panel.
        //
        // foxxycodeHome points the backend at a throwaway config dir. Without it the sandboxed
        // backend reads the developer's real ~/.foxxycode/config.yaml — so tests would inherit
        // their `ui.locale` (breaking every English locator on a `locale: ru` machine), see their
        // real sessions, and write into their real history. `user.language=en` above only wins
        // while the backend config leaves the locale on auto, which a fresh home guarantees.
        //
        // Only when the file is absent: if it exists the plugin has already persisted settings
        // here, so whatever a previous run or a test decided stays. (If the dialog still shows
        // up, delete the sandbox config dir — see the skill's troubleshooting section.)
        doFirst {
            val settings = File(configDir.get(), "options/foxxycode.xml")
            val home = layout.buildDirectory.dir("uitest-foxxycode-home").get().asFile
            // A backend home with no config.yaml puts the SPA on its "Choose a provider"
            // onboarding screen, which is a modal *inside* the browser: it covers the composer,
            // and nothing on the Swing side can dismiss it. Seed a throwaway provider so the
            // panel opens straight into the chat. The key is deliberately fake - no scripted
            // check here talks to a model.
            val backendConfig = File(home, "config.yaml")
            if (!backendConfig.exists()) {
                home.mkdirs()
                backendConfig.writeText(
                    """
                    providers:
                      - name: uitest
                        type: openai
                        api_key: uitest-not-a-real-key
                    models:
                      - model: uitest/gpt-4o
                    agent:
                      model: uitest/gpt-4o
                    """.trimIndent() + "\n"
                )
                logger.lifecycle("Seeded ${backendConfig.absolutePath} so the panel skips onboarding.")
            }
            if (!settings.exists()) {
                home.mkdirs()
                settings.parentFile.mkdirs()
                settings.writeText(
                    """
                    <application>
                      <component name="FoxxyCodeSettings">
                        <option name="firstRunCompleted" value="true" />
                        <option name="foxxycodeHome" value="${home.absolutePath.replace('\\', '/')}" />
                      </component>
                    </application>
                    """.trimIndent()
                )
                logger.lifecycle("Seeded ${settings.absolutePath} to skip the first-run dialog.")
            }
        }
    }

    register<JavaExec>("uiConsole") {
        group = "verification"
        description = "Runs a UI script against the sandbox started by runIdeForUiTests " +
            "(-PuiScript=<file>). Writes screenshots and a log to build/uiconsole/."
        classpath = uiTest.runtimeClasspath
        mainClass.set("dev.foxxycode.intellij.uitest.UiConsoleKt")
        jvmArgs(remoteRobotJvmArgs)
        systemProperty("foxxycode.cef.debug.url", "http://127.0.0.1:$cefDebugPort")
        // `uiScript`, not `script`: findProperty("script") resolves against the Project bean
        // before extra properties and silently hands back `false`.
        // Resolved at execution time so configuring any other task does not require the property.
        argumentProviders.add(CommandLineArgumentProvider {
            val script = project.findProperty("uiScript")?.toString()
                ?: throw GradleException(
                    "uiConsole needs a script: -PuiScript=uitest-scripts/<name>.uiscript"
                )
            listOf(
                script,
                layout.buildDirectory.dir("uiconsole").get().asFile.absolutePath,
                "http://127.0.0.1:$robotServerPort",
            )
        })
    }

    register<Test>("uiTest") {
        group = "verification"
        description = "Remote Robot UI tests (src/uiTest). Needs a sandbox already running " +
            "via runIdeForUiTests, and a real unlocked display."
        testClassesDirs = uiTest.output.classesDirs
        classpath = uiTest.runtimeClasspath
        useJUnit()
        systemProperty("robot-server.url", "http://127.0.0.1:$robotServerPort")
        systemProperty("foxxycode.cef.debug.url", "http://127.0.0.1:$cefDebugPort")
        jvmArgs(remoteRobotJvmArgs)
        stripIntellijPlatformJvmArguments()
        // Nothing here is up-to-date-able: the IDE it talks to is outside Gradle's model.
        outputs.upToDateWhen { false }
    }
}
