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

// Regression: the code-block "Copy" button (.md-copy) hardcoded a dark
// background rgba(20, 20, 22, ...) with no light-theme override, so it
// rendered as a dark blob on the light theme. Background must be theme-aware.
test("code-block copy button background is theme-aware (not a dark blob on light)", () => {
  const css = cssText();
  const base = /^\.md-copy\s*\{[^}]+\}/m.exec(css);
  const hover = /^\.md-copy:hover\s*\{[^}]+\}/m.exec(css);
  expect(base).not.toBeNull();
  expect(hover).not.toBeNull();
  // No hardcoded near-black background on either state.
  expect(base![0]).not.toMatch(/background:\s*rgba\(\s*20,\s*20,\s*22/);
  expect(hover![0]).not.toMatch(/background:\s*rgba\(\s*20,\s*20,\s*22/);
  // Derives from the shared glass-panel token so it flips with the theme.
  expect(base![0]).toMatch(/background:[^;]*var\(--coddy-glass-panel-bg\)/);
});
