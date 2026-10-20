// @ts-check
import { execFileSync } from "node:child_process";

/**
 * The content of `path` (relative to the repository root) at `tag`, or null when the tag or the
 * file is not available in this clone (a shallow checkout, a tag not fetched).
 * @param {string} repoRoot
 * @param {string} tag
 * @param {string} path
 * @returns {string | null}
 */
export function fileAtTag(repoRoot, tag, path) {
  try {
    return execFileSync("git", ["show", `refs/tags/${tag}:${path}`], {
      cwd: repoRoot,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "ignore"],
      maxBuffer: 32 * 1024 * 1024,
    });
  } catch {
    return null;
  }
}
