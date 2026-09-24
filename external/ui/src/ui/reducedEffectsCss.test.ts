import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "vitest";
import { UI_THEME_IDS } from "./theme/themeCookie";

// <html data-effects="reduced"> (ui/theme/uiEffects.ts; the default in the IntelliJ
// panel, switchable in Appearance) turns off what the page would otherwise repaint
// frame after frame: infinite animations and frosted glass.
//
// Why it matters there: JCEF renders the IntelliJ panel off-screen (the default in
// 2022.3-2026.2 IDEs) and copies every frame the page paints into the IDE on its UI
// thread. In PyCharm 2023.3 with the GPU off, the bouncing typing dots kept the IDE's
// UI thread ~5% busy through every turn, 1-2% once they stood still; the dark hero
// title kept it 5.4% busy with the panel merely open, 0.3% without it. The glass
// panels lose their 52px backdrop blur too, and turn opaque in the colour they showed.

const cssPath = join(dirname(fileURLToPath(import.meta.url)), "../styles.css");
const css = readFileSync(cssPath, "utf8").replace(/\/\*[\s\S]*?\*\//g, "");

const REDUCED = '[data-effects="reduced"]';

/** Spinners shown for the second or two a load takes, never for a whole turn. */
const SHORT_LIVED = new Set([
  ".chat-skeleton-spinner",
  ".chat-skeleton-bar",
  ".session-export-spin",
  ".composer-enhance-icon.is-spinning",
  ".settings-icon-spin",
]);

function ruleBlocks(): Array<{ selectors: string[]; body: string }> {
  const out: Array<{ selectors: string[]; body: string }> = [];
  for (const m of css.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    out.push({
      selectors: m[1]!.split(",").map((s) => s.trim().replace(/\s+/g, " ")),
      body: m[2]!,
    });
  }
  return out;
}

/** Keyframes the stylesheet defines; an animation naming anything else never runs. */
function definedKeyframes(): Set<string> {
  return new Set([...css.matchAll(/@keyframes\s+([\w-]+)/g)].map((m) => m[1]!));
}

function isReducedRule(selectors: string[]): boolean {
  return selectors.some((s) => s.includes("[data-effects="));
}

function infiniteAnimationSelectors(): string[] {
  const keyframes = definedKeyframes();
  const out: string[] = [];
  for (const block of ruleBlocks()) {
    if (isReducedRule(block.selectors)) continue;
    const decl = /(?:^|;)\s*animation\s*:([^;]*)/.exec(block.body);
    if (!decl || !/\binfinite\b/.test(decl[1]!)) continue;
    const runs = decl[1]!.split(/\s+/).some((token) => keyframes.has(token));
    if (runs) out.push(...block.selectors);
  }
  return out;
}

/** Selectors that blur their backdrop with a literal filter, not the glass token. */
function literalBlurSelectors(): string[] {
  const out: string[] = [];
  for (const block of ruleBlocks()) {
    if (isReducedRule(block.selectors)) continue;
    if (/(?:^|;)\s*backdrop-filter\s*:\s*blur\(/.test(block.body)) {
      out.push(...block.selectors);
    }
  }
  return out;
}

/**
 * A slow stepped animation changes the picture only a few times a second, so the
 * off-screen panel paints a few frames instead of sixty. Measured idle with the GPU
 * off: a 3s steps(6) colour cycle on the three typing dots kept the IDE's UI thread
 * ~0.2% busy, the same cycle eased 5.5%, the bounce 3.6-5.4%, and the bounce as a
 * 12 fps animated WebP 2.3%.
 */
function isSlowStepped(body: string): boolean {
  const decl = /(?:^|;)\s*animation\s*:([^;]*)/.exec(body);
  if (!decl) return false;
  const value = decl[1]!;
  const steps = /\bsteps\(\s*(\d+)/.exec(value);
  const duration = /(\d+(?:\.\d+)?)(ms|s)\b/.exec(value);
  if (!steps || !duration) return false;
  const seconds = Number(duration[1]) / (duration[2] === "ms" ? 1000 : 1);
  // At most three picture changes a second per element.
  return Number(steps[1]) / seconds <= 3;
}

function reducedRuleSets(selector: string, declaration: RegExp): boolean {
  return ruleBlocks().some(
    (b) => b.selectors.includes(`${REDUCED} ${selector}`) && declaration.test(b.body),
  );
}

test("the scans find what they are about", () => {
  expect(infiniteAnimationSelectors()).toContain(".typing-dots-dot");
  expect(infiniteAnimationSelectors()).toContain(".hero-title-accent");
  expect(literalBlurSelectors()).toContain(".chat-header");
});

test("no infinite animation keeps running smoothly with reduced effects, loading spinners aside", () => {
  const running = infiniteAnimationSelectors().filter(
    (s) =>
      !SHORT_LIVED.has(s) &&
      !reducedRuleSets(s, /(?:^|;)\s*animation\s*:\s*none\b/) &&
      !ruleBlocks().some(
        (b) => b.selectors.includes(`${REDUCED} ${s}`) && isSlowStepped(b.body),
      ),
  );
  expect(running, "infinite animations still running with reduced effects").toEqual([]);
});

// The one motion kept with reduced effects: the typing dots take turns in the
// accent colour, a few stepped changes a second instead of a bounce every frame.
test("with reduced effects the typing dots glow in slow steps instead of bouncing", () => {
  const dots = ruleBlocks().find(
    (b) => b.selectors.includes(`${REDUCED} .typing-dots-dot`) && /animation\s*:/.test(b.body),
  );
  expect(dots, "no reduced animation rule for the typing dots").toBeTruthy();
  expect(isSlowStepped(dots!.body)).toBe(true);
  const name = /animation\s*:\s*([\w-]+)/.exec(dots!.body)![1]!;
  const at = css.indexOf(`@keyframes ${name}`);
  expect(at, `@keyframes ${name} missing`).toBeGreaterThan(-1);
  const rest = css.slice(at);
  const frames = rest.slice(0, rest.search(/\}\s*\}/));
  // Colour only: a transform or opacity would hand the dots to the compositor,
  // which produces a frame every tick however few the steps.
  expect(frames).toMatch(/background-color/);
  expect(frames).not.toMatch(/transform|opacity/);
});

test("every infinite animation kept with reduced effects is slow and stepped", () => {
  for (const b of ruleBlocks()) {
    if (!isReducedRule(b.selectors)) continue;
    const decl = /(?:^|;)\s*animation\s*:([^;]*)/.exec(b.body);
    if (!decl || !/\binfinite\b/.test(decl[1]!)) continue;
    expect(isSlowStepped(b.body), `${b.selectors.join(", ")} animates smoothly`).toBe(true);
  }
});

test("the loading spinners on the allowlist still exist, so the list cannot rot", () => {
  const found = new Set(infiniteAnimationSelectors());
  for (const s of SHORT_LIVED) {
    expect(found.has(s), `${s} is no longer an infinite animation`).toBe(true);
  }
});

test("reduced effects switch the shared glass blur off", () => {
  const root = ruleBlocks().find((b) => b.selectors.includes(`html${REDUCED}`));
  expect(root, `no html${REDUCED} token block`).toBeTruthy();
  expect(root!.body).toMatch(/--foxxycode-glass-panel-backdrop\s*:\s*none/);
});

// The page keeps its gradient and glow; a flattened canvas was tried and rejected.
test("reduced effects keep the canvas gradient and glow", () => {
  const reducedBodies = ruleBlocks()
    .filter((b) => isReducedRule(b.selectors))
    .map((b) => b.body)
    .join("\n");
  for (const token of [
    "--foxxycode-canvas-glow-purple",
    "--foxxycode-canvas-glow-indigo",
    "--foxxycode-canvas-gradient-bottom",
  ]) {
    expect(reducedBodies, `${token} is overridden with reduced effects`).not.toContain(token);
  }
});

type Rgba = [number, number, number, number];

function parseColor(value: string): Rgba | null {
  const v = value.trim();
  const hex = /^#([0-9a-f]{6})$/i.exec(v);
  if (hex) {
    const n = parseInt(hex[1]!, 16);
    return [(n >> 16) & 255, (n >> 8) & 255, n & 255, 1];
  }
  const rgba = /^rgba?\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*(?:,\s*([\d.]+)\s*)?\)$/i.exec(v);
  if (rgba) {
    return [Number(rgba[1]), Number(rgba[2]), Number(rgba[3]), rgba[4] ? Number(rgba[4]) : 1];
  }
  return null;
}

function tokenIn(body: string, token: string): string | null {
  const m = new RegExp(`(?:^|;)\\s*${token}\\s*:\\s*([^;]+)`).exec(body);
  return m ? m[1]!.trim() : null;
}

// Without the blur a translucent tint lets the transcript read straight through the
// sticky header and the top bar (in the dark themes the header tint is only 35-45%
// opaque). So each theme's panels become opaque in the colour they already showed:
// the tint composited over the top of that theme's canvas.
const OPAQUE_TOKENS = ["--foxxycode-glass-panel-bg", "--foxxycode-chat-header-bg", "--nav"];

// The light theme's tint is 90-92% white over a white canvas top, so the composite is
// plain white, which read as harsh. Its panels take the soft grey of the IntelliJ
// composer field instead (color-mix(in srgb, --text 5%, canvas top) in light).
const EXPLICIT: Partial<Record<(typeof UI_THEME_IDS)[number], string>> = { light: "#f3f3f4" };

test("with reduced effects the light theme's panels are the composer's soft grey", () => {
  const reduced = ruleBlocks().find((b) =>
    b.selectors.includes(`html${REDUCED}[data-theme="light"]`),
  );
  expect(reduced, `no html${REDUCED}[data-theme="light"] rule`).toBeTruthy();
  for (const token of OPAQUE_TOKENS) {
    expect(tokenIn(reduced!.body, token)?.toLowerCase(), `light ${token}`).toBe(EXPLICIT.light);
  }
});

test("with reduced effects every theme's panels are opaque in their own composited colour", () => {
  for (const theme of UI_THEME_IDS) {
    if (EXPLICIT[theme]) continue;
    const base = ruleBlocks().find((b) => b.selectors.includes(`[data-theme="${theme}"]`) && /--bg\s*:/.test(b.body));
    expect(base, `no token block for ${theme}`).toBeTruthy();
    const reduced = ruleBlocks().find((b) =>
      b.selectors.includes(`html${REDUCED}[data-theme="${theme}"]`),
    );
    expect(reduced, `no html${REDUCED}[data-theme="${theme}"] rule`).toBeTruthy();
    const canvas = parseColor(tokenIn(base!.body, "--foxxycode-canvas-gradient-top")!)!;
    for (const token of OPAQUE_TOKENS) {
      const tint = parseColor(tokenIn(base!.body, token)!)!;
      const got = parseColor(tokenIn(reduced!.body, token) ?? "");
      expect(got, `${theme} ${token} is not a plain colour`).toBeTruthy();
      expect(got![3], `${theme} ${token} is still translucent`).toBe(1);
      const want = [0, 1, 2].map((i) => Math.round(tint[3] * tint[i]! + (1 - tint[3]) * canvas[i]!));
      for (const i of [0, 1, 2]) {
        expect(Math.abs(got![i]! - want[i]!), `${theme} ${token} drifted from its tint`).toBeLessThanOrEqual(1);
      }
    }
  }
});

test("every literal backdrop blur is switched off with reduced effects", () => {
  const blurred = literalBlurSelectors().filter(
    (s) => !reducedRuleSets(s, /(?:^|;)\s*backdrop-filter\s*:\s*none\b/),
  );
  expect(blurred, "backdrop blurs left on with reduced effects").toEqual([]);
});
