import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "vitest";

const here = dirname(fileURLToPath(import.meta.url));
const css = readFileSync(join(here, "../../styles.css"), "utf8").replace(/\r\n/g, "\n");
const sidebar = readFileSync(join(here, "SessionsSidebar.tsx"), "utf8").replace(/\r\n/g, "\n");

/** Declarations of every rule whose selector is exactly `selector`, merged in source order. */
function declarations(selector: string): Record<string, string> {
  const out: Record<string, string> = {};
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const rule = new RegExp(`(?:^|\\n)${escaped}\\s*\\{([^}]*)\\}`, "g");
  for (const m of css.matchAll(rule)) {
    for (const decl of (m[1] ?? "").split(";")) {
      const i = decl.indexOf(":");
      if (i > 0) out[decl.slice(0, i).trim()] = decl.slice(i + 1).trim();
    }
  }
  return out;
}

/** The session-row-* blocks rendered inside the row link, in markup order. */
function linkChildren(): string[] {
  const start = sidebar.indexOf('className="session-row-link"');
  const end = sidebar.indexOf("</a>", start);
  expect(start).toBeGreaterThan(0);
  const body = sidebar.slice(start + 1, end);
  return [...body.matchAll(/className="(session-row-[a-z]+)"/g)].flatMap((m) => (m[1] ? [m[1]] : []));
}

// The row link is a grid (state marks, then the title with what sits under it).
// A child without an explicit cell is auto-placed into the first empty one: the
// fork's folder line landed in the marks column, beside the title, on every row
// without a mark ("projTitle"), and under the mark on a row with one.
test("every block of a History row has its own grid cell", () => {
  expect(declarations(".session-row-link").display).toBe("grid");
  const children = linkChildren();
  expect(children).toEqual(
    expect.arrayContaining(["session-row-marks", "session-row-leading", "session-row-cwd", "session-row-tags"]),
  );
  const cells = new Map<string, string>();
  for (const cls of children) {
    const d = declarations(`.${cls}`);
    expect(d["grid-column"], `.${cls} has no grid-column`).toBeTruthy();
    expect(d["grid-row"], `.${cls} has no grid-row`).toBeTruthy();
    const cell = `${d["grid-column"]}/${d["grid-row"]}`;
    expect(cells.get(cell), `.${cls} shares its cell with ${cells.get(cell)}`).toBeUndefined();
    cells.set(cell, cls);
  }
});

test("the folder line sits under the title and above the tags, on the title's edge", () => {
  const title = declarations(".session-row-leading");
  const cwd = declarations(".session-row-cwd");
  const tags = declarations(".session-row-tags");
  expect(cwd["grid-column"]).toBe(title["grid-column"]);
  expect(tags["grid-column"]).toBe(title["grid-column"]);
  expect(Number(cwd["grid-row"])).toBe(Number(title["grid-row"]) + 1);
  expect(Number(tags["grid-row"])).toBe(Number(cwd["grid-row"]) + 1);
});
