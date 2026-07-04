import * as vscode from "vscode";
import { localeFromSetting, setLocale, type Locale } from "./i18n/bundle";

/** All FoxxyCode settings surfaced under the `foxxycode.*` namespace. */
export interface FoxxyCodeSettings {
  binaryPath: string;
  host: string;
  port: number;
  home: string;
  extraArgs: string;
  language: "system" | "en" | "ru";
  followVscodeTheme: boolean;
  nativeDiffs: boolean;
  autoApproveEdits: boolean;
}

function readRaw(): vscode.WorkspaceConfiguration {
  return vscode.workspace.getConfiguration("foxxycode");
}

/** Snapshot of current settings (copied, so mutating the result does not affect live config). */
export function readSettings(): FoxxyCodeSettings {
  const c = readRaw();
  return {
    binaryPath: c.get<string>("binaryPath", ""),
    host: c.get<string>("host", "127.0.0.1") || "127.0.0.1",
    port: c.get<number>("port", 0),
    home: c.get<string>("home", ""),
    extraArgs: c.get<string>("extraArgs", ""),
    language: c.get<"system" | "en" | "ru">("language", "system"),
    followVscodeTheme: c.get<boolean>("followVscodeTheme", true),
    nativeDiffs: c.get<boolean>("nativeDiffs", true),
    autoApproveEdits: c.get<boolean>("autoApproveEdits", false),
  };
}

/** Active locale resolved from the language setting + VS Code display language. */
export function activeLocale(): Locale {
  const s = readSettings();
  return localeFromSetting(s.language, vscode.env.language);
}

/** Re-apply the i18n locale so `t()` uses the latest setting. */
export function refreshLocale(): Locale {
  const locale = activeLocale();
  setLocale(locale);
  return locale;
}

/** Refresh runtime i18n and push `foxxycode.locale` for command palette / toolbar `when` clauses. */
export function syncLocaleContext(): Locale {
  const locale = refreshLocale();
  void vscode.commands.executeCommand("setContext", "foxxycode.locale", locale);
  return locale;
}

/** Subscribe to changes of any `foxxycode.*` setting; returns a disposable. */
export function onSettingsChanged(cb: () => void): vscode.Disposable {
  return vscode.workspace.onDidChangeConfiguration((e) => {
    if (e.affectsConfiguration("foxxycode")) cb();
  });
}

/** Open the VS Code Settings UI filtered to the FoxxyCode namespace. */
export function openSettingsUi(): Thenable<void> {
  return vscode.commands.executeCommand("workbench.action.openSettings", "@ext:foxxycode.foxxycode-vscode");
}
