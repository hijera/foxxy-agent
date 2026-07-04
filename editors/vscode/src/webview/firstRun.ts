import * as vscode from "vscode";
import { t } from "../i18n/bundle";
import { openSettingsUi } from "../settings";

/** First-run helper: informs the user that the foxxycode binary is bundled with
 *  the extension and lets them start immediately. Re-runnable by clearing the
 *  globalState flag. Mirrors `editors/intellij/.../ui/FirstRunDialog.kt`. */

const STATE_KEY = "firstRunCompleted";

export async function showFirstRunIfNeeded(context: vscode.ExtensionContext): Promise<void> {
  if (context.globalState.get<boolean>(STATE_KEY)) return;
  const choice = await vscode.window.showInformationMessage(
    t("firstrun.body"),
    t("firstrun.openSettings"),
    t("firstrun.dismiss"),
  );
  if (choice === t("firstrun.openSettings")) {
    await openSettingsUi();
  }
  await context.globalState.update(STATE_KEY, true);
}
