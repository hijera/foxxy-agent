// A `void fetch(...)` with no `.catch` is how the red "FoxxyCode UI error —
// TypeError: Failed to fetch" bar got in front of the chat: the panel polls
// 127.0.0.1 several times a second during a turn, and one dropped keep-alive
// rejected into the plugin bootstrap's unhandledrejection listener.
//
// `fetchJSON` no longer throws, so the helper covers its own call sites. Raw
// fire-and-forget calls have to opt in by hand, and they are easy to add without
// noticing -- a fourth one appeared on main while this guard's fix was in review.
// So assert it in the suite instead of relying on someone spotting it.

import { readFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const SRC = join(__dirname, "..", "..");

function sourceFiles(dir: string): string[] {
  const out: string[] = [];
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) {
      out.push(...sourceFiles(full));
      continue;
    }
    if (!/\.tsx?$/.test(entry) || /\.test\.tsx?$/.test(entry)) continue;
    out.push(full);
  }
  return out;
}

/** Lines starting a `void fetch(` whose statement carries no `.catch`. */
function unguardedFloatingFetches(text: string): number[] {
  const lines = text.split(/\r?\n/);
  const bad: number[] = [];
  lines.forEach((line, i) => {
    if (!/\bvoid\s+fetch\s*\(/.test(line)) return;
    // The call spans a few lines; the statement ends at the first line closing it
    // back at or below the opening indentation.
    const window = lines.slice(i, i + 15).join("\n");
    const stmt = window.split(/;\s*(?:\r?\n|$)/)[0] ?? window;
    if (!/\.catch\s*\(/.test(stmt)) bad.push(i + 1);
  });
  return bad;
}

describe("floating fetch calls", () => {
  it("every `void fetch(...)` in the SPA handles its own rejection", () => {
    const offenders: string[] = [];
    for (const file of sourceFiles(SRC)) {
      const text = readFileSync(file, "utf8");
      for (const line of unguardedFloatingFetches(text)) {
        offenders.push(`${file.slice(SRC.length + 1).replace(/\\/g, "/")}:${line}`);
      }
    }
    expect(
      offenders,
      "add `.catch(() => {})` — a fire-and-forget fetch that rejects paints the " +
        "plugin's error overlay over the chat",
    ).toEqual([]);
  });

  it("recognises an unguarded call", () => {
    const guarded = `      void fetch("/x", { method: "PATCH" }).catch(() => {\n        // ignore\n      });\n`;
    const bare = `      void fetch("/x", {\n        method: "PATCH",\n      });\n`;
    expect(unguardedFloatingFetches(guarded)).toEqual([]);
    expect(unguardedFloatingFetches(bare)).toEqual([1]);
  });
});
