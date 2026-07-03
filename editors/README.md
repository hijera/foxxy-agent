# Editor integrations

Thin editor clients that embed the **foxxy** agent's web UI. Both follow the same contract, so the
build tooling and the bundled binaries are shared in spirit:

```
editor  ──webview/tool window──▶  foxxy web UI  ──http──▶  foxxy http   (one process per workspace)
                                                             --cwd  = workspace/project root
                                                             --home = ~/.coddy (shared config & history)
```

Each client:

1. Ships a **full-feature** `foxxy` binary (`http ui scheduler memory`) bundled per desktop target,
   cross-compiled from the repo root (`./cmd/coddy/`).
2. Resolves the binary for the running OS/arch, starts `foxxy http` on a free localhost port with
   `--cwd` set to the open project, and points an embedded browser at it.
3. Re-uses the entire foxxy SPA — nothing is re-implemented natively.

## Clients

| Dir                | Status      | Packaging                                                        |
| ------------------ | ----------- | --------------------------------------------------------------- |
| [`intellij/`](intellij) | Implemented | One cross-platform plugin zip; `foxxy-bin/<os>-<arch>/foxxy[.exe]`. Built via `make intellij-build`. |
| [`vscode/`](vscode)     | Scaffold    | Platform-specific VSIX (`vsce package --target <os>-<arch>`), bundling only that target's binary.    |

## Shared binary layout

Both clients expect the bundled binary under `foxxy-bin/<goos>-<goarch>/foxxy[.exe]`, where the
targets mirror `.github/workflows/release-binaries.yaml`:
`linux-amd64, linux-arm64, darwin-amd64, darwin-arm64, windows-amd64`.

The cross-compile is a plain `GOOS=<os> GOARCH=<arch> CGO_ENABLED=0 go build -tags "http ui
scheduler memory" ./cmd/coddy/`. The IntelliJ plugin drives this from Gradle; the VSCode extension
will drive the same command from its build script (see `vscode/README.md`). There is no separate Go
source tree — the repo root is the single source of truth.
