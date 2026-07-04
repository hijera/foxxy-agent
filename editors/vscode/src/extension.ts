import * as vscode from "vscode";
import { ProcessManager } from "./process/processManager";
import { IdeDiffService } from "./diff/ideDiffService";
import {
  FoxxyCodePanelController,
} from "./webview/panel";
import { showFirstRunIfNeeded } from "./webview/firstRun";
import {
  onSettingsChanged,
  openSettingsUi,
  readSettings,
  syncLocaleContext,
} from "./settings";
import { t } from "./i18n/bundle";
import { error, withProgress } from "./notifications";

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
 *    5. Dispose the child process on deactivate / window close.
 *
 *  Activation is lazy: the extension activates when the FoxxyCode sidebar view
 *  is opened (`onView:foxxycode.view`) or the Open Panel command is invoked
 *  (`onCommand:foxxycode.openPanel`). The server is started from inside the
 *  view/panel resolver so the controller always receives its base URL — this
 *  mirrors the working coddy-vscode extension and avoids the "infinite loading"
 *  race that an eager start-on-activate would cause. */

let processManager: ProcessManager | null = null;
let diffService: IdeDiffService | null = null;
let viewProvider: FoxxyCodeViewProvider | null = null;
let editorPanel: vscode.WebviewPanel | null = null;
let editorPanelController: FoxxyCodePanelController | null = null;
let editorPanelDisposed = false;
let activationOutput: vscode.OutputChannel | null = null;
let currentUrl: string | null = null;

export function activate(context: vscode.ExtensionContext): void {
  activationOutput = vscode.window.createOutputChannel("FoxxyCode");
  context.subscriptions.push(activationOutput);
  activationOutput.appendLine(`[foxxycode] activate (ext=${context.extensionPath})`);
  syncLocaleContext();

  const workspaceRoot = currentWorkspaceRoot();
  const log = (line: string): void => activationOutput?.appendLine(line);

  processManager = new ProcessManager({
    extensionPath: context.extensionPath,
    workspaceRoot,
    settings: readSettings(),
    log,
  });
  diffService = new IdeDiffService(workspaceRoot, log);

  // Activity bar webview view. The view provider owns the start flow so the
  // controller always gets its base URL (see class doc).
  viewProvider = new FoxxyCodeViewProvider(vscode.Uri.file(context.extensionPath), processManager, diffService, (url) => {
    currentUrl = url;
  });
  context.subscriptions.push(
    vscode.window.registerWebviewViewProvider("foxxycode.view", viewProvider, {
      webviewOptions: { retainContextWhenHidden: true },
    }),
  );

  // Commands (English + Russian palette variants share the same handlers).
  registerCommandPair(context, "foxxycode.openPanel", () => openEditorPanel(context));
  registerCommandPair(context, "foxxycode.restart", () => void restartActive());
  registerCommandPair(context, "foxxycode.reload", () => activeController()?.reload());
  registerCommandPair(context, "foxxycode.openInBrowser", () =>
    void activeController()?.openInBrowser(),
  );
  registerCommandPair(context, "foxxycode.openDevtools", () =>
    activeController()?.openDevtools(),
  );
  registerCommandPair(context, "foxxycode.openSettings", () => void openSettingsUi());
  registerCommandPair(context, "foxxycode.showLogs", () => activationOutput?.show());

  // Live locale refresh + re-read settings for the next process start.
  context.subscriptions.push(
    onSettingsChanged(() => {
      syncLocaleContext();
      // Re-snapshot settings so the next start()/restart() uses fresh values.
      if (processManager) (processManager as any).opts.settings = readSettings();
      activeController()?.refresh();
    }),
  );

  // First-run info message (non-blocking).
  void showFirstRunIfNeeded(context);
}

export function deactivate(): void {
  diffService?.dispose();
  processManager?.dispose();
  editorPanelController?.dispose();
  viewProvider?.controller?.dispose();
}

// ---- helpers ---------------------------------------------------------------

/** Register `id` and `id.ru` with the same handler (Russian palette title variant). */
function registerCommandPair(
  context: vscode.ExtensionContext,
  id: string,
  handler: (...args: unknown[]) => unknown,
): void {
  context.subscriptions.push(
    vscode.commands.registerCommand(id, handler),
    vscode.commands.registerCommand(`${id}.ru`, handler),
  );
}

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

function showStartFailedNotification(msg: string): void {
  void error(
    `${t("notification.title.startFailed")} — ${msg}`,
    t("process.button.openSettings"),
  ).then((choice) => {
    if (choice === t("process.button.openSettings")) {
      void openSettingsUi();
    }
  });
}

/** Start or restart the server with a notification progress indicator. */
async function startServer(mode: "start" | "restart"): Promise<{ baseUrl: string }> {
  if (!processManager) throw new Error(t("process.error.startFailed"));
  const pm = processManager;
  let progressReport: ((increment: number, message?: string) => void) | null = null;
  (pm as any).opts.onLaunching = (host: string, port: number) => {
    progressReport?.(0, t("process.indicator.launching", host, String(port)));
  };

  const title = mode === "restart" ? t("process.status.restarting") : t("process.status.starting");
  return withProgress(title, async (report) => {
    progressReport = report;
    return mode === "restart" ? pm.restart() : pm.start();
  });
}

/** Shared start flow used by both the sidebar view and the editor panel.
 *  Shows a status message, starts the server, points the controller at it,
 *  and wires the diff service. On error, shows an error view with Retry. */
async function startController(controller: FoxxyCodePanelController): Promise<void> {
  controller.showStatus(t("process.status.starting"));
  try {
    const { baseUrl } = await startServer("start");
    await controller.setBaseUrl(baseUrl);
    diffService?.startIfNeeded(baseUrl);
  } catch (e) {
    const msg = (e as Error).message ?? String(e);
    activationOutput?.appendLine(`[foxxycode] start failed: ${msg}`);
    controller.showError(t("process.error.startFailedPanel", msg));
    showStartFailedNotification(msg);
  }
}

/** Restart the server, then re-render whichever controller is active. */
async function restartActive(): Promise<void> {
  if (!processManager) return;
  const controller = activeController();
  if (!controller) return;
  controller.showStatus(t("process.status.restarting"));
  try {
    const { baseUrl } = await startServer("restart");
    await controller.setBaseUrl(baseUrl);
    diffService?.startIfNeeded(baseUrl);
  } catch (e) {
    const msg = (e as Error).message ?? String(e);
    activationOutput?.appendLine(`[foxxycode] restart failed: ${msg}`);
    controller.showError(t("process.error.startFailedPanel", msg));
    showStartFailedNotification(msg);
  }
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
    onRetry: () => void startController(editorPanelController!),
    onOpenSettings: () => void openSettingsUi(),
  });
  // Surface the editor-panel toolbar buttons (gated by `foxxycode.editorPanelActive`).
  void vscode.commands.executeCommand("setContext", "foxxycode.editorPanelActive", true);
  panel.onDidDispose(() => {
    editorPanelController?.dispose();
    editorPanelController = null;
    editorPanel = null;
    editorPanelDisposed = true;
    void vscode.commands.executeCommand("setContext", "foxxycode.editorPanelActive", false);
  });
  // If the server is already running (sidebar view opened first), reuse its URL;
  // otherwise boot the server now from the panel resolver.
  if (processManager?.baseUrl) {
    void editorPanelController.setBaseUrl(processManager.baseUrl);
  } else {
    void startController(editorPanelController);
  }
}

// ---- view provider for the activitybar webview view -------------------------

class FoxxyCodeViewProvider implements vscode.WebviewViewProvider {
  public controller: FoxxyCodePanelController | null = null;

  constructor(
    private readonly extensionUri: vscode.Uri,
    private readonly server: ProcessManager,
    private readonly diffService: IdeDiffService,
    private readonly onUrl: (url: string) => void,
  ) {}

  resolveWebviewView(view: vscode.WebviewView): void {
    this.controller = new FoxxyCodePanelController(view.webview, view, {
      extensionUri: this.extensionUri,
      onUrl: this.onUrl,
      onRetry: () => void this.start(),
      onOpenSettings: () => void openSettingsUi(),
    });
    void this.start();
  }

  /** Start the server (if needed) and show the embedded UI in this view. */
  async start(): Promise<void> {
    const controller = this.controller;
    if (!controller || !processManager) return;
    controller.showStatus(t("process.status.starting"));
    try {
      const { baseUrl } = await startServer("start");
      await controller.setBaseUrl(baseUrl);
      this.diffService.startIfNeeded(baseUrl);
    } catch (e) {
      const msg = (e as Error).message ?? String(e);
      activationOutput?.appendLine(`[foxxycode] start failed: ${msg}`);
      controller.showError(t("process.error.startFailedPanel", msg));
      showStartFailedNotification(msg);
    }
  }
}
