/** Expand fork-owned CSS at its original cascade position before theme transforms. */
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import postcss from "postcss";

const moduleDirectory = path.dirname(fileURLToPath(import.meta.url));
const defaultDirectory = path.join(moduleDirectory, "src", "styles", "fork");
const features = new Set([
  "svn",
  "onboarding",
  "provider-transfer",
  "ide-embed",
]);

export function expandForkStyles(
  root,
  { directory = defaultDirectory, result } = {},
) {
  root.walkComments((comment) => {
    if (!comment.text.startsWith("foxxycode:styles")) return;
    const name = comment.text.match(/^foxxycode:styles ([a-z-]+)$/)?.[1];
    if (!features.has(name))
      throw comment.error(`Unknown fork stylesheet: ${comment.text}`);
    if (comment.parent.type !== "root")
      throw comment.error("Fork stylesheet markers must be at the top level");
    const file = path.join(directory, `${name}.css`);
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
      if (child.text.startsWith("foxxycode:styles")) {
        throw child.error("Feature styles cannot include other feature styles");
      }
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
