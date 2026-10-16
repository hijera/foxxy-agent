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
