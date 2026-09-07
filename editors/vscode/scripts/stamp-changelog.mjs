#!/usr/bin/env node
// Stamps the `## Unreleased — YYYY-MM-DD` section of CHANGELOG.md with the version being
// packaged, the way editors/intellij/build.gradle.kts does for the IntelliJ plugin.
//
// The version a merge releases under is only known when CI tags it (see
// .claude/rules/release-changelog.md), so the newest section is written as `Unreleased`
// and `make vscode-package` rewrites the heading right before `vsce package`, after
// snapshotting the file (CHANGELOG.md.vsce.bak) so the source tree is restored afterwards.
// VS Code renders CHANGELOG.md verbatim in the extension's Changelog tab, which is why the
// stamping has to happen in the packaged file rather than at display time.
//
// Usage: node scripts/stamp-changelog.mjs <X.Y.Z> [path/to/CHANGELOG.md]
// Exit 0 whether or not an Unreleased heading was present; a non-semver version is a no-op.

import { readFileSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));

/** Mirrors the heading regexes in editors/intellij/build.gradle.kts and changelog_test.go.
 *  Trailing whitespace is matched with `[ \t]*`, not `\s*`: with the `m` flag `\s*` would
 *  swallow the newline and the blank line after the heading. */
export const UNRELEASED_HEADING = /^##[ \t]+Unreleased[ \t]*[—-][ \t]*(\d{4}-\d{2}-\d{2})[ \t]*$/m;
export const SEMVER = /^\d+\.\d+\.\d+$/;

/** Returns `text` with the first `## Unreleased — <date>` heading replaced by
 *  `## <version> — <date>`. Unchanged when there is no such heading or when `version`
 *  is not a plain X.Y.Z (dev builds keep the Unreleased heading on purpose). */
export function stampUnreleased(text, version) {
  if (!SEMVER.test(version)) return text;
  return text.replace(UNRELEASED_HEADING, (_m, date) => `## ${version} — ${date}`);
}

function main() {
  const [, , version, file] = process.argv;
  if (!version) {
    console.error("usage: stamp-changelog.mjs <X.Y.Z> [CHANGELOG.md]");
    process.exit(2);
  }
  const path = resolve(file ?? resolve(here, "..", "CHANGELOG.md"));
  const before = readFileSync(path, "utf8");
  const after = stampUnreleased(before, version);
  if (after === before) {
    console.log(`[stamp-changelog] ${path}: nothing to stamp for ${version}`);
    return;
  }
  writeFileSync(path, after);
  console.log(`[stamp-changelog] ${path}: Unreleased -> ${version}`);
}

const invokedDirectly =
  process.argv[1] && resolve(process.argv[1]) === resolve(fileURLToPath(import.meta.url));
if (invokedDirectly) main();
