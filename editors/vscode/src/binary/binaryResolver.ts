import * as os from "os";
import * as path from "path";

/** Resolves the bundled foxxycode binary location for the running VS Code host platform.
 *  Mirrors `editors/intellij/.../binary/Platform.kt`. */

export interface GoPlatform {
  goos: string;
  goarch: string;
  binName: string;
  /** Path under `foxxycode-bin/` for the current platform, e.g. `linux-amd64/foxxycode`. */
  bundledRelative: string;
}

/** Node platform/arch → Go GOOS/GOARCH + binary name. */
export function detectPlatform(
  platform: NodeJS.Platform = process.platform,
  arch: string = process.arch,
): GoPlatform {
  const goos = platform === "win32" ? "windows" : platform === "darwin" ? "darwin" : "linux";
  const goarch = arch === "arm64" ? "arm64" : "amd64";
  const binName = goos === "windows" ? "foxxycode.exe" : "foxxycode";
  return { goos, goarch, binName, bundledRelative: `${goos}-${goarch}/${binName}` };
}

export interface BinaryValidation {
  ok: boolean;
  version: string | null;
  message: string;
}

/** Resolve the foxxycode binary to use.
 *
 *  Resolution order (matches IntelliJ `FoxxyCodeBinaryResolver`):
 *   1. The explicit `foxxycode.binaryPath` override (if it points to an existing file).
 *   2. The binary bundled with the extension under
 *      `<extensionPath>/foxxycode-bin/<goos>-<goarch>/foxxycode[.exe]`.
 *
 *  Returns `null` if no candidate exists; never throws.
 */
export function resolveExisting(extensionPath: string, overridePath: string): string | null {
  if (overridePath && overridePath.trim() !== "") {
    // Defer the existence check to the caller via fs — but we use a sync stat here.
    try {
      const fs = require("fs") as typeof import("fs");
      if (fs.statSync(overridePath).isFile()) return overridePath;
    } catch {
      // fall through to bundled lookup
    }
  }
  const plat = detectPlatform();
  const candidate = path.join(extensionPath, "foxxycode-bin", plat.bundledRelative);
  try {
    const fs = require("fs") as typeof import("fs");
    if (fs.statSync(candidate).isFile()) return candidate;
  } catch {
    return null;
  }
  return null;
}

/** Confirms the binary runs and is a full-feature build that supports `foxxycode http`.
 *  `run` executes the binary (see `execCapture.ts`; injected so this stays unit-testable)
 *  and `t` localizes the message. Mirrors `FoxxyCodeBinaryResolver.validate` step for
 *  step: missing file → `-v` failure → `http --help` failure → lean build → OK. */
export async function validateBinary(
  binaryPath: string,
  run: (cmd: string, args: string[]) => Promise<{ stdout: string }>,
  t: (key: string, ...params: unknown[]) => string,
): Promise<BinaryValidation> {
  const fs = require("fs") as typeof import("fs");
  if (!fs.existsSync(binaryPath)) {
    return { ok: false, version: null, message: t("binary.error.notFound", binaryPath) };
  }
  let version: string;
  try {
    version = (await run(binaryPath, ["-v"])).stdout.trim();
  } catch {
    return { ok: false, version: null, message: t("binary.error.executeVersion", binaryPath) };
  }
  let help: string;
  try {
    help = (await run(binaryPath, ["http", "--help"])).stdout;
  } catch {
    return { ok: false, version, message: t("binary.error.executeHelp", binaryPath) };
  }
  const text = help.toLowerCase();
  if (text.includes("not built") || text.includes("http support is not")) {
    return { ok: false, version, message: t("binary.error.leanBuild") };
  }
  return { ok: true, version, message: t("binary.ok.fullBuild", version) };
}

/** Host home directory fallback (used when `foxxycode.home` is empty). */
export function defaultFoxxycodeHome(): string {
  return path.join(os.homedir(), ".foxxycode");
}
