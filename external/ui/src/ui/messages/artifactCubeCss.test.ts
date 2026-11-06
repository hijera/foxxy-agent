import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "vitest";

const css = readFileSync(
  join(dirname(fileURLToPath(import.meta.url)), "../../styles.css"),
  "utf8",
);

function block(selector: string): string {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  return new RegExp(`${escaped}\\s*\\{([^}]*)\\}`).exec(css)?.[1] ?? "";
}

test("inline artifact cards are compact cubes with low clamped file names", () => {
  expect(block(".tool-artifact-card")).toMatch(/width:\s*168px/);
  expect(block(".tool-artifact-card")).toMatch(/height:\s*176px/);
  expect(block(".tool-artifact-name")).toMatch(/align-self:\s*end/);
  expect(block(".tool-artifact-name")).toMatch(/-webkit-line-clamp:\s*2/);
  expect(css).toMatch(/\.tool-artifact-card:hover\s*\{/);
});

test("phone artifact cards retain an accessible action trigger", () => {
  expect(css).toMatch(/@media \(max-width: 599px\)[\s\S]*\.tool-artifact-card\s*\{[^}]*width:\s*min\(148px, calc\(50vw - 28px\)\)/);
  expect(css).toMatch(/@media \(max-width: 599px\)[\s\S]*\.inline-artifact-menu-trigger\s*\{[^}]*opacity:\s*1/);
});

test("inline artifact groups are block rows with breathing room after files", () => {
  expect(block(".inline-artifacts")).toMatch(/display:\s*flex/);
  expect(block(".inline-artifacts")).toMatch(/margin:\s*12px 0 16px/);
  expect(block(".inline-artifacts")).toMatch(/max-width:\s*100%/);
});

test("image artifacts use a square extension badge and centered cover preview", () => {
  const badge = block(".inline-artifact-extension");
  expect(badge).toMatch(/display:\s*grid/);
  expect(badge).toMatch(/place-items:\s*center/);
  expect(badge).toMatch(/width:\s*28px/);
  expect(badge).toMatch(/height:\s*28px/);
  expect(badge).toMatch(/border-radius:\s*6px/);
  expect(badge).toMatch(/pointer-events:\s*none/);
  expect(block(".inline-artifact-thumb")).toMatch(/object-fit:\s*cover/);
  expect(block(".inline-artifact-thumb")).toMatch(/object-position:\s*center/);
});
