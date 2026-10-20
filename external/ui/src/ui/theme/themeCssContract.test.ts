import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "vitest";
import { UI_THEME_IDS } from "./themeCookie";
import { FOXXYCODE_UI_EFFECTS_COOKIE } from "./uiEffects";

const cssPath = join(
  dirname(fileURLToPath(import.meta.url)),
  "../../styles.css",
);

function cssText(): string {
  return readFileSync(cssPath, "utf8");
}

test("canvas background uses theme variables", () => {
  const css = cssText();
  expect(css).toMatch(/--foxxycode-canvas-gradient-bottom:/);
  expect(css).toMatch(
    /background-color:\s*var\(--foxxycode-canvas-gradient-bottom\)/,
  );
});

test("desktop canvas follows the dynamic viewport in Firefox", () => {
  const css = cssText();
  expect(css).toMatch(/html,\s*body,\s*#root\s*\{[^}]*height:\s*100%/s);
  expect(css).toMatch(/\.shell\s*\{[^}]*height:\s*100dvh/s);
  expect(css).toMatch(/\.rail-column\s*\{[^}]*height:\s*100dvh/s);
});

test("the docked composer paints no vignette over the transcript above it", () => {
  // Upstream switched the shade above the docked composer off (7a36c34c): the
  // last lines of the transcript stay readable right down to the composer.
  // A ::before without `content` is never generated, so any rule that brings
  // the pseudo-element back shows up here.
  const css = cssText().replace(/\/\*[\s\S]*?\*\//g, "");
  const rules = [...css.matchAll(/([^{}]+)\{([^{}]*)\}/g)].filter((m) =>
    /\.chat-bottom(--docked)?[^,{]*::before/.test(m[1]!),
  );
  for (const rule of rules) {
    expect(rule[2], `${rule[1]!.trim()} generates a pseudo-element`).not.toMatch(
      /(^|[;\s])content\s*:/,
    );
  }
});

test("index.html bootstraps theme before paint", () => {
  const html = readFileSync(
    join(dirname(fileURLToPath(import.meta.url)), "../../index.html"),
    "utf8",
  );
  expect(html).toContain("foxxycode_ui_theme");
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

// The IntelliJ panel starts with reduced effects (ui/theme/uiEffects.ts); marking
// <html> before paint keeps the first frame from animating or frosting.
test("index.html marks the effects level before paint, reduced by default in IntelliJ", () => {
  const html = readFileSync(
    join(dirname(fileURLToPath(import.meta.url)), "../../index.html"),
    "utf8",
  );
  expect(html).toContain(FOXXYCODE_UI_EFFECTS_COOKIE);
  expect(html).toContain("dataset.effects");
  expect(html).toMatch(/et === "intellij" \? "reduced" : "full"/);
});

test("index.html VALID theme map stays in sync with UI_THEME_IDS", () => {
  const html = readFileSync(
    join(dirname(fileURLToPath(import.meta.url)), "../../index.html"),
    "utf8",
  );
  const m = html.match(/var VALID = \{([^}]*)\}/);
  expect(m, "index.html must declare the VALID theme map").toBeTruthy();
  const keys = [...m![1]!.matchAll(/"?([\w-]+)"?\s*:/g)].map((k) => k[1]).sort();
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

// The Appearance tab is narrow (and narrower still inside an editor panel). A
// select sized by its longest option overflows the panel and clips the chevron,
// so the width contract is pinned here rather than left to the browser default.
test("language select fills narrow settings without overflow", () => {
  const css = cssText();
  const rule = css.match(/\.appearance-language-select\s*\{[^}]*\}/s);
  expect(rule, ".appearance-language-select rule should exist").toBeTruthy();
  expect(rule![0]).toContain("width: 100%");
  expect(rule![0]).toContain("min-width: 0");
  expect(rule![0]).toContain("min-height: 40px");
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
