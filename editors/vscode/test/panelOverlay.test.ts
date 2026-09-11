import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, it, expect } from "vitest";

// The error overlay exists twice: this extension injects it into the webview wrapper,
// and the IntelliJ plugin injects the same script into its JCEF panel. Nothing but this
// test keeps the two behaving alike, and they have to — a user reporting "the red bar
// again" is reporting the same code either way.
//
// Comments may differ per host (each explains its own embedding); the executable lines
// may not. The only permitted difference in code is the newline escape: Kotlin's raw
// string takes \n, this file's template literal needs \n.

const REPO = join(__dirname, "..", "..", "..");
const START = "// Show uncaught errors as an overlay";
const END = "window.addEventListener(\"unhandledrejection\"";

function overlayCode(source: string, file: string): string[] {
  const from = source.indexOf(START);
  expect(from, `${file}: overlay block not found`).toBeGreaterThan(-1);
  const to = source.indexOf(END, from);
  expect(to, `${file}: rejection listener not found`).toBeGreaterThan(-1);
  const tail = source.indexOf("});", source.indexOf("show(", to));
  return source
    .slice(from, tail)
    .split("\n")
    .map((line) => line.trim().replace(/\\\\n/g, "\\n"))
    .filter((line) => line !== "" && !line.startsWith("//"));
}

function read(...parts: string[]): string {
  return readFileSync(join(REPO, ...parts), "utf8").replace(/\r\n/g, "\n");
}

describe("the panel error overlay is the same in both editors", () => {
  const vscode = overlayCode(
    read("editors", "vscode", "src", "webview", "panel.ts"),
    "panel.ts",
  );
  const intellij = overlayCode(
    read("editors", "intellij", "src", "main", "kotlin", "dev", "foxxycode",
      "intellij", "ui", "FoxxyCodeBrowserPanel.kt"),
    "FoxxyCodeBrowserPanel.kt",
  );

  it("runs identical code in both hosts", () => {
    expect(vscode).toEqual(intellij);
  });

  // The bug this guards: the benign filter used to sit only on the "error" listener, so
  // one "TypeError: Failed to fetch" from a background poll painted a permanent red bar
  // over the chat. A dropped request to 127.0.0.1 is routine while a turn is running.
  it("keeps a dropped request off the overlay", () => {
    for (const code of [vscode, intellij]) {
      const benign = code.find((line) => line.startsWith("var benign ="));
      expect(benign).toBeDefined();
      expect(benign).toContain("Failed to fetch");
      expect(benign).toContain("AbortError");
      expect(benign).toContain("ResizeObserver loop");
    }
  });

  it("filters the rejection path, not only the error event", () => {
    for (const code of [vscode, intellij]) {
      const guards = code.filter((line) => line.startsWith("if (benign.test("));
      expect(guards.length).toBe(2);
    }
  });

  // Nothing but reloading the panel used to remove the bar, which is what turned a
  // survivable hiccup into a chat the user could not read.
  it("gives the overlay a close button", () => {
    for (const code of [vscode, intellij]) {
      expect(code.join("\n")).toContain("foxxycode-err-overlay-close");
      expect(code.join("\n")).toContain("el.parentNode.removeChild(el)");
    }
  });
});
