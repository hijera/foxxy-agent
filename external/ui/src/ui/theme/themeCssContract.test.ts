import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "vitest";
import { UI_THEME_IDS } from "./themeCookie";

const cssPath = join(
  dirname(fileURLToPath(import.meta.url)),
  "../../styles.css",
);

function cssText(): string {
  return readFileSync(cssPath, "utf8");
}

test("canvas background uses theme variables", () => {
  const css = cssText();
  expect(css).toMatch(/--coddy-canvas-gradient-bottom:/);
  expect(css).toMatch(
    /background-color:\s*var\(--coddy-canvas-gradient-bottom\)/,
  );
});

test("index.html bootstraps theme before paint", () => {
  const html = readFileSync(
    join(dirname(fileURLToPath(import.meta.url)), "../../index.html"),
    "utf8",
  );
  expect(html).toContain("coddy_ui_theme");
  expect(html).toContain("dataset.theme");
});

test("index.html honors the ?theme= query param for IDE embeddings", () => {
  const html = readFileSync(
    join(dirname(fileURLToPath(import.meta.url)), "../../index.html"),
    "utf8",
  );
  expect(html).toMatch(/theme=\(\[\^&\]\+\)/); // location.search parsing
  expect(html).toContain("Max-Age=31536000"); // persisted to the cookie
});

test("index.html VALID theme map stays in sync with UI_THEME_IDS", () => {
  const html = readFileSync(
    join(dirname(fileURLToPath(import.meta.url)), "../../index.html"),
    "utf8",
  );
  const m = html.match(/var VALID = \{([^}]*)\}/);
  expect(m, "index.html must declare the VALID theme map").toBeTruthy();
  const keys = [...m![1].matchAll(/"?([\w-]+)"?\s*:/g)].map((k) => k[1]).sort();
  expect(keys).toEqual([...UI_THEME_IDS].sort());
});

test("styles.css defines variable blocks for all 7 themes", () => {
  const css = cssText();
  const themeSelectors = [
    '[data-theme="dark"]',
    '[data-theme="light"]',
    '[data-theme="midnight"]',
    '[data-theme="solarized-dark"]',
    '[data-theme="monokai"]',
    '[data-theme="nord"]',
    '[data-theme="rose-pine"]',
  ];
  for (const sel of themeSelectors) {
    expect(css).toContain(sel);
  }
});

test("each theme block defines --accent", () => {
  const css = cssText();
  const themes = [
    "dark",
    "light",
    "midnight",
    "solarized-dark",
    "monokai",
    "nord",
    "rose-pine",
  ];
  for (const t of themes) {
    const block = new RegExp(
      `\\[data-theme="${t}"\\][^{]*\\{[^}]*--accent:[^}]*\\}`,
      "s",
    );
    expect(css, `${t} should have --accent`).toMatch(block);
  }
});
