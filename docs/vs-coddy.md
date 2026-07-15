# What FoxxyCode adds over coddy-agent

FoxxyCode is a fork of upstream [coddy-agent](https://github.com/coddy-project/coddy-agent)
(by the Coddy project, MIT). It keeps the upstream architecture — ACP harness, ReAct loop,
sessions, prompts, providers, MCP merge — and layers on an IDE/desktop integration story plus a
few agent capabilities that do not exist upstream.

This page lists the **fork-specific additions**. It reflects the fork's own history, so upstream
may converge on some of these over time; see [`UPSTREAM_SYNC.md`](../UPSTREAM_SYNC.md) for the
exact upstream commit this fork is synced to.

## IDE & desktop integration (the fork's main focus)

- **Native desktop window (WebView2)** - a standalone desktop build (`foxxycode-desktop.exe`) that
  hosts the embedded UI in a native window instead of a browser tab - see
  [`cmd/foxxycode/desktop.go`](../cmd/foxxycode/desktop.go), [`internal/desktop/`](../internal/desktop).
- **Desktop notifications + audio chime + nav plus icon** - bottom-right toasts and a WebAudio cue
  for permission prompts and plan-ready events, with a plus-icon brand mark on the nav rail.
- **Guided tour / onboarding** - a first-run desktop tour after the settings form closes, plus a
  "Restart onboarding" control in Appearance to replay it.
- **Project folder picker** - open projects as folders from the UI - see
  [`internal/project/`](../internal/project) and the `/foxxycode/project` route.
- **IDE open-files context** - the editor sends its open tabs and active file to the backend, which
  injects them into the prompt as `<foxxycode_ide_context>` - see
  [`internal/ideenv/`](../internal/ideenv), wired in `internal/agent/react.go`.
- **IDE terminal tracking (`@terminal`)** - terminal output is streamed to the backend
  (`POST /foxxycode/ide/terminal-state`) and exposed as always-on `<foxxycode_terminal_context>`
  and an explicit `@terminal` mention - see [`internal/ideterm/`](../internal/ideterm) and
  [`external/httpserver/ideterminalstate.go`](../external/httpserver/ideterminalstate.go).
- **IDE file drag-drop -> `@`-mention** - dropping a file into the composer produces a short
  `@`-chip while the full relative path is sent to the model, via the `/workspace/relativize` endpoint.
- **Native IntelliJ inline diffs** - plugin-side inline diff review with Accept/Reject and
  checkpoints, instead of separate diff panes.

## Agent capabilities

- **Interactive browser tool** - a `browser_action`-style toolset that drives a real
  Chrome/Chromium instance via chromedp (open, click, fill, hover, scroll, run JS) and returns
  **screenshots** to the model. Built behind the `browser` build tag - see
  [`docs/browser-tool.md`](browser-tool.md) and [`internal/tools/browser/`](../internal/tools/browser).
- **Automatic context compaction** - long conversations are auto-summarized to stay within the
  context window; config-gated and enabled by default - see
  [`internal/agent/compaction.go`](../internal/agent/compaction.go) and
  [`internal/config/compaction.go`](../internal/config/compaction.go) (configuration in
  [`docs/config.md`](config.md)).

## Localization & distribution

- **Settings-form i18n (RU overlay)** - a Russian translation of the settings schema, rendered as a
  frontend overlay driven off the English Go schema - see
  [`external/ui/src/ui/i18n/messages/schema.ru.ts`](../external/ui/src/ui/i18n/messages/schema.ru.ts).
- **Full `foxxyCode` rebrand** - the distribution is renamed end to end: Go module path, binary
  name, env vars (`FOXXYCODE_HOME` / `FOXXYCODE_CWD` / `FOXXYCODE_CONFIG`), HTTP routes
  (`/foxxycode/*`), home directory (`~/.foxxycode`), tool names (`foxxycode_*`), and CSS tokens
  (`--foxxycode-*`), while staying architecturally close to upstream.

---

For how the fork tracks and ports upstream changes, see [`UPSTREAM_SYNC.md`](../UPSTREAM_SYNC.md).
