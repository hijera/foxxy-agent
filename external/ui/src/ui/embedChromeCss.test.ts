import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "vitest";

// Both editor hosts share the flat "native input field" chrome, but only the
// JCEF panel (Chromium 104) needs the transcript-row containment opt-out, so
// the two families of `[data-embed=...]` rules must not drift apart: a new
// flat-chrome rule keyed on one id would leave the other host with the glass
// card, and a Chromium-104 workaround keyed on both would slow the modern
// Electron webview down for nothing.

const cssPath = join(dirname(fileURLToPath(import.meta.url)), "../styles.css");

function ruleBlocks(): Array<{ selector: string; body: string }> {
  const noComments = readFileSync(cssPath, "utf8").replace(/\/\*[\s\S]*?\*\//g, "");
  const out: Array<{ selector: string; body: string }> = [];
  for (const m of noComments.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    out.push({ selector: m[1]!.trim(), body: m[2]! });
  }
  return out;
}

function selectorsOf(block: { selector: string }): string[] {
  return block.selector.split(",").map((s) => s.trim());
}

const FLAT_CHROME_TARGETS = [
  ".composer-card",
  ".composer-wrap-docked .composer-card",
  ".hero-composer .composer-card",
  ".chat-bottom--docked::before",
  ".hero",
  ".hero-title",
];

test("the flat IDE chrome applies to both the intellij and the vscode embed", () => {
  const blocks = ruleBlocks().filter((b) => b.selector.includes("[data-embed="));
  for (const target of FLAT_CHROME_TARGETS) {
    const block = blocks.find((b) =>
      selectorsOf(b).includes(`[data-embed="intellij"] ${target}`),
    );
    expect(block, `no [data-embed] rule for ${target}`).toBeTruthy();
    expect(selectorsOf(block!)).toContain(`[data-embed="vscode"] ${target}`);
  }
});

test("every [data-embed] rule names either both hosts or only the JCEF workaround", () => {
  for (const block of ruleBlocks().filter((b) => b.selector.includes("[data-embed="))) {
    const sels = selectorsOf(block);
    const intellij = sels.filter((s) => s.includes('[data-embed="intellij"]'));
    const vscode = sels.filter((s) => s.includes('[data-embed="vscode"]'));
    if (/content-visibility/.test(block.body)) {
      // Chromium-104 containment opt-out: intellij only.
      expect(vscode, `containment opt-out must stay JCEF-only: ${block.selector}`).toEqual([]);
      continue;
    }
    expect(vscode.length, `vscode missing from ${block.selector}`).toBe(intellij.length);
  }
});
