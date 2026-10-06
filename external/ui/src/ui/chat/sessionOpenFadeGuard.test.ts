import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "vitest";

// Opening a session schedules a 110ms fade that clears the rows (pickSession),
// so every path that then paints a transcript has to cancel that timer first —
// otherwise the rows it painted are wiped a moment later.
//
// The fix this pins was for the shadow path: returning to a session whose turn
// this client streams restored the rows synchronously, the stale fade wiped
// them, and the chat sat empty until the stream next painted. Behind a
// foreground `spawn_agent` that is minutes of an empty window with a Stop
// button in it (reproduced 2026-09-08 from a running subagent's transcript via
// "Открыть родительский чат").
//
// This is a source check because the paths live inside App.tsx's session-load
// effect, which cannot be mounted in isolation.
const appSource = readFileSync(
  join(dirname(fileURLToPath(import.meta.url)), "..", "App.tsx"),
  "utf8",
);

/** The source between the start of the session-open effect and a given paint. */
function precedingSource(paintCall: string): string {
  const at = appSource.indexOf(paintCall);
  expect(at, `${paintCall} is gone — update this test with its replacement`).toBeGreaterThan(0);
  return appSource.slice(Math.max(0, at - 1200), at);
}

test("restoring a session from the stream shadow cancels the pending fade", () => {
  const before = precedingSource("setItems([...shadowSnap]);");
  expect(before).toContain("clearTimeout(fadeOutTimerRef.current)");
  expect(before).toContain("setSessionFadingOut(false)");
});

test("painting a fetched transcript cancels the pending fade", () => {
  const before = precedingSource("setItems(withBranches);");
  expect(before).toContain("clearTimeout(fadeOutTimerRef.current)");
  expect(before).toContain("setSessionFadingOut(false)");
});
