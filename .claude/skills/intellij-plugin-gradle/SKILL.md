---
name: intellij-plugin-gradle
description: Build, test, and visually verify the IntelliJ plugin in editors/intellij with Gradle. Use whenever a task touches editors/intellij (Kotlin sources, plugin.xml, message bundles, build.gradle.kts), or when asked to run gradlew, runIde, the plugin's tests, or to check how the plugin looks.
---

# Building and testing the IntelliJ plugin

The plugin lives in `editors/intellij` and has its own Gradle build. It is **not** covered by
`make test` — that runs Go only. Three environment facts decide whether Gradle works at all on
this machine; get them wrong and the failure messages point somewhere unhelpful.

## The standard invocation

```bash
cd editors/intellij && JAVA_HOME="/c/Program Files/JetBrains/PyCharm Community Edition 2023.3.2/jbr" ./gradlew -g H:/gradle-home --no-daemon --offline test
```

Every piece of that is load-bearing:

| Piece | Why |
|---|---|
| `JAVA_HOME=…/jbr` | The default `java` on PATH is **JDK 8**; the build needs 17. No JDK 17 is installed standalone, but every JetBrains IDE ships one at `<IDE>/jbr`. Any of them works. |
| `-g H:/gradle-home` | The real Gradle home is `C:\Users\Антон\.gradle` — **non-ASCII**. Gradle writes the test worker's argfile as UTF-8 while the JVM reads it as Cp1251, so every cached jar path turns to mojibake and the worker dies with `Could not find or load main class worker.org.gradle.process.internal.worker.GradleWorkerMain`. Compilation survives this; **tests do not**. An ASCII Gradle home is the fix (documented in `build.gradle.kts`). |
| `--no-daemon` | Keeps a stray daemon from holding the 2.8 GB platform cache and the sandbox open between runs. |
| `--offline` | Everything is cached after the first run; skip it only when dependencies changed (see below). |

### When Gradle needs the network

Drop `--offline` **and add the proxy properties**:

```bash
cd editors/intellij && JAVA_HOME="/c/Program Files/JetBrains/PyCharm Community Edition 2023.3.2/jbr" ./gradlew -g H:/gradle-home --no-daemon -Dhttps.proxyHost=127.0.0.1 -Dhttps.proxyPort=10808 -Dhttp.proxyHost=127.0.0.1 -Dhttp.proxyPort=10808 test
```

This machine reaches the internet through a local proxy exposed as `HTTP_PROXY`/`HTTPS_PROXY`.
**Gradle's JVM ignores those environment variables** — it only reads `-Dhttp(s).proxy*`. Without
them the download fails with a misleading TLS error:

> Could not GET '…/ideaIC-2022.2.1.pom' … The server may not support the client's requested TLS
> protocol versions … Remote host terminated the handshake

That is not a TLS problem. It is the proxy not being used. (`curl` works because curl *does* read
the env vars — so "curl can reach it, Gradle can't" is the signature of this.)

The first online run downloads ~2.8 GB of IntelliJ Community 2022.2.1. Expect ~4 minutes.

## Task cheatsheet

| Task | What it does |
|---|---|
| `compileKotlin` | Fastest correctness check (~15 s incremental). Use it while iterating. |
| `test` | Platform-free JUnit4 tests: proxy-env reflection, project-relative path math, port probing. The task strips the platform's JVM args to run at all — see the long comment in `build.gradle.kts`. |
| `buildPlugin` | The distributable zip. **Also needs Go and Node**: it cross-compiles the bundled `foxxycode` binary and builds the SPA. Slow. |
| `runIde` | Launches a sandbox IDE with the plugin installed. Host binary only. |
| `runIdeForUiTests` / `uiConsole` / `uiTest` | Remote Robot UI automation — see the **intellij-plugin-uitest** skill. |

From the repo root, `make intellij-build` produces the release zip.

Add `-Pproduction=true` to cross-compile every binary target, and `-PpluginVersion=X.Y.Z` to stamp
a version.

## Go tests also guard this plugin

`go test ./editors/intellij/` (part of `make test`, no build tags) parses `plugin.xml` and
`build.gradle.kts` without compiling anything. It fails if:

- a class named in `plugin.xml` has no matching `.kt` file (every new service, action, or
  configurable must be a real file with a matching `package` and `class`);
- `FoxxyCodeBundle.properties` and `FoxxyCodeBundle_ru.properties` do not define the **same keys**
  — add every new string to **both**;
- these `build.gradle.kts` literals change: `sinceBuild.set("222")`, `untilBuild.set("")`,
  `JavaVersion.VERSION_17`, `jvmTarget = "17"`, `version.set("2022.2.1")`, `foxxycodeVerifyBinaries`;
- the binary targets drift from the VS Code extension's list.

The same package also holds `changelog_test.go`, which guards `editors/intellij/CHANGELOG.md`
(the IDE's Change Notes tab): sections in strictly descending SemVer order, written in Russian,
no stubs. See `.claude/rules/release-changelog.md`.

Run these after touching `plugin.xml`, the bundles, the Gradle config, or the changelog — they
are fast and catch what the Kotlin compiler cannot.

## Platform constraints when writing code here

- Compiled against **IntelliJ 2022.2.1**, `jvmTarget 17`, **`apiVersion 1.6`**, and the Kotlin
  stdlib is *not* bundled. A newer platform API compiles nowhere and fails at runtime on the
  floor version, so prefer APIs that have existed for years.
- Only `com.intellij.modules.platform` is depended on — the markdown plugin, the terminal plugin,
  and anything IDEA-specific are **not** available in every host (PhpStorm, GoLand, …).
- `java.net.http.HttpClient` is fine (JDK 11+, and every 2022.2+ IDE runs on JBR 17).
  `HttpURLConnection` cannot do `PATCH`.

## Visual verification

UI automation lives in the **intellij-plugin-uitest** skill: it launches a robot-equipped
sandbox (`runIdeForUiTests`), drives the plugin from scripts (`uiConsole`), captures
screenshots you can read, and runs the permanent `uiTest` suite. Use it for anything
checklist-shaped below — `uitest-scripts/checklist-*.uiscript` cover items 1 and 2.

For a quick manual look instead, launch the plain sandbox:

```bash
cd editors/intellij && JAVA_HOME="/c/Program Files/JetBrains/PyCharm Community Edition 2023.3.2/jbr" ./gradlew -g H:/gradle-home --no-daemon runIde
```

Open **View → Tool Windows → FoxxyCode** and walk this list. It is the source of truth for
*what* to check (the uitest scripts reference these item numbers), ordered so that the things
that break most often come first:

1. **Narrow width.** Drag the tool window to ~320 px. Nothing may scroll horizontally: long
   assistant messages and tool output must wrap. This is the failure mode of the HTML panes.
2. **Light and dark.** Switch the IDE theme with the panel open. Text, code blocks, and the
   context label must all stay readable — the SPA follows the IDE through `FoxxyCodeThemeBridge`,
   so a hardcoded color shows up immediately here.
3. **HiDPI.** Run at 125% and 150% scaling; check that icons and the composer are not clipped.
4. **Keyboard only.** Click into the composer, type `@`, use ↑/↓ and Enter in the completion
   popup, send with Enter. Then Escape must close the popup without sending.
5. **Drag and drop.** Drag a file from the Project view onto the panel — the very first drag of a
   session must insert an `@`-mention carrying the file's full project-relative path.
6. **A live turn.** Send something that runs a tool: the tool card should show a progress icon,
   expand on click, and a permission prompt should render buttons that unblock the turn.

Capture screenshots with the OS tool for anything you report; state the theme and width you used.

## Known flake

On Windows, a run that follows a `clean*` task in the same invocation can fail in
`classpathIndexCleanup` or a `compile*Kotlin` task with a file-lock error. It is not a real
failure — re-run the task on its own and it succeeds. Prefer separate invocations over chaining
tasks after a clean.

## What to do when a test fails

The failure reports in `build/reports/tests/test/index.html` are readable, but the fastest path is
the raw XML — it holds the assertion message directly:

```bash
grep -A6 "<failure" editors/intellij/build/test-results/test/TEST-*.xml
```
