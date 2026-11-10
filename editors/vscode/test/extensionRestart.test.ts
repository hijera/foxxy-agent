import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const host = vi.hoisted(() => ({
  commands: new Map<string, (...args: unknown[]) => unknown>(),
  provider: null as any, panel: null as any, controllers: [] as any[], process: null as any, restart: vi.fn(),
}));
const disposable = () => ({ dispose() {} });
vi.mock("vscode", () => ({
  Uri: { file: (path: string) => ({ fsPath: path }) }, ViewColumn: { Active: 1 },
  workspace: { workspaceFolders: [{ uri: { fsPath: "/workspace" } }] },
  commands: {
    registerCommand: (name: string, fn: (...args: unknown[]) => unknown) => { host.commands.set(name, fn); return { dispose() {} }; },
    executeCommand: async () => undefined,
  },
  window: {
    createOutputChannel: () => ({ appendLine() {}, dispose() {} }),
    registerWebviewViewProvider: (_: string, provider: unknown) => { host.provider = provider; return { dispose() {} }; },
    createWebviewPanel: () => host.panel,
  },
}));
vi.mock("../src/process/processManager", () => ({
  StartCancelledError: class extends Error {},
  ProcessManager: class {
    baseUrl: string | null = null;
    constructor(public opts: unknown) { host.process = this; }
    async start() { return { baseUrl: "http://127.0.0.1:41000/" }; }
    restart() { return host.restart(); }
    dispose() {}
  },
}));
vi.mock("../src/settings", () => ({
  readSettings: () => ({}), readHttpProxyEnv: () => ({}), syncLocaleContext() {},
  onSettingsChanged: () => ({ dispose() {} }), openSettingsUi: async () => {},
}));
vi.mock("../src/i18n/localeState", () => ({ initLocaleState() {}, fetchBackendLocale: async () => {}, setEffectiveLocale() {}, resolveLocale: () => "en" }));
vi.mock("../src/webview/firstRun", () => ({ showFirstRunIfNeeded: async () => {}, openWelcomeWalkthrough: async () => {} }));
vi.mock("../src/notifications", () => ({
  withProgress: (_: string, task: (report: () => void) => unknown) => task(() => {}),
  error: async () => undefined, info: async () => undefined, warn: async () => undefined,
}));
vi.mock("../src/webview/panel", () => ({
  MAX_PENDING_MENTIONS: 100,
  FoxxyCodePanelController: class {
    setBaseUrl = vi.fn(async (_: string) => {}); showStatus = vi.fn(); showError = vi.fn(); dispose = vi.fn(); insertFileMentions = vi.fn();
    constructor() { host.controllers.push(this); }
  },
}));
vi.mock("../src/diff/ideDiffService", () => ({ IdeDiffService: class { startIfNeeded() {} dispose() {} } }));
vi.mock("../src/ide/editorStateService", () => ({ EditorStateService: class { startIfNeeded() {} dispose() {} } }));
vi.mock("../src/ide/terminalStateService", () => ({ TerminalStateService: class { startIfNeeded() {} dispose() {} requestScreenCapture() {} } }));
vi.mock("../src/autocomplete/inlineCompletionService", () => ({ InlineCompletionService: class { startIfNeeded() {} dispose() {} } }));

let extension: typeof import("../src/extension");
let disposePanel: () => void;
beforeEach(async () => {
  vi.resetModules(); host.commands.clear(); host.controllers = [];
  host.restart.mockReset().mockResolvedValue({ baseUrl: "http://127.0.0.1:42000/" });
  host.panel = { webview: {}, visible: true, active: true, reveal() {}, onDidChangeViewState: disposable, onDidDispose: (fn: () => void) => { disposePanel = fn; return disposable(); } };
  extension = await import("../src/extension");
  extension.activate({ subscriptions: [], extensionPath: "/extension", extensionUri: {}, extension: { packageJSON: { version: "test" } }, globalState: { get() {}, update: async () => {} } } as any);
  host.commands.get("foxxycode.openPanel")!();
  host.provider.resolveWebviewView({ webview: {}, onDidChangeVisibility: disposable });
  await vi.waitFor(() => {
    expect(host.controllers).toHaveLength(2);
    for (const c of host.controllers) expect(c.setBaseUrl).toHaveBeenCalledWith("http://127.0.0.1:41000/");
  });
});
afterEach(() => extension.deactivate());

describe("Restart with an editor tab and sidebar", () => {
  it("updates every open panel to the replacement backend URL", async () => {
    host.commands.get("foxxycode.restart")!();
    await vi.waitFor(() => {
      for (const c of host.controllers) expect(c.setBaseUrl).toHaveBeenLastCalledWith("http://127.0.0.1:42000/");
    });
  });
  it("shows a failed restart in every panel instead of retaining a dead URL", async () => {
    host.restart.mockRejectedValue(new Error("bind failed")); host.commands.get("foxxycode.restart")!();
    await vi.waitFor(() => {
      for (const c of host.controllers) expect(c.showError).toHaveBeenCalledWith(expect.stringContaining("bind failed"));
    });
  });
  it("updates the remaining sidebar when the editor tab closes during restart", async () => {
    let ready!: (value: { baseUrl: string }) => void;
    host.restart.mockReturnValue(new Promise((resolve) => { ready = resolve; }));
    host.commands.get("foxxycode.restart")!(); disposePanel(); ready({ baseUrl: "http://127.0.0.1:42000/" });
    await vi.waitFor(() => expect(host.controllers[1].setBaseUrl).toHaveBeenLastCalledWith("http://127.0.0.1:42000/"));
    expect(host.controllers[0].setBaseUrl).toHaveBeenCalledTimes(1);
  });
  it("keeps a superseded launch off the error panels", async () => {
    const { StartCancelledError } = await import("../src/process/processManager");
    host.restart.mockRejectedValue(new StartCancelledError());
    host.commands.get("foxxycode.restart")!();
    await new Promise((resolve) => setImmediate(resolve));
    for (const controller of host.controllers) expect(controller.showError).not.toHaveBeenCalled();
  });
});
