// The plugin changelogs as they are in the repository: the website must be able to show the
// recent releases in both languages without the network.
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import {
  compareSemver,
  mergeChangelogs,
  parseChangelog,
  resolveUnreleased,
  selectVersions,
  translationFloor,
} from "../scripts/lib/changelog.mjs";

const repo = resolve(__dirname, "../..");
const read = (path: string) => parseChangelog(readFileSync(resolve(repo, path), "utf8"));

describe("the repository changelogs", () => {
  const sources = {
    intellij: {
      ru: resolveUnreleased(read("editors/intellij/CHANGELOG.md"), "9.9.9"),
      en: resolveUnreleased(read("editors/intellij/CHANGELOG.en.md"), "9.9.9"),
    },
    vscode: {
      ru: resolveUnreleased(read("editors/vscode/CHANGELOG.md"), "9.9.9"),
      en: resolveUnreleased(read("editors/vscode/CHANGELOG.en.md"), "9.9.9"),
    },
  };
  const floors = [translationFloor(sources.intellij.en), translationFloor(sources.vscode.en)];

  it("have English twins", () => {
    expect(floors.every((f) => f !== null)).toBe(true);
  });

  it("give the page ten releases, all translated", () => {
    const floor = floors.reduce((max, v) => (compareSemver(v!, max!) > 0 ? v : max));
    const versions = selectVersions(mergeChangelogs(sources), { limit: 10, floor });
    expect(versions.length).toBeGreaterThan(0);
    for (const version of versions) {
      expect(version.translated, `${version.version} has entries without an English translation`).toBe(true);
      for (const entry of version.entries) {
        expect(entry.blocks.ru.length + (entry.title.ru ? 1 : 0), `${version.version}: empty entry`).toBeGreaterThan(0);
      }
    }
  });
});
