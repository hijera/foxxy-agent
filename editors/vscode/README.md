# FoxxyCode for VS Code

Run the full **foxxycode-agent** inside VS Code.

The extension launches `foxxycode http` scoped to the current workspace and embeds the **complete
foxxycode web UI** in a webview: chat, sessions/history, scheduler, settings (API keys), skills
browser, plan mode, model & reasoning selection, and memory. All of that already exists in the
foxxycode SPA, so nothing is re-implemented natively.

The `foxxycode` binary is **built from source and bundled with the extension** — no download step.
The Go source is the **repo root** (this extension lives at `editors/vscode/`); there is no nested
copy. Functionally it is the full peer of the [IntelliJ plugin](../intellij), including native
inline diffs (Accept / Reject / Revert / Show diff) driven by the foxxycode IDE event stream.

## How it works

```
VS Code  ──webview (iframe)──▶  foxxycode SPA  ──http──▶  foxxycode http  (one process per workspace)
                                                              --cwd  = workspace root
                                                              --home = ~/.foxxycode (shared config & history)
VS Code  ──SSE GET /foxxycode/ide/events──▶                  foxxycode http
VS Code  ──POST /foxxycode/sessions/<id>/permission──▶       foxxycode http
```

- One `foxxycode http` process per workspace, on a free localhost port, with `--cwd` set to the
  workspace root so the agent works on the open project.
- The bundled binary is a **full-feature build** (`http ui scheduler memory`), cross-compiled from
  the repo root by `scripts/prepare-binary.mjs`.
- The extension subscribes to `GET /foxxycode/ide/events` and renders each agent file edit in the
  real editor with green/red line decorations and an Accept / Reject (or Revert) decision.

## Requirements

- VS Code **1.75 or newer**.
- **Build prerequisites (host machine):** Go (per the root `go.mod`), Node.js with npm. Required
  for `vsce package` because the foxxycode binary is built locally during extension assembly.

## Build & run

Two packaging modes are supported:

### Universal VSIX (default — one package that runs everywhere)

```sh
make vscode-package         # -> editors/vscode/foxxycode-vscode-<version>.vsix
```

`make vscode-package` runs the **production** build: it cross-compiles the foxxycode binary for
every desktop target and bundles them all into a single, install-anywhere VSIX.

### Platform-specific VSIX (one per OS/arch)

```sh
make vscode-package-target TARGET=linux-amd64 VSCE_TARGET=linux-x64
make vscode-package-target TARGET=darwin-arm64 VSCE_TARGET=darwin-arm64
# …one per target: linux-x64, linux-arm64, darwin-x64, darwin-arm64, win32-x64
```

Each platform-specific VSIX bundles only its own target's binary, so it is much smaller than the
universal VSIX. The Go → VS Code target mapping (printed by `node scripts/prepare-binary.mjs --help`):

| Go target | VS Code `--target` |
| --- | --- |
| `linux-amd64` | `linux-x64` |
| `linux-arm64` | `linux-arm64` |
| `darwin-amd64` | `darwin-x64` |
| `darwin-arm64` | `darwin-arm64` |
| `windows-amd64` | `win32-x64` |

### From this directory (dev loop)

```sh
npm install
npm run build          # universal: builds all binaries + esbuild compile
npm run build:target   # host target only (fast)
npx vsce package       # universal VSIX
npx vsce package --target linux-x64   # platform-specific
```

Install the built VSIX via **Extensions: Install from VSIX…** in the Command Palette.

### Tests

```sh
npm test                # vitest: i18n, binaryResolver, portUtil, editEvent, lineFragments, prepareBinary
npm run typecheck       # tsc --noEmit
npm run compile         # esbuild bundle to out/extension.js
```

## Using the extension

### Onboarding

On first activation the extension opens the **Get started with FoxxyCode** walkthrough on the Welcome tab (5 steps: open the panel, set an API key, send a message, review inline diffs, toolbar shortcuts). Re-open it anytime via **FoxxyCode: Show Welcome** in the Command Palette.

1. Open the **FoxxyCode** view from the activity bar (left side). The extension starts the bundled
   `foxxycode http` and shows the embedded UI. Use **FoxxyCode: Open Panel** in the Command Palette
   to open the same UI in an editor tab.
2. In the UI's **Settings**, set a provider API key (or define `OPENAI_API_KEY` in the environment
   before launching VS Code), then chat, schedule jobs, browse skills, use plan mode, etc.

Reconfigure anytime in **Settings → Extensions → FoxxyCode**: optional binary path override (leave
empty to use the bundled foxxycode), host, port (0 = auto), FoxxyCode home, extra `foxxycode http`
args, **"Match FoxxyCode UI theme to the VS Code color theme"**, native inline diffs, and auto-apply
edits. The UI language is **not** an extension setting — it is set once in the FoxxyCode UI
(**Settings → General**) and shared across the whole app. Toolbar buttons on the webview title bar:
Restart, Reload, Open in Browser, Open DevTools, Settings. The extension version is shown next to
the view name in the sidebar header (and in the editor tab title when opened via **Open Panel**).

### Attaching files by drag and drop

Drag a file from the **Explorer**, or an open **editor tab**, onto the composer to insert it as a
short `@`-mention; the full workspace-relative path is sent with the prompt. **Hold `Shift` while
dragging** — VS Code disables pointer events over webviews during a workbench drag unless `Shift`
is held, so without it the drop lands on the editor group behind the panel. This is a VS Code
platform behaviour, not a FoxxyCode setting.

### Language

The extension UI is available in **English** and **Russian**. There is a single language switcher
for the whole application: **FoxxyCode UI → Settings → General → Language** (Auto / English /
Russian), which persists to the backend config (`ui.locale`). The extension has no language setting
of its own.

- **Runtime strings** (notifications, first-run message, diff prompts, progress indicators, error
  views) and **command titles** (Command Palette and webview toolbar, dual variants gated by the
  `foxxycode.locale` context key) follow the backend `ui.locale`: the extension reads it on start
  and updates live when you flip the switcher inside the embedded SPA. When `ui.locale` is **Auto**,
  they follow `vscode.env.language` (the VS Code display language).
- **Settings descriptions** and the activity-bar view container title follow the **VS Code display
  language** via `package.nls.json` / `package.nls.ru.json`. VS Code resolves those statically from
  `vscode.env.language`; they cannot be overridden by `ui.locale`. With the display language and
  **Auto** aligned, everything stays in sync.

The resolved locale is forwarded to the embedded SPA via the `?lang=` URL parameter on load; live
changes flow back from the SPA over the `{ type: "foxxycode:locale", locale }` webview message.
Missing translation keys fall back to English automatically.

### Theme

By default the embedded UI follows the VS Code color theme: a light VS Code theme maps to
foxxycode's `light` theme, anything else to `dark`. It is applied on the initial load via the
`?theme=` URL parameter (no flash) and updated live by reloading the iframe whenever the VS Code
color theme changes. Uncheck *Match FoxxyCode UI theme to the VS Code color theme* to instead use
whatever theme you pick inside foxxycode's own **Appearance** settings.

### Native inline diffs

When the agent edits a file inside the open workspace:

- **Proposed edit** (default): the file opens with green/red line decorations and a notification
  with **Accept** / **Reject** / **Show diff** buttons. Accept/Reject posts to
  `/foxxycode/sessions/<id>/permission`; Show diff opens a VS Code diff editor comparing the
  before/after content as virtual documents.
- **Applied edit**: the file opens with decorations and a notification with **Revert** / **Show
  diff**. Revert writes the pre-edit content back via a `WorkspaceEdit`.
- **Auto-apply** (opt-in): proposed edits are accepted automatically; the diff is still shown with
  a Revert affordance.

Disable inline diffs in Settings (*Show native inline diffs in the editor when the agent edits
files*) to let the SPA handle edits on its own.

### Troubleshooting

The extension writes stdout/stderr from the `foxxycode http` process to the **FoxxyCode** output
channel (View → Output → FoxxyCode). If the UI goes blank, run **FoxxyCode: Open DevTools** to
inspect the embedded webview; uncaught JS errors in the SPA also surface as a red overlay at the
bottom of the panel.

## Layout

```
src/
  extension.ts                    activation, command registration, process wiring
  settings.ts                     typed wrapper over the `foxxycode.*` settings
  notifications.ts                info/warn/error helpers
  i18n/bundle.ts                  localized strings (en + ru); 1:1 with the IntelliJ bundle
  package.nls.json                VS Code NLS (English) for package.json %key% surfaces
  package.nls.ru.json             VS Code NLS (Russian) for package.json %key% surfaces
  binary/binaryResolver.ts        OS/arch → bundled foxxycode binary path; validate()
  process/portUtil.ts             free-port picker
  process/processManager.ts       per-workspace process lifecycle + readiness polling
  util/http.ts                    GET (readiness) + POST (permission) helpers
  webview/panel.ts                WebviewPanel + WebviewView host; iframe + CSP + theme/lang sync
  webview/themeBridge.ts          VS Code color theme → foxxycode theme id
  webview/firstRun.ts             first-run walkthrough opener + WALKTHROUGH_ID
  diff/editEvent.ts               one `edit_proposed`/`edit_applied` SSE event
  diff/ideEventClient.ts          SSE reader for `/foxxycode/ide/events`
  diff/lineFragments.ts           pure line-diff helper (testable)
  diff/ideDiffService.ts          decorations + Accept/Reject/Revert + diff editor + revert
scripts/
  prepare-binary.mjs              cross-compile foxxycode for one or all targets
test/                             vitest unit tests (no vscode dependency)
```
