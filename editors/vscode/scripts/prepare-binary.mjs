// Cross-compile the foxxy binary for a single target and stage it under
// editors/vscode/foxxy-bin/<goos>-<goarch>/ for the VSIX to bundle.
//
// SCAFFOLD: mirrors the Gradle `foxxyGoBuild_*` tasks used by the IntelliJ plugin.
// Usage:  node scripts/prepare-binary.mjs [--target <goos>-<goarch>]
// Default target is the host platform. VS Code ships platform-specific VSIX, so
// CI would call `vsce package --target <os>-<arch>` once per target, each time
// preparing the matching binary here first.
//
// Not invoked by repo CI yet — this documents the intended build for when the
// extension is implemented.

import { execFileSync } from "node:child_process";
import { mkdirSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join, resolve } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(here, "..", "..", ".."); // editors/vscode/scripts -> repo root

// Map Node's process.platform/arch to Go's GOOS/GOARCH.
const goosByPlatform = { win32: "windows", darwin: "darwin", linux: "linux" };
const goarchByArch = { x64: "amd64", arm64: "arm64" };

function parseTarget() {
  const i = process.argv.indexOf("--target");
  if (i !== -1 && process.argv[i + 1]) {
    const [goos, goarch] = process.argv[i + 1].split("-");
    return { goos, goarch };
  }
  return {
    goos: goosByPlatform[process.platform] ?? "linux",
    goarch: goarchByArch[process.arch] ?? "amd64",
  };
}

const { goos, goarch } = parseTarget();
const binName = goos === "windows" ? "foxxy.exe" : "foxxy";
const outDir = join(here, "..", "foxxy-bin", `${goos}-${goarch}`);
mkdirSync(outDir, { recursive: true });

console.log(`Building foxxy for ${goos}-${goarch} -> ${join(outDir, binName)}`);
execFileSync(
  "go",
  [
    "build",
    "-tags",
    "http ui scheduler memory",
    "-trimpath",
    "-ldflags",
    "-s -w",
    "-o",
    join(outDir, binName),
    "./cmd/coddy/",
  ],
  {
    cwd: repoRoot,
    stdio: "inherit",
    env: { ...process.env, GOOS: goos, GOARCH: goarch, CGO_ENABLED: "0" },
  }
);
