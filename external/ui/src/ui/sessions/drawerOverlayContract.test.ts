/**
 * Contract: History/sessions drawer must be a pure overlay — it must never push
 * the chat canvas aside. Scheduler and Settings are also fixed overlays and the
 * three drawers must be mutually exclusive (no side-by-side dual-drawer layout).
 */
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

test("shell-history-open does not push chat canvas via padding-left", () => {
  const css = cssText();
  // The selector that previously added padding-left to shift the chat must not exist.
  expect(css).not.toMatch(
    /shell-history-open\s*(?:>|[^\{]*)\s*\.main\s*\{[^}]*padding-left/s,
  );
});

test("no dual-drawer beside-scheduler CSS positioning for History", () => {
  const css = cssText();
  expect(css).not.toContain("shell-history-beside-scheduler");
  expect(css).not.toContain("sessions-drawer-beside-scheduler");
});
