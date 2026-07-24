import * as vscode from "vscode";
import { ProcessManager } from "./process/processManager";
import type { ProxyEnv } from "./process/proxyEnv";
import { IdeDiffService } from "./diff/ideDiffService";
import { EditorStateService } from "./ide/editorStateService";
import { TerminalStateService } from "./ide/terminalStateService";
import {
  FoxxyCodePanelController,
} from "./webview/panel";
import { formatPanelTitle, formatViewDescription } from "./webview/panelTitle";
import { showFirstRunIfNeeded, openWelcomeWalkthrough } from "./webview/firstRun";
import {
  onSettingsChanged,
  openSettingsUi,
  readHttpProxyEnv,
  readSettings,
  syncLocaleContext,
} from "./settings";
import { t, type Locale } from "./i18n/bundle";
import {
  fetchBackendLocale,
  initLocaleState,
  setEffectiveLocale,
} from "./i18n/localeState";
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
let editorStateService: EditorStateService | null = null;
let terminalStateService: TerminalStateService | null = null;
let viewProvider: FoxxyCodeViewProvider | null = null;
let editorPanel: vscode.WebviewPanel | null = null;
let editorPanelController: FoxxyCodePanelController | null = null;
let editorPanelDisposed = false;
let activationOutput: vscode.OutputChannel | null = null;
let currentUrl: string | null = null;
/** Signature of the last-applied proxy env, so we only restart when it actually changes. */
let lastProxyEnvSig = "";
/** globalState key caching the last backend UI locale (for pre-boot strings). */
const CACHED_LOCALE_KEY = "foxxycode.cachedLocale";
/** Plugin version from the manifest, shown next to the panel/view title. */
let extensionVersion = "";

/** Apply a locale switch originating from the embedded SPA: adopt it and refresh
 *  extension chrome (command titles via the `foxxycode.locale` context key). The
 *  SPA already re-rendered itself, so we do NOT reload the iframe. */
function onSpaLocale(locale: Locale): void {
  setEffectiveLocale(locale);
  syncLocaleContext();
}

export function activate(context: vscode.ExtensionContext): void {
  activationOutput = vscode.window.createOutputChannel("FoxxyCode");
  context.subscriptions.push(activationOutput);
  extensionVersion = String(
    (context.extension?.packageJSON as { version?: unknown } | undefined)
      ?.version ?? "",
  );
  activationOutput.appendLine(
    `[foxxycode] activate (ext=${context.extensionPath}, version=${extensionVersion || "unknown"})`,
  );
  // Seed the UI language from the last known backend value so pre-boot strings
  // already match; the authoritative ui.locale is fetched once the server is up.
  initLocaleState(context.globalState.get(CACHED_LOCALE_KEY), (locale) => {
    void context.globalState.update(CACHED_LOCALE_KEY, locale);
  });
  syncLocaleContext();

  const workspaceRoot = currentWorkspaceRoot();
  const log = (line: string): void => activationOutput?.appendLine(line);

  const initialProxyEnv = readHttpProxyEnv();
  lastProxyEnvSig = proxyEnvSig(initialProxyEnv);
  processManager = new ProcessManager({
    extensionPath: context.extensionPath,
    workspaceRoot,
    settings: readSettings(),
    proxyEnv: initialProxyEnv,
    log,
  });
  diffService = new IdeDiffService(workspaceRoot, log);
  editorStateService = new EditorStateService(log);
  terminalStateService = new TerminalStateService(log);

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
  registerCommandPair(context, "foxxycode.showWelcome", () => void openWelcomeWalkthrough(true));

  // Live locale refresh + re-read settings for the next process start.
  context.subscriptions.push(
    onSettingsChanged(() => {
      syncLocaleContext();
      // Re-snapshot settings so the next start()/restart() uses fresh values.
      const proxyEnv = readHttpProxyEnv();
      processManager?.updateLaunchOptions(readSettings(), proxyEnv);
      // A live proxy change only takes effect on restart: the running process keeps its
      // spawn-time env. Restart automatically so the new proxy applies without user action.
      const sig = proxyEnvSig(proxyEnv);
      if (sig !== lastProxyEnvSig && processManager?.isRunning) {
        void restartActive();
      }
      lastProxyEnvSig = sig;
      activeController()?.refresh();
    }),
  );

  // First-run info message (non-blocking).
  void showFirstRunIfNeeded(context);
}

export function deactivate(): void {
  diffService?.dispose();
  editorStateService?.dispose();
  terminalStateService?.dispose();
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

/** Order-independent signature of a proxy env, used to detect proxy setting changes. */
function proxyEnvSig(env: ProxyEnv): string {
  return Object.entries(env)
    .filter(([, v]) => v)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([k, v]) => `${k}=${v}`)
    .join("\n");
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
    // Adopt the backend UI language before the first iframe render so its
    // `?lang=` and the extension chrome agree from the first frame.
    await fetchBackendLocale(baseUrl);
    syncLocaleContext();
    await controller.setBaseUrl(baseUrl);
    diffService?.startIfNeeded(baseUrl);
    editorStateService?.startIfNeeded(baseUrl);
    terminalStateService?.startIfNeeded(baseUrl);
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
    await fetchBackendLocale(baseUrl);
    syncLocaleContext();
    await controller.setBaseUrl(baseUrl);
    diffService?.startIfNeeded(baseUrl);
    editorStateService?.startIfNeeded(baseUrl);
    terminalStateService?.startIfNeeded(baseUrl);
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
    formatPanelTitle(extensionVersion),
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
    onSpaLocale,
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
    // VS Code renders the description dimmed right after the view name, so the
    // header reads "FOXXYCODE  0.1.6" next to the toolbar buttons.
    view.description = formatViewDescription(extensionVersion);
    this.controller = new FoxxyCodePanelController(view.webview, view, {
      extensionUri: this.extensionUri,
      onUrl: this.onUrl,
      onRetry: () => void this.start(),
      onOpenSettings: () => void openSettingsUi(),
      onSpaLocale,
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
      await fetchBackendLocale(baseUrl);
      syncLocaleContext();
      await controller.setBaseUrl(baseUrl);
      this.diffService.startIfNeeded(baseUrl);
      editorStateService?.startIfNeeded(baseUrl);
      terminalStateService?.startIfNeeded(baseUrl);
    } catch (e) {
      const msg = (e as Error).message ?? String(e);
      activationOutput?.appendLine(`[foxxycode] start failed: ${msg}`);
      controller.showError(t("process.error.startFailedPanel", msg));
      showStartFailedNotification(msg);
    }
  }
}
