import * as vscode from "vscode";
import { ProcessManager } from "./process/processManager";
import { IdeDiffService } from "./diff/ideDiffService";
import {
  FoxxyCodePanelController,
  FoxxyCodeViewProvider,
} from "./webview/panel";
import { showFirstRunIfNeeded } from "./webview/firstRun";
import {
  onSettingsChanged,
  openSettingsUi,
  readSettings,
  refreshLocale,
} from "./settings";
import { error, info } from "./notifications";
import { t } from "./i18n/bundle";

/** FoxxyCode VS Code extension — full port of the JetBrains plugin.
 *
 *  Contract (identical to the IntelliJ client, see ../README.md):
 *    1. Resolve the bundled foxxycode binary for the running platform:
 *         <extension>/foxxycode-bin/<goos>-<goarch>/foxxycode[.exe]
 *       or the `foxxycode.binaryPath` setting override.
 *    2. Start `foxxycode http -H <host> -P <port> --cwd <workspaceRoot> [--home <home>]`
 *       on a free localhost port (port 0 => auto-pick).
 *    3. Poll `http://host:port/v1/models` until ready (30s), then point a
 *       WebviewPanel/WebviewView iframe at
 *       `http://host:port/?theme=<vscodeTheme>&lang=<lang>&embed=intellij`.
 *    4. Subscribe to `GET /foxxycode/ide/events` for native inline diffs.
 *    5. Dispose the child process on deactivate / window close. */

let processManager: ProcessManager | null = null;
let diffService: IdeDiffService | null = null;
let viewProvider: FoxxyCodeViewProvider | null = null;
let editorPanel: vscode.WebviewPanel | null = null;
let editorPanelController: FoxxyCodePanelController | null = null;
let editorPanelDisposed = false;
let activationOutput: vscode.OutputChannel | null = null;
let currentUrl: string | null = null;

export function activate(context: vscode.ExtensionContext): void {
  refreshLocale();
  activationOutput = vscode.window.createOutputChannel("FoxxyCode");
  context.subscriptions.push(activationOutput);

  const workspaceRoot = currentWorkspaceRoot();
  const log = (line: string): void => activationOutput?.appendLine(line);

  processManager = new ProcessManager({
    extensionPath: context.extensionPath,
    workspaceRoot,
    settings: readSettings(),
    log,
  });
  diffService = new IdeDiffService(workspaceRoot, log);

  // Activity bar webview view.
  viewProvider = new FoxxyCodeViewProvider(context.extensionUri, (url) => {
    currentUrl = url;
  });
  context.subscriptions.push(
    vscode.window.registerWebviewViewProvider("foxxycode.view", viewProvider, {
      webviewOptions: { retainContextWhenHidden: true },
    }),
  );

  // Commands.
  context.subscriptions.push(
    vscode.commands.registerCommand("foxxycode.openPanel", () => openEditorPanel(context)),
    vscode.commands.registerCommand("foxxycode.restart", () => void restartServer()),
    vscode.commands.registerCommand("foxxycode.reload", () => activeController()?.reload()),
    vscode.commands.registerCommand("foxxycode.openInBrowser", () =>
      void activeController()?.openInBrowser(),
    ),
    vscode.commands.registerCommand("foxxycode.openDevtools", () =>
      activeController()?.openDevtools(),
    ),
    vscode.commands.registerCommand("foxxycode.openSettings", () => void openSettingsUi()),
  );

  // Live locale refresh + re-read settings for the next process start.
  context.subscriptions.push(
    onSettingsChanged(() => {
      refreshLocale();
      // Re-snapshot settings so the next start()/restart() uses fresh values.
      if (processManager) (processManager as any).opts.settings = readSettings();
      activeController()?.refresh();
    }),
  );

  // `foxxycode.active` gates the webview/title toolbar commands in package.json.
  void vscode.commands.executeCommand("setContext", "foxxycode.active", true);

  // First-run info message (non-blocking).
  void showFirstRunIfNeeded(context);

  // Kick off the process + wire the diff service once it's ready.
  void ensureStartedAndWire();
}

export function deactivate(): void {
  diffService?.dispose();
  processManager?.dispose();
  editorPanelController?.dispose();
  viewProvider?.controller?.dispose();
  void vscode.commands.executeCommand("setContext", "foxxycode.active", false);
}

// ---- helpers ---------------------------------------------------------------

/** Returns the controller for the currently focused webview (editor panel takes
 *  precedence over the activity bar view), or `null` if neither is ready. */
function activeController(): FoxxyCodePanelController | null {
  if (editorPanelController && editorPanel && !editorPanelDisposed) return editorPanelController;
  return viewProvider?.controller ?? null;
}

function currentWorkspaceRoot(): string | undefined {
  const folders = vscode.workspace.workspaceFolders;
  return folders && folders.length > 0 ? folders[0].uri.fsPath : undefined;
}

async function ensureStartedAndWire(): Promise<void> {
  try {
    const { baseUrl } = await processManager!.start();
    pushBaseUrlToControllers(baseUrl);
    diffService?.startIfNeeded(baseUrl);
  } catch (e) {
    reportStartFailure((e as Error).message ?? String(e));
  }
}

async function restartServer(): Promise<void> {
  if (!processManager) return;
  info(t("process.status.restarting"));
  try {
    const { baseUrl } = await processManager.restart();
    pushBaseUrlToControllers(baseUrl);
    diffService?.startIfNeeded(baseUrl);
  } catch (e) {
    reportStartFailure((e as Error).message ?? String(e));
  }
}

function pushBaseUrlToControllers(baseUrl: string): void {
  viewProvider?.controller?.setBaseUrl(baseUrl);
  editorPanelController?.setBaseUrl(baseUrl);
  currentUrl = baseUrl;
}

function reportStartFailure(msg: string): void {
  activationOutput?.appendLine(`[foxxycode] start failed: ${msg}`);
  void error(
    t("process.error.startFailedPanel", msg),
    t("process.button.retry"),
    t("process.button.openSettings"),
  ).then((choice) => {
    if (choice === t("process.button.retry")) void restartServer();
    else if (choice === t("process.button.openSettings")) void openSettingsUi();
  });
}

function openEditorPanel(context: vscode.ExtensionContext): void {
  if (editorPanel && !editorPanelDisposed) {
    editorPanel.reveal(vscode.ViewColumn.Active);
    return;
  }
  const panel = vscode.window.createWebviewPanel(
    "foxxycode.panel",
    "FoxxyCode",
    vscode.ViewColumn.Active,
    {
      enableScripts: true,
      enableForms: true,
      retainContextWhenHidden: true,
    },
  );
  editorPanel = panel;
  editorPanelDisposed = false;
  editorPanelController?.dispose();
  editorPanelController = new FoxxyCodePanelController(panel.webview, panel, {
    extensionUri: context.extensionUri,
    onUrl: (url) => {
      currentUrl = url;
    },
  });
  panel.onDidDispose(() => {
    editorPanelController?.dispose();
    editorPanelController = null;
    editorPanel = null;
    editorPanelDisposed = true;
  });
  if (processManager?.baseUrl) editorPanelController.setBaseUrl(processManager.baseUrl);
}
