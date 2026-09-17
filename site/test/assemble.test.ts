// @vitest-environment node
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import { assemble } from "../scripts/assemble.mjs";

const dirs: string[] = [];
afterEach(() => {
  for (const dir of dirs.splice(0)) rmSync(dir, { recursive: true, force: true });
});

function tree(files: Record<string, string>): string {
  const root = mkdtempSync(join(tmpdir(), "foxxy-site-"));
  dirs.push(root);
  for (const [path, content] of Object.entries(files)) {
    mkdirSync(dirname(join(root, path)), { recursive: true });
    writeFileSync(join(root, path), content);
  }
  return root;
}

const DOCS = {
  "updatePlugins.xml": "<plugins/>",
  "config.schema.json": "{}",
  ".nojekyll": "",
  "assets/foxxycode1.png": "png",
  "getting-started/install.md": "# Install",
};

describe("assemble", () => {
  it("publishes docs/ byte for byte with the site on top", () => {
    const docs = tree(DOCS);
    const dist = tree({ "index.html": "<html></html>", "compare/index.html": "c", "site-assets/app.js": "x" });
    const out = join(tree({}), "_site");
    assemble({ docs, dist, out });
    for (const [path, content] of Object.entries(DOCS)) {
      expect(readFileSync(join(out, path), "utf8"), path).toBe(content);
    }
    expect(readFileSync(join(out, "index.html"), "utf8")).toBe("<html></html>");
    expect(existsSync(join(out, "site-assets/app.js"))).toBe(true);
  });

  it("refuses to overwrite a documentation file", () => {
    const docs = tree({ ...DOCS, "index.html": "docs index" });
    const dist = tree({ "index.html": "site" });
    expect(() => assemble({ docs, dist, out: join(tree({}), "_site") })).toThrow(/overwrite documentation files: index.html/);
  });

  it("requires the plugin repository and the schema", () => {
    const docs = tree({ "config.schema.json": "{}" });
    expect(() => assemble({ docs, out: join(tree({}), "_site") })).toThrow(/updatePlugins.xml is missing/);
  });

  it("rejects a site that calls the GitHub API from the browser", () => {
    const dist = tree({ "index.html": "i", "site-assets/a.js": 'fetch("https://api.github.com/repos")' });
    expect(() => assemble({ docs: tree(DOCS), dist, out: join(tree({}), "_site") })).toThrow(/api.github.com/);
  });

  it("publishes the docs alone when there is no site build", () => {
    const out = join(tree({}), "_site");
    assemble({ docs: tree(DOCS), out });
    expect(readdirSync(out).sort()).toEqual([".nojekyll", "assets", "config.schema.json", "getting-started", "updatePlugins.xml"]);
  });
});

describe("the repository docs/ tree", () => {
  const docs = resolve(__dirname, "../../docs");

  it("leaves the paths the site is published at free", () => {
    for (const path of ["index.html", "compare", "changelog", "site-assets", "screenshots", "404.html"]) {
      expect(existsSync(join(docs, path)), `docs/${path} would collide with the website`).toBe(false);
    }
  });
});
