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
//   node scripts/prepare-binary.mjs --target              # ONE target from $npm_config_vsce_target
//   node scripts/prepare-binary.mjs --help
//
// `--target` accepts a Go target (linux-amd64) or a vsce target (linux-x64); with no
// value it falls back to $npm_config_vsce_target, the same value `package:target` feeds
// to `vsce package --target`, so one flag drives both the Go build and the VSIX.
//
// Universal mode produces one VSIX that runs on every desktop target by bundling all
// binaries; platform-specific mode (paired with `vsce package --target <vsce-target>`)
// produces one VSIX per platform. See editors/vscode/README.md.

import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync, rmSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join, resolve } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(here, "..", "..", ".."); // editors/vscode/scripts -> repo root

// Version stamped into the bundled binary's internal/version.Version, mirroring the
// IntelliJ gradle build (`-X …version.Version=${project.version}`). The Makefile passes
// FOXXYCODE_PLUGIN_VERSION (from PLUGIN_VERSION); otherwise fall back to the extension
// manifest version so a bare `npm run prepare-binary` still stamps something meaningful.
function pluginVersion() {
  const fromEnv = (process.env.FOXXYCODE_PLUGIN_VERSION || "").trim();
  if (fromEnv) return fromEnv;
  try {
    const pkg = JSON.parse(readFileSync(join(here, "..", "package.json"), "utf8"));
    return String(pkg.version || "dev");
  } catch {
    return "dev";
  }
}

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

// Normalize a target token to Go GOOS/GOARCH, accepting either a Go target
// (`windows-amd64`) or a vsce target (`win32-x64`) so callers can reuse the same
// `npm_config_vsce_target` value that drives `vsce package --target`.
export function normalizeToGoTarget(token) {
  const [rawOs, rawArch] = String(token).split("-");
  if (!rawOs || !rawArch) {
    throw new Error(`target expects <os>-<arch>, got "${token}"`);
  }
  const goos = rawOs === "win32" ? "windows" : rawOs;
  const goarch = rawArch === "x64" ? "amd64" : rawArch;
  return { goos, goarch };
}

function parseArgs(argv) {
  if (argv.includes("--help") || argv.includes("-h")) {
    return { mode: "help" };
  }
  const i = argv.indexOf("--target");
  if (i !== -1) {
    // Target value comes from the positional arg when given, else from the
    // npm_config_vsce_target env var (set by `npm run … --vsce_target=win32-x64`),
    // so a single source of truth drives both the Go build and `vsce package`.
    const positional = argv[i + 1] && !argv[i + 1].startsWith("-") ? argv[i + 1] : undefined;
    const raw = positional ?? process.env.npm_config_vsce_target;
    if (!raw) {
      throw new Error(
        "--target needs a value: pass `--target <os>-<arch>` (e.g. windows-amd64) or set " +
          "npm_config_vsce_target (e.g. `npm run package:target --vsce_target=win32-x64`).",
      );
    }
    return { mode: "single", ...normalizeToGoTarget(raw) };
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
      `-s -w -X github.com/hijera/foxxycode-agent/internal/version.Version=${pluginVersion()}`,
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
    // Platform-specific mode: wipe any stale binaries from other targets left
    // over from a previous universal/single build, so only the requested
    // target ends up bundled in the VSIX.
    rmSync(join(here, "..", "foxxycode-bin"), { recursive: true, force: true });
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
