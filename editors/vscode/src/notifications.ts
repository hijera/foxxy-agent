import * as vscode from "vscode";

/** Thin wrappers over `vscode.window.show*Message` mirroring `FoxxyCodeNotifications.kt`. */

export function info(message: string, ...actions: string[]): Thenable<string | undefined> {
  return vscode.window.showInformationMessage(message, ...actions);
}

export function warn(message: string, ...actions: string[]): Thenable<string | undefined> {
  return vscode.window.showWarningMessage(message, ...actions);
}

export function error(message: string, ...actions: string[]): Thenable<string | undefined> {
  return vscode.window.showErrorMessage(message, ...actions);
}

/** Status-bar progress helper for long-running background operations. */
export function withProgress<R>(
  title: string,
  task: (report: (increment: number, message?: string) => void) => Promise<R>,
): Thenable<R> {
  return vscode.window.withProgress(
    { location: vscode.ProgressLocation.Notification, title, cancellable: false },
    async (progress) => {
      let total = 0;
      const report = (increment: number, message?: string) => {
        total += increment;
        progress.report({ increment, message });
      };
      return task(report);
    },
  );
}
