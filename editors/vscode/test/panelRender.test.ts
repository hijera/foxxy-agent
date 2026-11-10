import { beforeEach, expect, it, vi } from "vitest";

const host = vi.hoisted(() => ({ asExternalUri: vi.fn() }));
const disposable = () => ({ dispose() {} });
vi.mock("vscode", () => ({
  Uri: { parse: (value: string) => ({ toString: () => value }) },
  env: { language: "en", asExternalUri: (uri: unknown) => host.asExternalUri(uri) },
  window: { onDidChangeActiveColorTheme: () => disposable() },
  workspace: { onDidChangeConfiguration: () => disposable() },
}));
vi.mock("../src/settings", () => ({ readSettings: () => ({ followVscodeTheme: false }) }));
vi.mock("../src/webview/themeBridge", () => ({ currentFoxxyCodeTheme: () => "dark" }));
vi.mock("../src/i18n/bundle", () => ({ t: (_key: string, value: string) => value, spaLanguageCode: () => "en" }));
vi.mock("../src/notifications", () => ({ info: vi.fn() }));

import { FoxxyCodePanelController } from "../src/webview/panel";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}
function uri(value: string) {
  return { toString: () => value };
}

beforeEach(() => host.asExternalUri.mockReset());

it("does not commit an older asynchronous render over the current panel", async () => {
  const first = deferred<any>();
  const second = deferred<any>();
  host.asExternalUri.mockImplementation((value: any) => value?.toString().includes("42001") ? first.promise : second.promise);
  const webview = { html: "", options: {}, onDidReceiveMessage: () => disposable(), postMessage: async () => true };
  const controller = new FoxxyCodePanelController(webview as any, {} as any, { extensionUri: {} as any });

  const oldRender = controller.setBaseUrl("http://127.0.0.1:42001/");
  await vi.waitFor(() => expect(host.asExternalUri).toHaveBeenCalledOnce());
  controller.showStatus("Restarting");
  const currentRender = controller.setBaseUrl("http://127.0.0.1:42002/");
  second.resolve(uri("http://127.0.0.1:42002/"));
  await currentRender;
  expect(webview.html).toContain("http://127.0.0.1:42002/");

  first.resolve(uri("http://127.0.0.1:42001/"));
  await oldRender;
  expect(webview.html).toContain("http://127.0.0.1:42002/");
  expect(webview.html).not.toContain("http://127.0.0.1:42001/");
});
