import { afterEach, expect, test, vi } from "vitest";
import { downloadText } from "./DiagramBlock";

afterEach(() => {
  vi.runOnlyPendingTimers();
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  window.history.replaceState(null, "", "/");
  window.sessionStorage.clear();
});

test("routes diagram downloads through the VS Code host without creating a blob", () => {
  vi.useFakeTimers();
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
  window.history.replaceState(null, "", "/?embed=vscode");
  const postMessage = vi.fn();
  vi.spyOn(window, "parent", "get").mockReturnValue({ postMessage } as unknown as Window);
  const createObjectURL = vi.fn(() => "blob:fixture");
  vi.stubGlobal("URL", { createObjectURL, revokeObjectURL: vi.fn() });
  const source = "flowchart LR\n  A[Привет] --> B[世界]\n";

  downloadText("diagram.mmd", source, "text/plain");

  expect(postMessage).toHaveBeenCalledWith(
    { type: "foxxycode:downloadText", filename: "diagram.mmd", text: source },
    "*",
  );
  expect(createObjectURL).not.toHaveBeenCalled();
});

test.each(["", "intellij"])("keeps browser downloads for the %s surface", (embed) => {
  vi.useFakeTimers();
  window.history.replaceState(null, "", embed ? `/?embed=${embed}` : "/");
  const names: string[] = [];
  const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (this: HTMLAnchorElement) {
    names.push(this.download);
  });
  const createObjectURL = vi.fn(() => "blob:fixture");
  vi.stubGlobal("URL", { createObjectURL, revokeObjectURL: vi.fn() });

  downloadText("diagram.svg", "<svg/>", "image/svg+xml");

  expect(createObjectURL).toHaveBeenCalledOnce();
  expect(click).toHaveBeenCalledOnce();
  expect(names).toEqual(["diagram.svg"]);
});
