/**
 * Canonical form for the workspace-relative paths that dropped-file **`@`**-mentions
 * carry. Both hosts hand the composer a path in their own dialect — IntelliJ relativizes
 * against the project base in Kotlin, VS Code goes through **`/workspace/relativize`** —
 * and the draft must end up with the same token either way.
 */

/** Forward slashes, no leading **`./`** / slashes, no trailing slash. */
export function normalizeRelPath(pathRel: string): string {
  return pathRel
    .replace(/\\/g, "/")
    .replace(/^\.\//, "")
    .replace(/^\/+/, "")
    .replace(/\/+$/, "");
}
