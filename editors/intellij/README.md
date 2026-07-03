# Foxxy for IntelliJ-based IDEs

Run the full **foxxy-agent** inside any JetBrains IDE (IntelliJ IDEA, PhpStorm, WebStorm,
PyCharm, GoLand, OpenIDE, …).

The plugin launches `foxxy http` scoped to the current project and embeds the **complete foxxy
web UI** in a tool window via JCEF: chat, sessions/history, scheduler, settings (API keys), skills
browser, plan mode, model & reasoning selection, and memory. All of that already exists in the
foxxy SPA, so nothing is re-implemented natively.

The `foxxy` binary is **built from source and bundled with the plugin** — no download step. The Go
source is the **repo root** (this plugin lives at `editors/intellij/`); there is no nested copy.

## How it works

```
IDE  ──tool window──▶  JBCefBrowser  ──http──▶  foxxy http  (one process per open project)
                                                  --cwd  = project root
                                                  --home = ~/.coddy (shared config & history)
```

- One `foxxy http` process per open project, on a free localhost port, with `--cwd` set to the
  project root so the agent works on the open project.
- The bundled binary is a **full-feature build** (`http ui scheduler memory`), produced by the
  `foxxyGoBuild_*` Gradle tasks from the repo root.

## Requirements

- An IntelliJ-platform IDE, **build 223 (2022.3) or newer** (no upper bound), running on a JetBrains
  Runtime with JCEF (the default). Without JCEF the plugin falls back to "Open in browser".
- The plugin compiles against the 2022.3 SDK with **Java 17** bytecode (Gradle IntelliJ Plugin 1.x).
- **Build prerequisites (host machine):** Go (per the root `go.mod`), Node.js with npm, and a
  **JDK 17** for Gradle. These are required for `buildPlugin` / `runIde` because the foxxy binary
  and its embedded SPA are built locally during plugin assembly.

## Build & run

### From the repo root (Linux/CI)

```sh
make intellij-build       # -> editors/intellij/build/distributions/foxxy-intellij-<version>.zip
make intellij-run         # dev sandbox IDE (host-platform binary only)
```

`make intellij-build` runs the **production** build: it cross-compiles the foxxy binary for every
desktop target and bundles them all into a single, install-anywhere plugin zip.

### From this directory (Windows dev)

This machine may only have JDK 8 on `PATH`; a JetBrains Runtime 17 ships with installed IDEs. Use it
as `JAVA_HOME` for Gradle. PowerShell:

```powershell
$env:JAVA_HOME = "C:\Program Files\JetBrains\PyCharm Community Edition 2023.3.2\jbr"
.\gradlew.bat runIde                          # sandbox IDE with the plugin (host binary only)
.\gradlew.bat buildPlugin -Pproduction=true   # full cross-platform plugin zip
```

The build runs these foxxy-related Gradle tasks:

- `foxxyNpmInstall` — `npm install` in `external/ui`.
- `foxxyUiBuild` — `npm run build:go` (vite build + Chromium 104 compat check + sync to `go:embed`).
- `foxxyGoBuild_<os>_<arch>` — `GOOS/GOARCH go build -tags "http ui scheduler memory"` per target.

Binaries are placed under `<plugin>/foxxy-bin/<os>-<arch>/foxxy[.exe]` inside the plugin distribution
and resolved at runtime by `FoxxyBinaryResolver` for the running IDE's platform. Without
`-Pproduction`, only the host target is built (fast local loop).

Install the built zip via **Settings | Plugins | ⚙ | Install Plugin from Disk…**.

> **Gradle distribution.** The first build downloads Gradle 8.10.2 and the IntelliJ IDEA 2022.3 SDK,
> then caches both. Easiest alternative: open `editors/intellij/` in IntelliJ IDEA as a Gradle
> project — the IDE provides Gradle and a JDK.

## Using the plugin

1. Open the **Foxxy** tool window (right side). The plugin starts the bundled `foxxy http` and
   shows the embedded UI.
2. In the UI's **Settings**, set a provider API key (or define `OPENAI_API_KEY` in the environment
   before launching the IDE), then chat, schedule jobs, browse skills, use plan mode, etc.

Reconfigure anytime in **Settings | Tools | Foxxy**: optional binary path override (leave empty to
use the bundled foxxy), host, port (0 = auto), Foxxy home, extra `foxxy http` args, and **"Match
Foxxy UI theme to the IDE theme"**. Toolbar buttons: Restart, Reload, Open in Browser,
**Open DevTools**, Settings.

### Language

The plugin UI (settings panel, first-run dialog, toolbar, notifications, native diff prompts) is
available in **English** and **Russian**. In **Settings | Tools | Foxxy**, use the **Language**
dropdown:

- **System** (default) — follows the JVM locale (`Locale.getDefault()`), which matches the IDE
  language on most JetBrains installations. On a Russian-locale system the plugin starts in Russian
  on first launch without any configuration.
- **English** / **Русский** — force a specific language regardless of the OS locale.

Missing translation keys fall back to English automatically. The embedded Foxxy web UI inside JCEF
has its own language settings and is not affected by this selector.

### Troubleshooting

If the UI goes blank on an action, open **DevTools** (toolbar) to see the JS/network error;
uncaught errors are also shown as a red overlay at the bottom of the panel. The plugin polyfills
`crypto.randomUUID` for older embedded Chromium (JCEF < Chromium 92) — without it the foxxy SPA
crashed to a blank page when creating a chat draft.

### Theme

By default the embedded UI follows the IDE theme: a light IDE LAF maps to foxxy's `light` theme,
a dark LAF to `dark`. It is applied on the initial load via the `?theme=` URL parameter (no flash)
and updated live through `window.foxxyUi.setTheme(...)` whenever you switch the IDE theme
(see `FoxxyThemeBridge` and `docs/intellij-embedding.md`). Uncheck *Match Foxxy UI theme to the IDE
theme* to instead use whatever theme you pick inside foxxy's own **Appearance** settings.

## Layout

```
src/main/kotlin/dev/foxxy/intellij/
  settings/FoxxySettings.kt        persisted settings (application level)
  settings/FoxxyConfigurable.kt    Settings | Tools | Foxxy
  binary/Platform.kt               OS/arch → bundled foxxy binary path
  binary/FoxxyBinaryResolver.kt    resolve (bundled or override) + validate (rejects lean builds)
  process/PortUtil.kt              free-port picker
  process/FoxxyProcessManager.kt   per-project process lifecycle + readiness
  ui/FoxxyToolWindowFactory.kt     tool window
  ui/FoxxyBrowserPanel.kt          JCEF browser + toolbar (+ fallbacks)
  ui/FoxxyThemeBridge.kt           IDE LAF → foxxyUi theme bridging
  ui/FirstRunDialog.kt             first-run wizard
  FoxxyBundle.kt                   localized strings (en + ru)
  FoxxyNotifications.kt            notification group
```
