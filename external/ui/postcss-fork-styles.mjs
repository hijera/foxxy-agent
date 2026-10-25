/** Expand fork-owned CSS at its original cascade position before theme transforms. */
import { readFileSync, readdirSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import postcss from "postcss";

const moduleDirectory = path.dirname(fileURLToPath(import.meta.url));
const defaultDirectory = path.join(moduleDirectory, "src", "styles", "fork");

/**
 * A marker is `foxxycode:styles` plus exactly one token: the feature name whose
 * `<name>.css` the directory holds. A longer comment is documentation prose (a
 * porting note may name the mechanism) and stays in the composed stylesheet, so
 * only a single token after the reserved prefix is ever treated as a marker.
 */
const marker = /^foxxycode:styles\s+(\S+)\s*$/;

/** Feature name (lowercased, without .css) to the file the directory actually holds. */
function featureFiles(directory) {
  const files = new Map();
  for (const entry of readdirSync(directory)) {
    if (!entry.endsWith(".css")) continue;
    files.set(entry.slice(0, -".css".length).toLowerCase(), entry);
  }
  return files;
}

export function expandForkStyles(
  root,
  { directory = defaultDirectory, result } = {},
) {
  const features = featureFiles(directory);
  root.walkComments((comment) => {
    const name = comment.text.match(marker)?.[1]?.toLowerCase();
    if (!name) return;
    if (comment.parent.type !== "root")
      throw comment.error("Fork stylesheet markers must be at the top level");
    const entry = features.get(name);
    if (!entry)
      throw comment.error(`Unknown fork stylesheet: ${comment.text}`);
    const file = path.join(directory, entry);
    let source;
    try {
      source = readFileSync(file, "utf8");
    } catch (error) {
      throw comment.error(
        `Cannot load fork stylesheet ${file}: ${error.message}`,
      );
    }
    const feature = postcss.parse(source, { from: file });
    feature.walkComments((child) => {
      if (child.text.match(marker)) {
        throw child.error("Feature styles cannot include other feature styles");
      }
    });
    // The shipped stylesheet is one file: scripts-sync-to-go.mjs copies
    // dist/styles.css alone, so an import a feature file left in the cascade
    // would 404 in the embedded page and drop those styles silently.
    feature.walkAtRules("import", (rule) => {
      throw rule.error(
        "Feature styles cannot use @import: the composed stylesheet carries no runtime imports",
      );
    });
    result?.messages.push({
      type: "dependency",
      plugin: "foxxycode-fork-styles",
      file,
      parent: root.source?.input.file,
    });
    comment.replaceWith(...feature.nodes);
  });
  return root;
}

export function readStyleSource() {
  const file = path.join(moduleDirectory, "src", "styles.css");
  return expandForkStyles(
    postcss.parse(readFileSync(file, "utf8"), { from: file }),
  ).toString();
}

export default function forkStyles(options = {}) {
  return {
    postcssPlugin: "foxxycode-fork-styles",
    Once(root, { result }) {
      expandForkStyles(root, { ...options, result });
    },
  };
}
forkStyles.postcss = true;
