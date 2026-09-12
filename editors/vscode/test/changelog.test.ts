// Guards for editors/vscode/CHANGELOG.md, the source of the Changelog tab VS Code shows on the
// extension page. Mirrors editors/intellij/changelog_test.go: the file is product copy read
// by users, so its shape is enforced here rather than discovered after a release.
import { describe, it, expect } from "vitest";
import * as fs from "node:fs";
import * as path from "node:path";
import { stampUnreleased, UNRELEASED_HEADING } from "../scripts/stamp-changelog.mjs";

const root = path.resolve(__dirname, "..");
const changelogPath = path.join(root, "CHANGELOG.md");
const text = fs.readFileSync(changelogPath, "utf8");
const lines = text.split(/\r?\n/);

const HEADING = /^##\s+(\d+\.\d+\.\d+)\s*[—-]\s*(\d{4}-\d{2}-\d{2})\s*$/;
const UNRELEASED = /^##\s+Unreleased\s*[—-]\s*(\d{4}-\d{2}-\d{2})\s*$/;

interface Section {
  line: number;
  version: string; // "Unreleased" or X.Y.Z
  body: string[];
}

function sections(): Section[] {
  const out: Section[] = [];
  for (let i = 0; i < lines.length; i++) {
    const trimmed = lines[i].trim();
    let version: string | null = null;
    if (HEADING.test(trimmed)) version = trimmed.match(HEADING)![1];
    else if (UNRELEASED.test(trimmed)) version = "Unreleased";
    if (version !== null) {
      out.push({ line: i + 1, version, body: [] });
    } else if (out.length > 0) {
      out[out.length - 1].body.push(lines[i]);
    }
  }
  return out;
}

function compareSemver(a: string, b: string): number {
  const pa = a.split(".").map(Number);
  const pb = b.split(".").map(Number);
  for (let i = 0; i < 3; i++) {
    if (pa[i] !== pb[i]) return pa[i] - pb[i];
  }
  return 0;
}

const hasCyrillic = (s: string): boolean => /[Ѐ-ӿ]/.test(s);

describe("CHANGELOG.md", () => {
  const all = sections();

  it("has at least one section", () => {
    expect(all.length, "no `## X.Y.Z — YYYY-MM-DD` sections").toBeGreaterThan(0);
  });

  it("shows users nothing but the notes: no authoring instructions before the first section", () => {
    // VS Code renders the whole file in the Changelog tab (IntelliJ only renders the
    // sections), so process notes for authors must stay in an HTML comment or in the rules.
    const preamble = text
      .slice(0, text.search(/^##\s/m))
      .replace(/<!--[\s\S]*?-->/g, "")
      .split(/\r?\n/)
      .map((l) => l.trim())
      .filter((l) => l !== "");
    expect(preamble, "only the `# title` line may precede the first section").toEqual(
      preamble.filter((l) => l.startsWith("# ")),
    );
    expect(preamble.length).toBeLessThanOrEqual(1);
  });

  it("keeps the Unreleased section single and newest", () => {
    const unreleased = all.filter((s) => s.version === "Unreleased");
    expect(unreleased.length, "more than one `## Unreleased` section").toBeLessThanOrEqual(1);
    if (unreleased.length === 1) {
      expect(all[0].version, "`## Unreleased` must be the first section").toBe("Unreleased");
    }
  });

  it("orders released versions newest first", () => {
    const released = all.filter((s) => s.version !== "Unreleased");
    for (let i = 1; i < released.length; i++) {
      expect(
        compareSemver(released[i].version, released[i - 1].version),
        `line ${released[i].line}: ${released[i].version} is not below ${released[i - 1].version}`,
      ).toBeLessThan(0);
    }
  });

  it("has no merge-conflict markers", () => {
    for (let i = 0; i < lines.length; i++) {
      for (const marker of ["<<<<<<<", "=======", ">>>>>>>"]) {
        expect(lines[i].startsWith(marker), `line ${i + 1}: conflict marker ${marker}`).toBe(false);
      }
    }
  });

  it("writes every entry in Russian, for humans", () => {
    for (const s of all) {
      const body = s.body.join("\n").trim();
      expect(body, `${s.version}: empty section`).not.toBe("");
      expect(/\bTBD\b/i.test(body), `${s.version}: stub entry`).toBe(false);
      expect(hasCyrillic(body), `${s.version}: no Russian text`).toBe(true);
      expect(
        /^-\s+[0-9a-f]{7,}\b/m.test(body),
        `${s.version}: reads like a commit list; describe the change instead`,
      ).toBe(false);
    }
  });
});

describe("stamp-changelog.mjs", () => {
  const sample = "# Title\n\n## Unreleased — 2026-09-07\n\n**Что-то.**\nТекст.\n\n## 0.2.50 — 2026-09-06\n\nСтарое.\n";

  it("stamps the Unreleased heading with the release version", () => {
    const out = stampUnreleased(sample, "0.2.51");
    expect(out).toContain("## 0.2.51 — 2026-09-07");
    expect(out).not.toMatch(UNRELEASED_HEADING);
    expect(out).toContain("## 0.2.50 — 2026-09-06");
    // Only the heading changes: the blank line after it (and everything else) stays.
    expect(out).toBe(sample.replace("## Unreleased — 2026-09-07", "## 0.2.51 — 2026-09-07"));
  });

  it("does not touch the real changelog when run against a copy", () => {
    // The CLI writes in place, so packaging snapshots the file first; the pure
    // function must never leave the tree dirty on its own.
    expect(fs.readFileSync(changelogPath, "utf8")).toBe(text);
  });

  it("leaves dev builds and files without an Unreleased section alone", () => {
    expect(stampUnreleased(sample, "0.0.0-dev-abc123")).toBe(sample);
    const released = sample.replace("## Unreleased — 2026-09-07", "## 0.2.51 — 2026-09-07");
    expect(stampUnreleased(released, "0.2.52")).toBe(released);
  });

  it("recognises the real changelog's Unreleased heading, when present", () => {
    const first = sections()[0];
    if (first?.version === "Unreleased") {
      expect(stampUnreleased(text, "9.9.9")).toContain("## 9.9.9 — ");
    }
  });
});

describe("packaging", () => {
  const vscodeignore = fs.readFileSync(path.join(root, ".vscodeignore"), "utf8");
  const makefile = fs.readFileSync(path.join(root, "..", "..", "Makefile"), "utf8");

  it("ships CHANGELOG.md in the VSIX but not its packaging backup", () => {
    const ignored = vscodeignore.split(/\r?\n/).map((l) => l.trim());
    expect(ignored).not.toContain("CHANGELOG.md");
    expect(ignored).toContain("CHANGELOG.md.vsce.bak");
  });

  it("stamps the Unreleased section in both VSIX packaging targets", () => {
    for (const target of ["vscode-package:", "vscode-package-target:"]) {
      const start = makefile.indexOf(target);
      expect(start, `Makefile target ${target} not found`).toBeGreaterThanOrEqual(0);
      const body = makefile.slice(start, makefile.indexOf("\n\n", start));
      expect(body, `${target} does not stamp CHANGELOG.md`).toContain("scripts/stamp-changelog.mjs");
      expect(body, `${target} does not restore CHANGELOG.md`).toContain("CHANGELOG.md.vsce.bak");
    }
  });
});
