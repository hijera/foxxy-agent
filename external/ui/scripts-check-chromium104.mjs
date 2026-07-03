/**
 * scripts-check-chromium104.mjs — post-build regression guard.
 *
 * The UI is embedded in an IntelliJ/PhpStorm 2022.3.3 plugin via JCEF, whose
 * Chromium is version 104 (see docs/intellij-embedding.md). This script scans
 * the built bundle (dist/styles.css + dist/app.js — including dependency
 * code) for CSS/JS features newer than Chromium 104 and fails the build when
 * any are found. Wired into `npm run build:go` between `vite build` and the
 * go:embed sync.
 *
 * Allowlist (progressive enhancements that degrade gracefully on 104):
 *   - `text-wrap: balance` (hero title; falls back to normal wrapping)
 */

import { readFile } from "node:fs/promises";
import path from "node:path";
import postcss from "postcss";

const dist = path.join(path.resolve(import.meta.dirname), "dist");
const problems = [];

// ── CSS ────────────────────────────────────────────────────────────────────
const cssPath = path.join(dist, "styles.css");
const cssText = await readFile(cssPath, "utf8");
const root = postcss.parse(cssText, { from: cssPath });

const cssValueBans = [
  { re: /color-mix\(/i, what: "color-mix() (Chromium 111+)" },
  { re: /\boklch\(/i, what: "oklch() (Chromium 111+)" },
  { re: /\boklab\(/i, what: "oklab() (Chromium 111+)" },
  { re: /\d(\.\d+)?lh\b/, what: "lh unit (Chromium 109+)" },
];
const VIEWPORT_UNIT_RE = /\d(?:\.\d+)?(?:d|s|l)vh\b/;

const where = (node) =>
  node.source?.start ? `styles.css:${node.source.start.line}` : "styles.css";

root.walkAtRules((at) => {
  if (at.name === "container") {
    problems.push(`${where(at)}: @container (Chromium 105+)`);
  }
});

root.walkRules((rule) => {
  if (rule.selector.includes(":has(")) {
    problems.push(
      `${where(rule)}: :has() selector (Chromium 105+) — ${rule.selector.replace(/\s+/g, " ").slice(0, 80)}`,
    );
  }
});

root.walkDecls((decl) => {
  if (decl.prop === "container-type" || decl.prop === "container") {
    problems.push(`${where(decl)}: ${decl.prop} (Chromium 105+)`);
    return;
  }
  if (decl.prop === "text-wrap" && decl.value.includes("balance")) {
    return; // allowlisted, degrades gracefully
  }
  for (const ban of cssValueBans) {
    if (ban.re.test(decl.value)) {
      problems.push(
        `${where(decl)}: ${ban.what} in "${decl.prop}: ${decl.value.replace(/\s+/g, " ").slice(0, 80)}"`,
      );
    }
  }
  if (VIEWPORT_UNIT_RE.test(decl.value)) {
    // dvh/svh/lvh (Chromium 108+) need a same-property fallback declaration
    // without those units directly before them in the same rule.
    const prev = decl.prev();
    const ok =
      prev &&
      prev.type === "decl" &&
      prev.prop === decl.prop &&
      !VIEWPORT_UNIT_RE.test(prev.value);
    if (!ok) {
      problems.push(
        `${where(decl)}: ${decl.prop} uses dvh/svh/lvh (Chromium 108+) without a preceding ${decl.prop} fallback`,
      );
    }
  }
});

// ── JS ─────────────────────────────────────────────────────────────────────
const jsPath = path.join(dist, "app.js");
const jsText = await readFile(jsPath, "utf8");

const jsBans = [
  [".toSorted(", "Array.prototype.toSorted (Chromium 110+)"],
  [".toReversed(", "Array.prototype.toReversed (Chromium 110+)"],
  ["Object.groupBy", "Object.groupBy (Chromium 117+)"],
  ["Promise.withResolvers", "Promise.withResolvers (Chromium 119+)"],
  [".withResolvers", "withResolvers (Chromium 119+)"],
  ["URL.canParse", "URL.canParse (Chromium 120+)"],
  ["AbortSignal.any(", "AbortSignal.any (Chromium 116+)"],
  ["Array.fromAsync", "Array.fromAsync (Chromium 121+)"],
  ["new URLPattern", "URLPattern (Chromium 95+, unstable API surface)"],
  ["showOpenFilePicker", "showOpenFilePicker (secure-context FS Access API)"],
];

for (const [token, what] of jsBans) {
  const idx = jsText.indexOf(token);
  if (idx !== -1) {
    const ctx = jsText
      .slice(Math.max(0, idx - 40), idx + token.length + 40)
      .replace(/\s+/g, " ");
    problems.push(`app.js @${idx}: ${what} — …${ctx}…`);
  }
}

// ── Verdict ────────────────────────────────────────────────────────────────
if (problems.length > 0) {
  console.error(
    `\n[check-chromium104] FAILED — ${problems.length} finding(s) newer than the Chromium 104 JCEF baseline:\n`,
  );
  for (const p of problems) console.error(`  - ${p}`);
  console.error(
    "\nSee docs/intellij-embedding.md and .claude/rules/ui-spa.md for the compatibility rules.\n",
  );
  process.exit(1);
}

console.log("[check-chromium104] OK — dist output is Chromium 104 compatible");
