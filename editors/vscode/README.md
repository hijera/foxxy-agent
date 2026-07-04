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

1. Open the **FoxxyCode** view from the activity bar (left side). The extension starts the bundled
   `foxxycode http` and shows the embedded UI. Use **FoxxyCode: Open Panel** in the Command Palette
   to open the same UI in an editor tab.
2. In the UI's **Settings**, set a provider API key (or define `OPENAI_API_KEY` in the environment
   before launching VS Code), then chat, schedule jobs, browse skills, use plan mode, etc.

Reconfigure anytime in **Settings → Extensions → FoxxyCode**: optional binary path override (leave
empty to use the bundled foxxycode), host, port (0 = auto), FoxxyCode home, extra `foxxycode http`
args, **"Match FoxxyCode UI theme to the VS Code color theme"**, native inline diffs, auto-apply
edits, and UI language. Toolbar buttons on the webview title bar: Restart, Reload, Open in Browser,
Open DevTools, Settings.

### Language

The extension UI is available in **English** and **Russian**:

- **Runtime strings** (notifications, first-run message, diff prompts, progress indicators, error views) follow the **FoxxyCode: Language** setting (`foxxycode.language`):
  - **System** (default) — follows `vscode.env.language` (the VS Code display language).
  - **English** / **Русский** — force a specific language.
- **Command titles** in the Command Palette and webview toolbar also follow `foxxycode.language` (dual command variants gated by `foxxycode.locale`).
- **Settings descriptions** and the activity-bar view container title follow the **VS Code display language** via `package.nls.json` / `package.nls.ru.json`. VS Code resolves those statically from `vscode.env.language`; they cannot be overridden by `foxxycode.language` alone. When **System** is selected, everything stays in sync.

The selected language is forwarded to the embedded SPA via the `?lang=` URL parameter and applied on every iframe reload. Missing translation keys fall back to English automatically.

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
  webview/firstRun.ts             first-run info message
  diff/editEvent.ts               one `edit_proposed`/`edit_applied` SSE event
  diff/ideEventClient.ts          SSE reader for `/foxxycode/ide/events`
  diff/lineFragments.ts           pure line-diff helper (testable)
  diff/ideDiffService.ts          decorations + Accept/Reject/Revert + diff editor + revert
scripts/
  prepare-binary.mjs              cross-compile foxxycode for one or all targets
test/                             vitest unit tests (no vscode dependency)
```
