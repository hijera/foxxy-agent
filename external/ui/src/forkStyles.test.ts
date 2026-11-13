import {
  mkdirSync,
  mkdtempSync,
  readFileSync,
  readdirSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { afterEach, describe, expect, test } from "vitest";
import postcss from "postcss";
// @ts-ignore -- shared build plugin is plain JavaScript.
import forkStyles, { readStyleSource } from "../postcss-fork-styles.mjs";
// @ts-ignore -- shared Chromium 104 transform is plain JavaScript.
import resolveColorMix from "../postcss-resolve-color-mix.mjs";

const directories: string[] = [];
function directory() {
  const path = mkdtempSync(join(tmpdir(), "foxxy-fork-css-"));
  directories.push(path);
  return path;
}
afterEach(() =>
  directories
    .splice(0)
    .forEach((path) => rmSync(path, { recursive: true, force: true })),
);

test("feature styles preserve their cascade position and media conditions", async () => {
  const dir = directory();
  writeFileSync(
    join(dir, "svn.css"),
    ".card { color: blue; } @media (max-width: 599px) { .card { color: red; } }",
  );
  const result = await postcss([forkStyles({ directory: dir })]).process(
    ".card { color: black; } /* foxxycode:styles svn */ .card { color: green; }",
    { from: "styles.css" },
  );
  const colors: string[] = [];
  result.root.walkDecls("color", (declaration) => {
    colors.push(declaration.value);
  });
  expect(colors).toEqual(["black", "blue", "red", "green"]);
  expect(result.css).toContain("@media (max-width: 599px)");
  expect(result.css).not.toContain("foxxycode:styles");
  expect(result.messages).toContainEqual(
    expect.objectContaining({ type: "dependency", file: join(dir, "svn.css") }),
  );
});

test("feature color mixes see all theme variables before the Chromium 104 transform", async () => {
  const dir = directory();
  writeFileSync(
    join(dir, "ide-embed.css"),
    ".composer-card { color: color-mix(in srgb, var(--text) 50%, transparent); }",
  );
  const result = await postcss([
    forkStyles({ directory: dir }),
    resolveColorMix(),
  ]).process(
    ':root, [data-theme="dark"] { --text: #ffffff; } [data-theme="light"] { --text: #000000; } /* foxxycode:styles ide-embed */',
    { from: "styles.css" },
  );
  expect(result.css).not.toContain("color-mix(");
  expect(result.css).toMatch(/--cmix-[a-z0-9]+: rgba\(255, 255, 255, 0.5\)/);
  expect(result.css).toMatch(/--cmix-[a-z0-9]+: rgba\(0, 0, 0, 0.5\)/);
});

test("unknown feature styles fail the build instead of losing styling", async () => {
  const dir = directory();
  await expect(
    postcss([forkStyles({ directory: dir })]).process(
      "/* foxxycode:styles typo */",
      { from: "styles.css" },
    ),
  ).rejects.toThrow(/Unknown fork stylesheet/);
  // A name that looks right but has no file in the directory is just as lost.
  await expect(
    postcss([forkStyles({ directory: dir })]).process(
      "/* foxxycode:styles svn */",
      { from: "styles.css" },
    ),
  ).rejects.toThrow(/Unknown fork stylesheet/);
});

test("a feature file the directory lists but that cannot be read fails the build", async () => {
  const dir = directory();
  mkdirSync(join(dir, "svn.css"));
  await expect(
    postcss([forkStyles({ directory: dir })]).process(
      "/* foxxycode:styles svn */",
      { from: "styles.css" },
    ),
  ).rejects.toThrow(/Cannot load fork stylesheet/);
});

test("nested includes and includes inside media rules are rejected", async () => {
  const dir = directory();
  writeFileSync(join(dir, "svn.css"), "/* foxxycode:styles svn */");
  await expect(
    postcss([forkStyles({ directory: dir })]).process(
      "/* foxxycode:styles svn */",
      { from: "styles.css" },
    ),
  ).rejects.toThrow(/cannot include other feature styles/);
  await expect(
    postcss([forkStyles({ directory: dir })]).process(
      "@media print { /* foxxycode:styles svn */ }",
      { from: "styles.css" },
    ),
  ).rejects.toThrow(/top level/);
});

test("feature styles cannot pull in runtime imports", async () => {
  const dir = directory();
  writeFileSync(
    join(dir, "svn.css"),
    '@import "other.css";\n.x { color: red; }',
  );
  writeFileSync(join(dir, "other.css"), ".y { color: blue; }");
  await expect(
    postcss([forkStyles({ directory: dir })]).process(
      "/* foxxycode:styles svn */",
      { from: "styles.css" },
    ),
  ).rejects.toThrow(/@import/);
});

test("a comment that only mentions the mechanism stays documentation", async () => {
  const dir = directory();
  const note = "/* foxxycode:styles live in src/styles/fork/ */";
  const result = await postcss([forkStyles({ directory: dir })]).process(
    `${note}\n.card { color: black; }`,
    { from: "styles.css" },
  );
  expect(result.css).toContain(note);
  expect(result.css).toContain("color: black");
});

describe("the real stylesheet composition", () => {
  const here = dirname(fileURLToPath(import.meta.url));
  const forkDirectory = join(here, "styles", "fork");

  /** Every feature file the directory holds, by feature name. */
  const featureFiles = readdirSync(forkDirectory)
    .filter((entry) => entry.endsWith(".css"))
    .map((entry) => entry.slice(0, -".css".length))
    .sort();

  /** Marker names styles.css spells out, in source order. */
  const markers = [
    ...readFileSync(join(here, "styles.css"), "utf8").matchAll(
      /\/\*\s*foxxycode:styles\s+(\S+)\s*\*\//g,
    ),
  ].map((match) => match[1]!.toLowerCase());

  /**
   * One rule only each feature owns. A feature file without an entry here would
   * drop out of the pin below the moment it stops being pulled in.
   */
  const signatures: Record<string, string> = {
    "ide-embed": '[data-embed="intellij"]',
    onboarding: ".tour-overlay",
    "provider-transfer": ".provider-import",
    svn: ".svn-status-row",
  };

  test("every feature file has its marker in styles.css, and every marker its file", () => {
    expect([...markers].sort()).toEqual(featureFiles);
  });

  test("every feature contributes the rule its surface renders with", () => {
    expect(Object.keys(signatures).sort()).toEqual(featureFiles);
    const composed = readStyleSource();
    for (const [name, selector] of Object.entries(signatures)) {
      expect(composed, `${name} styles are missing`).toContain(selector);
    }
    expect(composed).not.toMatch(/\/\*\s*foxxycode:styles\s+\S+\s*\*\//);
  });
});
