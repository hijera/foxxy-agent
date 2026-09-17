// @ts-check
// Assembles what GitHub Pages publishes: the whole docs/ tree as it is, so every address that
// already works keeps working (updatePlugins.xml, which JetBrains IDEs poll for plugin updates,
// and config.schema.json, which FoxxyCode writes into every config it saves), with the built
// site laid over the top.
//
//   node scripts/assemble.mjs --docs ../docs --dist dist --out _site
//   node scripts/assemble.mjs --docs ../docs --out _site          docs only, when the site failed to build
//
// The overlay never replaces a documentation file: a collision fails the run instead.

import { cpSync, existsSync, readdirSync, readFileSync, rmSync, statSync } from "node:fs";
import { join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

/** Files the published site cannot lose. */
export const REQUIRED_DOCS = ["updatePlugins.xml", "config.schema.json"];

/**
 * @param {string} dir
 * @returns {string[]} paths relative to dir, files only
 */
function listFiles(dir) {
  /** @type {string[]} */
  const out = [];
  const walk = (/** @type {string} */ current) => {
    for (const entry of readdirSync(current)) {
      const path = join(current, entry);
      // statSync follows symbolic links, as the copy below does.
      if (statSync(path).isDirectory()) walk(path);
      else out.push(relative(dir, path).split("\\").join("/"));
    }
  };
  walk(dir);
  return out;
}

/**
 * @param {{ docs: string, dist?: string | null, out: string }} options
 * @returns {{ docsFiles: number, siteFiles: number }}
 */
export function assemble({ docs, dist = null, out }) {
  if (!existsSync(docs)) throw new Error(`no documentation tree at ${docs}`);
  for (const file of REQUIRED_DOCS) {
    if (!existsSync(join(docs, file))) throw new Error(`${file} is missing from ${docs}`);
  }

  const docsFiles = new Set(listFiles(docs));
  /** @type {string[]} */
  let siteFiles = [];
  if (dist) {
    if (!existsSync(join(dist, "index.html"))) throw new Error(`${dist} has no index.html; build the site first`);
    siteFiles = listFiles(dist);
    const collisions = siteFiles.filter((f) => docsFiles.has(f));
    if (collisions.length > 0) {
      throw new Error(`the site would overwrite documentation files: ${collisions.join(", ")}`);
    }
    // The page reads baked data only; a call to the API from the browser is a regression.
    for (const file of siteFiles.filter((f) => f.endsWith(".js") || f.endsWith(".html"))) {
      if (readFileSync(join(dist, file), "utf8").includes("api.github.com")) {
        throw new Error(`${file} calls api.github.com; the site must use the data baked at build time`);
      }
    }
  }

  rmSync(out, { recursive: true, force: true });
  cpSync(docs, out, { recursive: true, dereference: true });
  if (dist) cpSync(dist, out, { recursive: true, dereference: true });
  return { docsFiles: docsFiles.size, siteFiles: siteFiles.length };
}

/** @param {string[]} argv */
function parseArgs(argv) {
  /** @type {{ docs: string | null, dist: string | null, out: string | null }} */
  const args = { docs: null, dist: null, out: null };
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    const value = argv[++i];
    if (value === undefined) throw new Error(`${arg} needs a value`);
    if (arg === "--docs") args.docs = resolve(value);
    else if (arg === "--dist") args.dist = resolve(value);
    else if (arg === "--out") args.out = resolve(value);
    else throw new Error(`unknown argument ${arg}`);
  }
  if (!args.docs || !args.out) throw new Error("usage: assemble.mjs --docs <dir> [--dist <dir>] --out <dir>");
  return { docs: args.docs, dist: args.dist, out: args.out };
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const args = parseArgs(process.argv.slice(2));
    const { docsFiles, siteFiles } = assemble(args);
    console.log(`assemble: ${docsFiles} documentation files + ${siteFiles} site files -> ${args.out}`);
  } catch (err) {
    console.error(`assemble: ${err instanceof Error ? err.message : String(err)}`);
    process.exit(1);
  }
}
