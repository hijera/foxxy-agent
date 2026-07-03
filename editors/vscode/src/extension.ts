// Foxxy VS Code extension — SCAFFOLD.
//
// This is a structural placeholder that mirrors the JetBrains plugin
// (../intellij). It documents the intended flow so the real implementation
// can be filled in without moving anything. It is NOT wired into CI yet.
//
// Contract (identical to the IntelliJ client, see ../README.md):
//
//   1. Resolve the bundled foxxy binary for the running platform:
//        <extension>/foxxy-bin/<goos>-<goarch>/foxxy[.exe]
//      or the `foxxy.binaryPath` setting override.
//   2. Start `foxxy http -H <host> -P <port> --cwd <workspaceRoot> [--home <home>]`
//      on a free localhost port (port 0 => auto-pick).
//   3. Poll `http://host:port/v1/models` until ready (30s), then open a
//      WebviewPanel whose iframe points at `http://host:port/?theme=<vscodeTheme>`.
//   4. Dispose the child process on deactivate / window close.
//
// Because VS Code ships platform-specific VSIX packages, each VSIX bundles only
// the single `foxxy-bin/<goos>-<goarch>/foxxy[.exe]` for its target (see
// scripts/prepare-binary.mjs), unlike the IntelliJ plugin which bundles all
// targets in one zip.

import * as vscode from "vscode";

export function activate(context: vscode.ExtensionContext): void {
  context.subscriptions.push(
    vscode.commands.registerCommand("foxxy.openPanel", () => {
      vscode.window.showInformationMessage(
        "Foxxy VS Code extension is scaffolding only — not implemented yet."
      );
      // TODO: resolveBinary() -> startFoxxyHttp() -> createWebviewPanel(url).
    }),
    vscode.commands.registerCommand("foxxy.restart", () => {
      // TODO: stop the child process and re-run startFoxxyHttp().
    })
  );
}

export function deactivate(): void {
  // TODO: terminate the foxxy http child process if running.
}
