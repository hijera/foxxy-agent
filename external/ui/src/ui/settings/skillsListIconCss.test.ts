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

// A long skill name must not squeeze the leading enable switch.
test("skills list leading switch does not shrink in the flex row", () => {
  const css = cssText();
  expect(css).toMatch(
    /\.skill-switch\s*\{[^}]*flex:\s*0 0 auto/s,
  );
});

// A fieldset is as wide as its min-content by default, and the marketplaces
// box holds an add form and long addresses: without this rule it came out
// 17px wider than its siblings at 390px and the drawer scrolled sideways.
test("the marketplaces box never outgrows the settings panel", () => {
  const body =
    /\.skills-marketplaces-box\s*\{([^}]*)\}/.exec(cssText())?.[1] ?? "";
  expect(body).toMatch(/min-inline-size:\s*0/);
  expect(body).toMatch(/max-width:\s*100%/);
});
