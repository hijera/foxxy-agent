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

/** Body of the first rule whose selector list contains exactly `selector`. */
function ruleBody(css: string, selector: string): string | undefined {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const rule = new RegExp(
    `(?:^|,)[ \\t]*${escaped}[ \\t]*(?:,[^{]*)?\\{([^}]*)\\}`,
    "m",
  );
  return css.match(rule)?.[1];
}

function declaration(
  body: string | undefined,
  property: string,
): string | undefined {
  return body?.match(new RegExp(`(?:^|[;\\s])${property}:\\s*([^;]+);`))?.[1];
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

function luminance([r, g, b]: Rgba): number {
  const channel = (c: number) => {
    const s = c / 255;
    return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
  };
  return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b);
}

/** WCAG contrast of `fg` (alpha-composited) over an opaque `ground`. */
function contrast([r, g, b, a]: Rgba, ground: Rgba): number {
  const blend = (c: number, i: number) => c * a + ground[i]! * (1 - a);
  const text = luminance([blend(r, 0), blend(g, 1), blend(b, 2), 1]);
  const [hi, lo] = [text, luminance(ground)].sort((x, y) => y - x);
  return (hi! + 0.05) / (lo! + 0.05);
}

/** The theme's canvas colour (`--bg` in its token block). */
function themeGround(css: string, theme: string): Rgba {
  const blocks = css.matchAll(
    new RegExp(`\\[data-theme="${theme}"\\]\\s*\\{([^}]*)\\}`, "g"),
  );
  for (const [, body] of blocks) {
    const bg = declaration(body, "--bg");
    if (bg) {
      return parseColor(bg);
    }
  }
  throw new Error(`no --bg token for the ${theme} theme`);
}

// Regression: `.settings-btn-danger` only carried the pale pink tuned for a
// dark canvas (`#fecaca`), so on the Light theme an enabled delete button
// (provider and model rows, installed skills, MCP servers) read as disabled.
// Walking every shipped theme keeps a light theme added later from shipping
// without its own override: the colour that applies must stay readable
// (WCAG AA, 4.5:1) on that theme's canvas.
test.each(UI_THEME_IDS)(
  "settings danger button text is readable on the %s theme",
  (theme) => {
    const css = cssText();
    const color =
      declaration(
        ruleBody(css, `[data-theme="${theme}"] .settings-btn-danger`),
        "color",
      ) ?? declaration(ruleBody(css, ".settings-btn-danger"), "color");
    expect(color, ".settings-btn-danger should set a text colour").toBeTruthy();
    expect(
      contrast(parseColor(color!), themeGround(css, theme)),
    ).toBeGreaterThanOrEqual(4.5);
  },
);

// A readable red must not make a disabled button look actionable: the light
// hover tint stays behind `:not(:disabled)`, and disabled buttons keep the
// shared `.settings-btn:disabled` dimming.
test("light danger hover skips disabled buttons, which stay dimmed", () => {
  const css = cssText();
  expect(
    declaration(
      ruleBody(
        css,
        '[data-theme="light"] .settings-btn-danger:hover:not(:disabled)',
      ),
      "border-color",
    ),
  ).toBeTruthy();
  const dimmed = declaration(ruleBody(css, ".settings-btn:disabled"), "opacity");
  expect(Number(dimmed)).toBeLessThan(1);
});
