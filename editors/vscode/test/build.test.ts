// Build tests for the VS Code extension.
//
// The extension broke in the field once because the bundle was built with
// `--external:diff` while node_modules/** is excluded from the VSIX: the
// extension host failed on require("diff") with no logs (infinite loading).
// These tests compile the real bundle with the same options as the `compile`
// npm script and assert the artifact is self-contained, plus a set of
// packaging invariants that keep `vsce package` producing a working VSIX.
import { describe, it, expect, beforeAll } from "vitest";
import { build } from "esbuild";
import { builtinModules } from "node:module";
import * as fs from "node:fs";
import * as os from "node:os";
import * as path from "node:path";

const root = path.resolve(__dirname, "..");

interface PackageJson {
  main?: string;
  engines?: Record<string, string>;
  activationEvents?: string[];
  devDependencies?: Record<string, string>;
  scripts?: Record<string, string>;
  contributes?: {
    commands?: { command: string }[];
    views?: Record<string, { id: string; type?: string }[]>;
    viewsContainers?: { activitybar?: { id: string; icon: string }[] };
    walkthroughs?: {
      id: string;
      steps?: {
        id: string;
        media?: { markdown?: string };
        completionEvents?: string[];
      }[];
    }[];
  };
}

const pkg = JSON.parse(fs.readFileSync(path.join(root, "package.json"), "utf8")) as PackageJson;
const compileScript = pkg.scripts?.compile ?? "";

function externalsFromCompileScript(): string[] {
  return [...compileScript.matchAll(/--external:(\S+)/g)].map((m) => m[1]);
}

describe("compile script", () => {
  it("declares vscode as the ONLY external module", () => {
    // Anything else external is a runtime require() the VSIX cannot satisfy,
    // because .vscodeignore excludes node_modules/**.
    expect(externalsFromCompileScript()).toEqual(["vscode"]);
  });

  it("bundles into the file that package.json main points to", () => {
    const m = compileScript.match(/--outfile=(\S+)/);
    expect(m, "compile script must set --outfile").toBeTruthy();
    const outfile = m![1].replace(/^\.\//, "");
    const main = (pkg.main ?? "").replace(/^\.\//, "");
    expect(main).toBe(outfile);
  });
});

describe("extension bundle", () => {
  let bundle = "";

  beforeAll(async () => {
    const outDir = fs.mkdtempSync(path.join(os.tmpdir(), "foxxycode-build-test-"));
    const outfile = path.join(outDir, "extension.js");
    // Same shape as the `compile` script, but the externals come from the
    // script itself so the bundle-scan test below exercises the real config.
    await build({
      entryPoints: [path.join(root, "src", "extension.ts")],
      bundle: true,
      platform: "node",
      format: "cjs",
      external: externalsFromCompileScript(),
      outfile,
      logLevel: "silent",
    });
    bundle = fs.readFileSync(outfile, "utf8");
    fs.rmSync(outDir, { recursive: true, force: true });
  }, 120_000);

  it("produces a non-trivial self-contained bundle", () => {
    expect(bundle.length).toBeGreaterThan(10_000);
  });

  it("requires nothing at runtime except vscode and node builtins", () => {
    const builtins = new Set(builtinModules);
    const offenders = new Set<string>();
    for (const m of bundle.matchAll(/\brequire\("([^"]+)"\)/g)) {
      const name = m[1];
      if (name.startsWith(".") || name === "vscode") continue;
      if (builtins.has(name.replace(/^node:/, ""))) continue;
      offenders.add(name);
    }
    expect([...offenders], "unbundled require() calls would crash inside the VSIX").toEqual([]);
  });
});

describe("VSIX packaging invariants", () => {
  const vscodeignore = fs.readFileSync(path.join(root, ".vscodeignore"), "utf8");
  const ignored = vscodeignore
    .split(/\r?\n/)
    .map((l) => l.trim())
    .filter((l) => l && !l.startsWith("#"));

  it("excludes sources and node_modules from the VSIX", () => {
    for (const required of ["src/**", "node_modules/**", "test/**", "scripts/**"]) {
      expect(ignored, `.vscodeignore must exclude ${required}`).toContain(required);
    }
  });

  it("ships out/, foxxycode-bin/ and media/ in the VSIX", () => {
    for (const shipped of ["out/**", "out", "foxxycode-bin/**", "foxxycode-bin", "media/**", "media"]) {
      expect(ignored, `.vscodeignore must NOT exclude ${shipped}`).not.toContain(shipped);
    }
  });

  it("declares every activation command in contributes.commands", () => {
    const declared = new Set((pkg.contributes?.commands ?? []).map((c) => c.command));
    for (const event of pkg.activationEvents ?? []) {
      if (!event.startsWith("onCommand:")) continue;
      const cmd = event.slice("onCommand:".length);
      expect(declared.has(cmd), `activation event ${event} has no matching command`).toBe(true);
    }
  });

  it("activates on the view it contributes", () => {
    const viewIds = Object.values(pkg.contributes?.views ?? {})
      .flat()
      .map((v) => v.id);
    for (const event of pkg.activationEvents ?? []) {
      if (!event.startsWith("onView:")) continue;
      const id = event.slice("onView:".length);
      expect(viewIds, `activation event ${event} has no matching view`).toContain(id);
    }
  });

  it("has the activity bar icon on disk", () => {
    for (const container of pkg.contributes?.viewsContainers?.activitybar ?? []) {
      expect(fs.existsSync(path.join(root, container.icon)), `missing icon ${container.icon}`).toBe(
        true,
      );
    }
  });

  it("keeps engines.vscode in sync with @types/vscode", () => {
    const engine = pkg.engines?.vscode ?? "";
    const types = pkg.devDependencies?.["@types/vscode"] ?? "";
    expect(engine.replace(/^[^\d]*/, "")).toBe(types.replace(/^[^\d]*/, ""));
  });
});

describe("walkthrough", () => {
  const walkthrough = (pkg.contributes?.walkthroughs ?? []).find((w) => w.id === "foxxycode.welcome");

  it("contributes the foxxycode.welcome walkthrough", () => {
    expect(walkthrough, "missing walkthrough id foxxycode.welcome").toBeTruthy();
  });

  it("ships every walkthrough markdown step on disk", () => {
    expect(walkthrough?.steps?.length).toBeGreaterThan(0);
    for (const step of walkthrough?.steps ?? []) {
      const md = step.media?.markdown;
      expect(md, `step ${step.id} must declare media.markdown`).toBeTruthy();
      expect(
        fs.existsSync(path.join(root, md!)),
        `walkthrough step ${step.id} references missing file ${md}`,
      ).toBe(true);
    }
  });

  it("declares completionEvents for every onCommand step hook", () => {
    const declared = new Set((pkg.contributes?.commands ?? []).map((c) => c.command));
    for (const step of walkthrough?.steps ?? []) {
      for (const event of step.completionEvents ?? []) {
        if (!event.startsWith("onCommand:")) continue;
        const cmd = event.slice("onCommand:".length);
        expect(declared.has(cmd), `walkthrough step ${step.id} references unknown command ${cmd}`).toBe(
          true,
        );
      }
    }
  });

  it("declares foxxycode.showWelcome and activates on it", () => {
    const declared = new Set((pkg.contributes?.commands ?? []).map((c) => c.command));
    expect(declared.has("foxxycode.showWelcome")).toBe(true);
    expect(pkg.activationEvents ?? []).toContain("onCommand:foxxycode.showWelcome");
  });
});
