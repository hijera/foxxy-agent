// Cross-compile the foxxycode binary for one or all targets and stage it/them under
// editors/vscode/foxxycode-bin/<goos>-<goarch>/ for the VSIX to bundle.
//
// Mirrors the Gradle `foxxycodeGoBuild_*` tasks used by the IntelliJ plugin. The Go
// source is the repo root (editors/vscode/scripts -> repo root = two levels up); there
// is no nested copy.
//
// Usage:
//   node scripts/prepare-binary.mjs                       # build ALL 5 targets (universal VSIX)
//   node scripts/prepare-binary.mjs --target linux-amd64  # build ONE target (platform-specific VSIX)
//   node scripts/prepare-binary.mjs --help
//
// Universal mode produces one VSIX that runs on every desktop target by bundling all
// binaries; platform-specific mode (paired with `vsce package --target <vsce-target>`)
// produces one VSIX per platform. See editors/vscode/README.md.

import { execFileSync } from "node:child_process";
import { mkdirSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join, resolve } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(here, "..", "..", ".."); // editors/vscode/scripts -> repo root

// All desktop targets shipped by the IntelliJ plugin and release-binaries.yaml.
export const TARGETS = [
  { goos: "linux", goarch: "amd64" },
  { goos: "linux", goarch: "arm64" },
  { goos: "darwin", goarch: "amd64" },
  { goos: "darwin", goarch: "arm64" },
  { goos: "windows", goarch: "amd64" },
];

// Map a Go target to the VS Code `vsce --target` identifier.
export function goToVscodeTarget(goos, goarch) {
  const arch = goarch === "amd64" ? "x64" : goarch;
  const os = goos === "windows" ? "win32" : goos;
  return `${os}-${arch}`;
}

function parseArgs(argv) {
  const i = argv.indexOf("--target");
  if (i !== -1 && argv[i + 1]) {
    const [goos, goarch] = argv[i + 1].split("-");
    if (!goos || !goarch) {
      throw new Error(`--target expects <goos>-<goarch>, got "${argv[i + 1]}"`);
    }
    return { mode: "single", goos, goarch };
  }
  if (argv.includes("--help") || argv.includes("-h")) {
    return { mode: "help" };
  }
  return { mode: "all" };
}

function binNameFor(goos) {
  return goos === "windows" ? "foxxycode.exe" : "foxxycode";
}

function buildOne(goos, goarch) {
  const outDir = join(here, "..", "foxxycode-bin", `${goos}-${goarch}`);
  mkdirSync(outDir, { recursive: true });
  const outFile = join(outDir, binNameFor(goos));
  console.log(`Building foxxycode for ${goos}-${goarch} -> ${outFile}`);
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
      outFile,
      "./cmd/foxxycode/",
    ],
    {
      cwd: repoRoot,
      stdio: "inherit",
      env: { ...process.env, GOOS: goos, GOARCH: goarch, CGO_ENABLED: "0" },
    },
  );
}

function printHelp() {
  console.log(`Usage:
  node scripts/prepare-binary.mjs                 Build ALL 5 targets (universal VSIX)
  node scripts/prepare-binary.mjs --target <t>    Build ONE target (platform-specific VSIX)

Targets (Go <goos>-<goarch>, VS Code target in parentheses):
${TARGETS.map((t) => `  ${t.goos}-${t.goarch}  ->  ${goToVscodeTarget(t.goos, t.goarch)}`).join("\n")}

Pair platform-specific builds with:
  npx vsce package --target <vsce-target>`);
}

function main() {
  const parsed = parseArgs(process.argv);
  if (parsed.mode === "help") {
    printHelp();
    return;
  }
  if (parsed.mode === "single") {
    buildOne(parsed.goos, parsed.goarch);
    return;
  }
  // Universal: build every target.
  for (const t of TARGETS) buildOne(t.goos, t.goarch);
}

// Only run main() when invoked directly via `node scripts/prepare-binary.mjs ...`,
// not when imported by tests.
const invokedDirectly =
  process.argv[1] && resolve(process.argv[1]) === resolve(fileURLToPath(import.meta.url));
if (invokedDirectly) main();
