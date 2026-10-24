import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, expect, test } from "vitest";
import postcss from "postcss";
// @ts-ignore -- shared build plugin is plain JavaScript.
import forkStyles from "../postcss-fork-styles.mjs";
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

test("unknown and missing feature styles fail the build instead of losing styling", async () => {
  const dir = directory();
  await expect(
    postcss([forkStyles({ directory: dir })]).process(
      "/* foxxycode:styles typo */",
      { from: "styles.css" },
    ),
  ).rejects.toThrow(/Unknown fork stylesheet/);
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
