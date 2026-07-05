import * as vscode from "vscode";

/** Full walkthrough category id: `${publisher}.${name}.${walkthroughId}`. */
export const WALKTHROUGH_ID = "foxxycode.foxxycode-vscode.foxxycode.welcome";

const STATE_KEY = "firstRunCompleted";

/** Open the FoxxyCode onboarding walkthrough on the Welcome tab. */
export async function openWelcomeWalkthrough(force = true): Promise<void> {
  await vscode.commands.executeCommand("workbench.action.openWalkthrough", {
    category: WALKTHROUGH_ID,
    force,
  });
}

/** First-run helper: opens the walkthrough once per VS Code profile.
 *  Re-runnable via **FoxxyCode: Show Welcome** or by clearing globalState `firstRunCompleted`.
 *  Mirrors `editors/intellij/.../ui/FirstRunDialog.kt`. */
export async function showFirstRunIfNeeded(context: vscode.ExtensionContext): Promise<void> {
  if (context.globalState.get<boolean>(STATE_KEY)) return;
  await openWelcomeWalkthrough(true);
  await context.globalState.update(STATE_KEY, true);
}
