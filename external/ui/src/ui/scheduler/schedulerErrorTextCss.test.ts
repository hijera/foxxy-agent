import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "vitest";
import { UI_THEME_IDS } from "../theme/themeCookie";

const cssPath = join(
  dirname(fileURLToPath(import.meta.url)),
  "../../styles.css",
);

function cssText(): string {
  return readFileSync(cssPath, "utf8");
}

type Rgba = [number, number, number, number];

/**
 * Bodies of every rule whose selector list contains exactly `selector`, in
 * source order, so a later rule (a grouped override block) is not missed.
 */
function ruleBodies(css: string, selector: string): string {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const rule = new RegExp(
    `(?:^|,)[ \\t]*${escaped}[ \\t]*(?:,[^{]*)?\\{([^}]*)\\}`,
    "gm",
  );
  return [...css.matchAll(rule)].map((m) => m[1]).join(";\n");
}

/** The last value `body` gives `property` - the one the cascade keeps. */
function declaration(body: string, property: string): string | undefined {
  const values = body.matchAll(
    new RegExp(`(?:^|[;\\s])${property}:\\s*([^;]+);`, "g"),
  );
  return [...values].pop()?.[1];
}

/**
 * The value `property` takes on `.cls` under `theme`: the theme's own
 * `[data-theme="…"] .cls` override when it sets one, else the base rule.
 */
function themedDeclaration(
  css: string,
  theme: string,
  cls: string,
  property: string,
): string | undefined {
  return (
    declaration(ruleBodies(css, `[data-theme="${theme}"] .${cls}`), property) ??
    declaration(ruleBodies(css, `.${cls}`), property)
  );
}

function parseColor(value: string): Rgba {
  const v = value.trim().toLowerCase();
  const hex = v.match(/^#([0-9a-f]{3}|[0-9a-f]{6})$/)?.[1];
  if (hex) {
    const full = hex.length === 3 ? hex.replace(/./g, "$&$&") : hex;
    return [0, 2, 4]
      .map((i) => parseInt(full.slice(i, i + 2), 16))
      .concat(1) as Rgba;
  }
  const args = v.match(/^rgba?\(([^)]*)\)$/)?.[1];
  if (args) {
    const [r, g, b, a = 1] = args.split(",").map(Number);
    return [r!, g!, b!, a];
  }
  throw new Error(`unsupported colour: ${value}`);
}

/** `top` alpha-composited over an opaque `ground`. */
function over([r, g, b, a]: Rgba, ground: Rgba): Rgba {
  const blend = (c: number, i: number) => c * a + ground[i]! * (1 - a);
  return [blend(r, 0), blend(g, 1), blend(b, 2), 1];
}

function luminance([r, g, b]: Rgba): number {
  const channel = (c: number) => {
    const s = c / 255;
    return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
  };
  return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b);
}

/** WCAG contrast of `fg` (alpha-composited) over an opaque `ground`. */
function contrast(fg: Rgba, ground: Rgba): number {
  const [hi, lo] = [luminance(over(fg, ground)), luminance(ground)].sort(
    (x, y) => y - x,
  );
  return (hi! + 0.05) / (lo! + 0.05);
}

/** The theme's canvas colour (`--bg` in its token block). */
function themeGround(css: string, theme: string): Rgba {
  const blocks = css.matchAll(
    new RegExp(`\\[data-theme="${theme}"\\]\\s*\\{([^}]*)\\}`, "g"),
  );
  for (const [, body] of blocks) {
    const bg = declaration(body!, "--bg");
    if (bg) {
      return parseColor(bg);
    }
  }
  throw new Error(`no --bg token for the ${theme} theme`);
}

// The job editor's error text - a field's validation message (job id,
// description, schedule, body), the cron hint once the expression does not
// parse, and the failed-save message - only carried the pale red tuned for a
// dark canvas (`rgba(252, 165, 165, 0.95)`), which is about 1.7:1 on the Light
// theme. Walking every shipped theme keeps a light theme added later from
// shipping without its own override: the colour that applies must stay
// readable (WCAG AA, 4.5:1) on that theme's canvas, and on the tint the cron
// hint paints behind its text.
const ERROR_TEXT_CLASSES = [
  "scheduler-field-err",
  "scheduler-cron-hint-err",
  "scheduler-save-err",
];

test.each(
  UI_THEME_IDS.flatMap((theme) =>
    ERROR_TEXT_CLASSES.map((cls) => [cls, theme] as const),
  ),
)(".%s text is readable on the %s theme", (cls, theme) => {
  const css = cssText();
  const color = themedDeclaration(css, theme, cls, "color");
  expect(color, `.${cls} should set a text colour`).toBeTruthy();
  const canvas = themeGround(css, theme);
  const tint = themedDeclaration(css, theme, cls, "background");
  const ground = tint ? over(parseColor(tint), canvas) : canvas;
  expect(contrast(parseColor(color!), ground)).toBeGreaterThanOrEqual(4.5);
});
