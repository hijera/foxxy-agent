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

/** Every rule block whose declarations enable content-visibility on transcript rows. */
function containmentBlocks(
  css: string,
): Array<{ selector: string; body: string }> {
  const out: Array<{ selector: string; body: string }> = [];
  const noComments = css.replace(/\/\*[\s\S]*?\*\//g, "");
  const re = /([^{}]+)\{([^{}]*content-visibility:\s*auto[^{}]*)\}/g;
  for (const m of noComments.matchAll(re)) {
    out.push({ selector: m[1]!.trim(), body: m[2]! });
  }
  return out;
}

test("transcript rows never skip their own layout", () => {
  // `content-visibility: auto` sizes a row that has not been rendered yet from
  // `contain-intrinsic-size`, and the last remembered size only exists for rows
  // that were rendered at least once. A session opens scrolled to its end, so
  // every row above it is a guess: measured on a 360-row transcript, the
  // scrollport reported 42114px against a real 50342px. Scrolling up then
  // materialises those rows one screen at a time, the height grows under the
  // reader, and scroll anchoring pushes the position down by the difference on
  // every wheel notch - which is the flicker the transcript used to show.
  expect(containmentBlocks(cssText())).toEqual([]);
});

test("the user-row edit button still hangs outside the bubble", () => {
  // Documents the constraint the containment rule has to respect.
  const rule = cssText().match(/\.msg-user-edit\s*\{[^}]*\}/s);
  expect(rule?.[0]).toMatch(/position:\s*absolute/);
  expect(rule?.[0]).toMatch(/left:\s*-\d+px/);
});

// JCEF (IntelliJ, Chromium 104) could not run content-visibility: auto rows
// without raising "ResizeObserver loop limit exceeded" on every transcript open,
// which the panel painted as an error overlay, so the IDE embed used to opt out
// of the optimization row by row. The optimization is gone for everybody now,
// and the opt-out went with it: there is nothing left to switch off, and a rule
// that still tried would be a selector nobody could explain.
test("nothing opts out of a containment that no longer exists", () => {
  const noComments = cssText().replace(/\/\*[\s\S]*?\*\//g, "");
  expect(noComments).not.toMatch(/content-visibility\s*:/);
  expect(noComments).not.toMatch(/contain-intrinsic-size\s*:/);
});
