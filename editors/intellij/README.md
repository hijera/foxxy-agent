# FoxxyCode for IntelliJ-based IDEs

Run the full **foxxycode-agent** inside any JetBrains IDE (IntelliJ IDEA, PhpStorm, WebStorm,
PyCharm, GoLand, OpenIDE, …).

The plugin launches `foxxycode http` scoped to the current project and embeds the **complete foxxycode
web UI** in a tool window via JCEF: chat, sessions/history, scheduler, settings (API keys), skills
browser, plan mode, model & reasoning selection, and memory. All of that already exists in the
foxxycode SPA, so nothing is re-implemented natively.

The `foxxycode` binary is **built from source and bundled with the plugin** — no download step. The Go
source is the **repo root** (this plugin lives at `editors/intellij/`); there is no nested copy.

## How it works

```
IDE  ──tool window──▶  JBCefBrowser  ──http──▶  foxxycode http  (one process per open project)
                                                  --cwd  = project root
                                                  --home = ~/.foxxycode (shared config & history)
```

- One `foxxycode http` process per open project, on a free localhost port, with `--cwd` set to the
  project root so the agent works on the open project.
- The bundled binary is a **full-feature build** (`http ui scheduler memory`), produced by the
  `foxxycodeGoBuild_*` Gradle tasks from the repo root.

## Requirements

- An IntelliJ-platform IDE, **build 223 (2022.3) or newer** (no upper bound), running on a JetBrains
  Runtime with JCEF (the default). Without JCEF the plugin falls back to "Open in browser".
- The plugin compiles against the 2022.3 SDK with **Java 17** bytecode (Gradle IntelliJ Plugin 1.x).
- **Build prerequisites (host machine):** Go (per the root `go.mod`), Node.js with npm, and a
  **JDK 17** for Gradle. These are required for `buildPlugin` / `runIde` because the foxxycode binary
  and its embedded SPA are built locally during plugin assembly.

## Build & run

### From the repo root (Linux/CI)

```sh
make intellij-build       # -> editors/intellij/build/distributions/foxxycode-intellij-<version>.zip
make intellij-run         # dev sandbox IDE (host-platform binary only)
```

`make intellij-build` runs the **production** build: it cross-compiles the foxxycode binary for every
desktop target and bundles them all into a single, install-anywhere plugin zip.

### From this directory (Windows dev)

This machine may only have JDK 8 on `PATH`; a JetBrains Runtime 17 ships with installed IDEs. Use it
as `JAVA_HOME` for Gradle. PowerShell:

```powershell
$env:JAVA_HOME = "C:\Program Files\JetBrains\PyCharm Community Edition 2023.3.2\jbr"
.\gradlew.bat runIde                          # sandbox IDE with the plugin (host binary only)
.\gradlew.bat buildPlugin -Pproduction=true   # full cross-platform plugin zip
```

The build runs these foxxycode-related Gradle tasks:

- `foxxycodeNpmInstall` — `npm install` in `external/ui`.
- `foxxycodeUiBuild` — `npm run build:go` (vite build + Chromium 104 compat check + sync to `go:embed`).
- `foxxycodeGoBuild_<os>_<arch>` — `GOOS/GOARCH go build -tags "http ui scheduler memory"` per target.

Binaries are placed under `<plugin>/foxxycode-bin/<os>-<arch>/foxxycode[.exe]` inside the plugin distribution
and resolved at runtime by `FoxxyCodeBinaryResolver` for the running IDE's platform. Without
`-Pproduction`, only the host target is built (fast local loop).

Install the built zip via **Settings | Plugins | ⚙ | Install Plugin from Disk…**.

> **Gradle distribution.** The first build downloads Gradle 8.10.2 and the IntelliJ IDEA 2022.3 SDK,
> then caches both. Easiest alternative: open `editors/intellij/` in IntelliJ IDEA as a Gradle
> project — the IDE provides Gradle and a JDK.

## Using the plugin

### Onboarding

The first time you open the **FoxxyCode** tool window, a multi-step welcome wizard explains where to click (panel location, API key, composer, inline diffs, toolbar). Re-open it anytime via **Tools → Show FoxxyCode Welcome**. If JCEF is unavailable, a text fallback is shown instead.

1. Open the **FoxxyCode** tool window (right side). The plugin starts the bundled `foxxycode http` and
   shows the embedded UI.
2. In the UI's **Settings**, set a provider API key (or define `OPENAI_API_KEY` in the environment
   before launching the IDE), then chat, schedule jobs, browse skills, use plan mode, etc.

Reconfigure anytime in **Settings | Tools | FoxxyCode**: optional binary path override (leave empty to
use the bundled foxxycode), host, port (0 = auto), FoxxyCode home, extra `foxxycode http` args, and **"Match
FoxxyCode UI theme to the IDE theme"**. Toolbar buttons: Restart, Reload, Open in Browser,
**Open DevTools**, Settings.

### Language

The plugin UI (settings panel, first-run dialog, toolbar, notifications, native diff prompts) is
available in **English** and **Russian**. There is a single language switcher for the whole
application, in the embedded FoxxyCode web UI: **Settings → General → Language** (Auto / English /
Russian). It persists to the backend config (`ui.locale`); the plugin has no language dropdown of
its own.

- **Auto** — follows the JVM locale (`Locale.getDefault()`), which matches the IDE language on most
  JetBrains installations. On a Russian-locale system the plugin starts in Russian without any
  configuration.
- **English** / **Русский** — force a specific language regardless of the OS locale.

The plugin reads `ui.locale` from the backend on start and updates live when you flip the switcher
in the web UI (relayed over the JCEF `window.foxxycodeUi.onLocaleChange` bridge). Missing translation
keys fall back to English automatically.

### Troubleshooting

If the UI goes blank on an action, open **DevTools** (toolbar) to see the JS/network error;
uncaught errors are also shown as a red overlay at the bottom of the panel. The plugin polyfills
`crypto.randomUUID` for older embedded Chromium (JCEF < Chromium 92) — without it the foxxycode SPA
crashed to a blank page when creating a chat draft.

### Theme

By default the embedded UI follows the IDE theme: a light IDE LAF maps to foxxycode's `light` theme,
a dark LAF to `dark`. It is applied on the initial load via the `?theme=` URL parameter (no flash)
and updated live through `window.foxxycodeUi.setTheme(...)` whenever you switch the IDE theme
(see `FoxxyCodeThemeBridge` and `docs/intellij-embedding.md`). Uncheck *Match FoxxyCode UI theme to the IDE
theme* to instead use whatever theme you pick inside foxxycode's own **Appearance** settings.

## Layout

```
src/main/kotlin/dev/foxxycode/intellij/
  settings/FoxxyCodeSettings.kt        persisted settings (application level)
  settings/FoxxyCodeConfigurable.kt    Settings | Tools | FoxxyCode
  binary/Platform.kt               OS/arch → bundled foxxycode binary path
  binary/FoxxyCodeBinaryResolver.kt    resolve (bundled or override) + validate (rejects lean builds)
  process/PortUtil.kt              free-port picker
  process/FoxxyCodeProcessManager.kt   per-project process lifecycle + readiness
  ui/FoxxyCodeToolWindowFactory.kt     tool window
  ui/FoxxyCodeBrowserPanel.kt          JCEF browser + toolbar (+ fallbacks)
  ui/FoxxyCodeThemeBridge.kt           IDE LAF → foxxycodeUi theme bridging
  ui/WelcomeWizardDialog.kt        multi-step JCEF onboarding wizard
  ui/FoxxyCodeWelcomeAction.kt     Tools menu: re-open onboarding
  ui/FirstRunDialog.kt             first-run wrapper → WelcomeWizardDialog
  FoxxyCodeBundle.kt                   localized strings (en + ru)
  FoxxyCodeNotifications.kt            notification group
```
