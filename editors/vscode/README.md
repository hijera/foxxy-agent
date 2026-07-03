# Foxxy for VS Code (scaffold)

> **Status: scaffolding only.** The directory, manifest, and build script exist so the extension
> slots in beside the [JetBrains plugin](../intellij) without any restructuring. The activation
> logic in [`src/extension.ts`](src/extension.ts) is a documented stub — not yet implemented, not
> yet wired into CI.

## Intended design

Same contract as the IntelliJ client (see [../README.md](../README.md)): bundle a full-feature
`foxxy` binary, start `foxxy http --cwd <workspace>` on a free port, and embed the foxxy SPA in a
`WebviewPanel`.

## Build model

VS Code ships **platform-specific VSIX** packages, so — unlike the IntelliJ plugin, which bundles
every target in one zip — each VSIX bundles only its own target's binary:

```sh
# Prepare the matching binary, then package per target:
node scripts/prepare-binary.mjs --target linux-amd64
npx vsce package --target linux-x64

node scripts/prepare-binary.mjs --target darwin-arm64
npx vsce package --target darwin-arm64
# …one per target: linux-x64, linux-arm64, darwin-x64, darwin-arm64, win32-x64
```

`scripts/prepare-binary.mjs` runs the **same** `GOOS/GOARCH go build -tags "http ui scheduler
memory" ./cmd/coddy/` cross-compile the Gradle build uses, from the repo root. There is no separate
Go source — the repo root is the single source of truth.

## Next steps to implement

1. Port the binary resolver (os/arch → `foxxy-bin/<os>-<arch>/foxxy[.exe]`) and process manager
   from `../intellij/src/main/kotlin/dev/foxxy/intellij/{binary,process}`.
2. Implement the webview panel + theme sync (map VS Code color theme → `?theme=` / `window.foxxyUi`).
3. Add a `.github/workflows/vscode-plugin.yaml` matrix (one job per `vsce --target`) mirroring
   `intellij-plugin.yaml`, and wire it into `tag-on-merge.yaml`.
