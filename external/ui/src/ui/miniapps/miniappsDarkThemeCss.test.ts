import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "vitest";

const cssPath = join(
  dirname(fileURLToPath(import.meta.url)),
  "../../styles.css",
);

function cssText(): string {
  return readFileSync(cssPath, "utf8");
}

function themeBlock(css: string, theme: string): string {
  const selector =
    theme === "dark"
      ? /:root,\s*\[data-theme="dark"\]\s*\{([\s\S]*?)\n\}/
      : new RegExp(
          `\\[data-theme="${theme}"\\]\\s*\\{([\\s\\S]*?)\\n\\}`,
        );
  const match = css.match(selector);
  expect(match, `${theme} theme block`).toBeTruthy();
  return match?.[1] ?? "";
}

test("Mini Apps code textareas keep readable dark surfaces", () => {
  const css = cssText();
  for (const theme of ["dark", "midnight"]) {
    const block = themeBlock(css, theme);
    expect(block).toMatch(/--foxxycode-surface-code-bg:\s*rgba\(\d+,\s*\d+,\s*\d+,/);
    expect(block).not.toMatch(/--foxxycode-surface-code-bg:\s*rgba\(229/);
    expect(block).toMatch(/--foxxycode-surface-code-fg:/);
    expect(block).not.toMatch(/--foxxycode-surface-code-fg:\s*rgba\(186/);
  }

  expect(css).toMatch(
    /\.miniapps-codearea,[\s\S]*?\.miniapps-report\s*\{[\s\S]*?background:\s*var\(--foxxycode-surface-code-bg\)[\s\S]*?color:\s*var\(--foxxycode-surface-code-fg\)/,
  );
});
