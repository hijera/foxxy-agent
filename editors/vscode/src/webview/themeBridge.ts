import * as vscode from "vscode";

/** Bridges the VS Code color theme to the foxxycode web UI theme.
 *  Mirrors `editors/intellij/.../ui/FoxxyCodeThemeBridge.kt`.
 *
 *  Only `light` is a light theme; every other id is dark. We map the VS Code
 *  ColorThemeKind to `light` / `dark`. See `docs/intellij-embedding.md` for
 *  the full `?theme=` / `window.foxxycodeUi.setTheme` contract. */

export type FoxxyCodeThemeId = "light" | "dark";

/** foxxycode theme id matching the current VS Code color theme. */
export function currentFoxxyCodeTheme(
  theme: vscode.ColorTheme = vscode.window.activeColorTheme,
): FoxxyCodeThemeId {
  return theme.kind === vscode.ColorThemeKind.Light ? "light" : "dark";
}
