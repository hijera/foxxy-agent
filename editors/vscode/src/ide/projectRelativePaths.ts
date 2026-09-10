/** Pure path helpers behind "Add to FoxxyCode". Kept free of any `vscode`
 *  import so they can be unit-tested in a plain Node environment; mirrors
 *  `editors/intellij/.../ui/ProjectRelativePaths.kt`. */

import * as nodePath from "path";
import * as fs from "fs";

export interface RelativizeOptions {
  /** Directory probe; the default stats the path and treats errors as "not a directory". */
  isDirectory?: (absPath: string) => boolean;
  /** Path implementation (tests inject `path.win32` / `path.posix`); defaults to the host's. */
  pathImpl?: nodePath.PlatformPath;
}

function defaultIsDirectory(absPath: string): boolean {
  try {
    return fs.statSync(absPath).isDirectory();
  } catch {
    return false;
  }
}

/** Workspace-relative POSIX paths for `absPaths`, in input order and de-duplicated.
 *  Directories, the root itself, paths outside `basePath`, and anything that
 *  cannot be relativized are dropped, so an unrelated selection simply yields
 *  nothing. A blank `basePath` yields nothing as well. */
export function relativizeWorkspacePaths(
  basePath: string | undefined | null,
  absPaths: readonly string[],
  opts: RelativizeOptions = {},
): string[] {
  const base = (basePath ?? "").trim();
  if (base === "" || absPaths.length === 0) return [];
  const p = opts.pathImpl ?? nodePath;
  const isDirectory = opts.isDirectory ?? defaultIsDirectory;
  const out = new Set<string>();
  for (const abs of absPaths) {
    const target = (abs ?? "").trim();
    if (target === "" || isDirectory(target)) continue;
    let rel: string;
    try {
      rel = p.relative(base, target);
    } catch {
      continue;
    }
    // "" is the root itself; ".." leads outside; an absolute result is a
    // different Windows drive.
    if (rel === "" || rel === ".." || rel.startsWith(".." + p.sep) || p.isAbsolute(rel)) continue;
    out.add(rel.split(p.sep).join("/"));
  }
  return [...out];
}
