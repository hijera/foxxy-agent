# Editor integrations

Thin editor clients that embed the **foxxycode** agent's web UI. Both follow the same contract, so the
build tooling and the bundled binaries are shared in spirit:

```
editor  ──webview/tool window──▶  foxxycode web UI  ──http──▶  foxxycode http   (one process per workspace)
                                                             --cwd  = workspace/project root
                                                             --home = ~/.foxxycode (shared config & history)
```

Each client:

1. Ships a **full-feature** `foxxycode` binary (`http ui scheduler memory`) bundled per desktop target,
   cross-compiled from the repo root (`./cmd/foxxycode/`).
2. Resolves the binary for the running OS/arch, starts `foxxycode http` on a free localhost port with
   `--cwd` set to the open project, and points an embedded browser at it.
3. Re-uses the entire foxxycode SPA — nothing is re-implemented natively.

## Clients

| Dir                | Status      | Packaging                                                        |
| ------------------ | ----------- | --------------------------------------------------------------- |
| [`intellij/`](intellij) | Implemented | One cross-platform plugin zip; `foxxycode-bin/<os>-<arch>/foxxycode[.exe]`. Built via `make intellij-build`. |
| [`vscode/`](vscode)     | Scaffold    | Platform-specific VSIX (`vsce package --target <os>-<arch>`), bundling only that target's binary.    |

## Shared binary layout

Both clients expect the bundled binary under `foxxycode-bin/<goos>-<goarch>/foxxycode[.exe]`, where the
targets mirror `.github/workflows/release-binaries.yaml`:
`linux-amd64, linux-arm64, darwin-amd64, darwin-arm64, windows-amd64`.

The cross-compile is a plain `GOOS=<os> GOARCH=<arch> CGO_ENABLED=0 go build -tags "http ui
scheduler memory" ./cmd/foxxycode/`. The IntelliJ plugin drives this from Gradle; the VSCode extension
will drive the same command from its build script (see `vscode/README.md`). There is no separate Go
source tree — the repo root is the single source of truth.
