import { readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "vitest";

const css = readFileSync(join(__dirname, "..", "..", "styles.css"), "utf8");

function ruleBody(selector: string): string {
  const idx = css.indexOf(selector);
  expect(idx, `${selector} is missing from styles.css`).toBeGreaterThan(-1);
  const open = css.indexOf("{", idx);
  const close = css.indexOf("}", open);
  return css.slice(open + 1, close);
}

// The dropdown floats over the transcript, so a translucent surface lets the
// chat title and the messages read through it. It shipped using
// --foxxycode-surface-fieldset, which is an overlay tint (rgba(0,0,0,0.2))
// meant for a fieldset drawn on a solid card, not a standalone popup.
test("the export dropdown paints an opaque base under its tint", () => {
  const menu = ruleBody(".session-export-menu {");

  expect(menu).toContain("background-color: var(--bg)");
  expect(menu).not.toContain("var(--foxxycode-surface-fieldset)");
});

test("the export dropdown sits on the composer-menu stacking layer", () => {
  const menu = ruleBody(".session-export-menu {");

  const layer = /z-index:\s*(\d+)/.exec(menu);
  expect(layer, "the dropdown needs an explicit stacking layer").toBeTruthy();
  // 90 is the layer the mode / model / reasoning dropdowns use.
  expect(Number(layer?.[1])).toBeGreaterThanOrEqual(90);
});

// Every theme that supplies the panel tint has to supply the opaque base too; a
// theme defining only the tint would bring the see-through menu back for its
// own users.
test("the opaque base is defined wherever the panel tint is", () => {
  const bgDefs = css.match(/--bg:\s*#/g) ?? [];
  const panelDefs = css.match(/--foxxycode-glass-panel-bg:\s*/g) ?? [];

  expect(bgDefs.length).toBeGreaterThan(1);
  expect(bgDefs.length).toBe(panelDefs.length);
});
