import { describe, expect, it } from "vitest";
import {
  compareSemver,
  mergeChangelogs,
  parseChangelog,
  parseInline,
  resolveUnreleased,
  sameUnreleased,
  selectVersions,
  translationFloor,
} from "../scripts/lib/changelog.mjs";

const RU = [
  "# Изменения",
  "",
  "Преамбула для авторов, её не показываем.",
  "",
  "## Unreleased — 2026-09-14",
  "",
  "**Общая запись.**",
  "Текст с `code` и **жирным**,",
  "перенесённый на вторую строку.",
  "",
  "## 0.2.89 — 2026-09-13",
  "",
  "**Только в IntelliJ.**",
  "Абзац.",
  "",
  "- **Пункт** — первый",
  "  продолжение пункта",
  "- второй",
  "",
  "## 0.2.88 — 2026-09-12",
  "",
  "Раздел без заголовка записи.",
].join("\r\n");

const EN = [
  "<!-- note for authors -->",
  "# Changes",
  "",
  "## Unreleased — 2026-09-14",
  "",
  "**Shared entry.**",
  "Text with `code` and **bold**.",
  "",
  "## 0.2.89 — 2026-09-13",
  "",
  "**IntelliJ only.**",
  "Paragraph.",
  "",
  "- **Item** — first",
  "- second",
].join("\n");

describe("parseChangelog", () => {
  it("reads sections from a CRLF file and skips the preamble", () => {
    const sections = parseChangelog(RU);
    expect(sections.map((s) => [s.version, s.unreleased, s.date])).toEqual([
      [null, true, "2026-09-14"],
      ["0.2.89", false, "2026-09-13"],
      ["0.2.88", false, "2026-09-12"],
    ]);
  });

  it("splits entries on bold title lines and joins wrapped paragraphs", () => {
    const [unreleased] = parseChangelog(RU);
    expect(unreleased?.entries).toHaveLength(1);
    const entry = unreleased!.entries[0]!;
    expect(entry.title).toEqual([{ type: "text", text: "Общая запись." }]);
    expect(entry.blocks).toEqual([
      {
        type: "p",
        inlines: [
          { type: "text", text: "Текст с " },
          { type: "code", text: "code" },
          { type: "text", text: " и " },
          { type: "strong", text: "жирным" },
          { type: "text", text: ", перенесённый на вторую строку." },
        ],
      },
    ]);
  });

  it("reads bullet lists with continuation lines", () => {
    const blocks = parseChangelog(RU)[1]!.entries[0]!.blocks;
    expect(blocks[1]).toEqual({
      type: "ul",
      items: [
        [
          { type: "strong", text: "Пункт" },
          { type: "text", text: " — первый продолжение пункта" },
        ],
        [{ type: "text", text: "второй" }],
      ],
    });
  });

  it("keeps a section without a title line as one untitled entry", () => {
    const entries = parseChangelog(RU)[2]!.entries;
    expect(entries).toHaveLength(1);
    expect(entries[0]!.title).toBeNull();
  });

  it("ignores headings inside HTML comments", () => {
    expect(parseChangelog("<!--\n## 9.9.9 — 2026-01-01\n-->\n## 0.1.0 — 2026-01-02\n\ntext")).toHaveLength(1);
  });
});

describe("parseInline", () => {
  it("recognises http links only", () => {
    expect(parseInline("see [docs](https://example.com/x) and [bad](javascript:alert(1))")).toEqual([
      { type: "text", text: "see " },
      { type: "link", text: "docs", href: "https://example.com/x" },
      { type: "text", text: " and [bad](javascript:alert(1))" },
    ]);
  });
});

describe("merging", () => {
  const ru = resolveUnreleased(parseChangelog(RU), "0.2.90");
  const en = resolveUnreleased(parseChangelog(EN), "0.2.90");

  it("gives Unreleased the release version or drops it", () => {
    expect(ru[0]?.version).toBe("0.2.90");
    expect(resolveUnreleased(parseChangelog(RU), null).map((s) => s.version)).toEqual(["0.2.89", "0.2.88"]);
  });

  it("never gives Unreleased the number of a release that already has its own section", () => {
    expect(resolveUnreleased(parseChangelog(RU), "0.2.89").map((s) => s.version)).toEqual(["0.2.89", "0.2.88"]);
  });

  it("shows an entry both plugins carry once, with both surfaces", () => {
    const vscodeRu = parseChangelog(RU.split("## 0.2.89")[0]!);
    const vscodeEn = parseChangelog(EN.split("## 0.2.89")[0]!);
    const merged = mergeChangelogs({
      intellij: { ru, en },
      vscode: { ru: resolveUnreleased(vscodeRu, "0.2.90"), en: resolveUnreleased(vscodeEn, "0.2.90") },
    });
    expect(merged.map((v) => v.version)).toEqual(["0.2.90", "0.2.89", "0.2.88"]);
    expect(merged[0]!.entries).toHaveLength(1);
    expect(merged[0]!.entries[0]!.surfaces).toEqual(["intellij", "vscode"]);
    expect(merged[0]!.entries[0]!.title.en).toEqual([{ type: "text", text: "Shared entry." }]);
    expect(merged[1]!.entries[0]!.surfaces).toEqual(["intellij"]);
    expect(merged[2]!.translated).toBe(false);
  });

  it("keeps the same title with different text as two entries", () => {
    const other = RU.replace("Текст с `code`", "Другой текст с `code`");
    const merged = mergeChangelogs({
      intellij: { ru, en },
      vscode: { ru: resolveUnreleased(parseChangelog(other), "0.2.90"), en },
    });
    expect(merged[0]!.entries.map((e) => e.surfaces)).toEqual([["intellij"], ["vscode"]]);
  });

  it("stops at the oldest translated version", () => {
    const merged = mergeChangelogs({ intellij: { ru, en }, vscode: { ru: [], en: [] } });
    const floor = translationFloor(en);
    expect(floor).toBe("0.2.89");
    expect(selectVersions(merged, { limit: 10, floor }).map((v) => v.version)).toEqual(["0.2.90", "0.2.89"]);
    expect(selectVersions(merged, { limit: 1, floor: null }).map((v) => v.version)).toEqual(["0.2.90"]);
  });

  it("recognises the release that shipped an Unreleased section", () => {
    const current = parseChangelog(RU);
    expect(sameUnreleased(current, parseChangelog(RU))).toBe(true);
    expect(sameUnreleased(current, parseChangelog(RU.replace("Общая запись.", "Иная запись.")))).toBe(false);
    expect(sameUnreleased(current, parseChangelog(RU.replace("2026-09-14", "2026-09-15")))).toBe(false);
  });

  it("orders versions numerically", () => {
    expect(["0.2.9", "0.2.10", "0.10.0"].sort((a, b) => compareSemver(b, a))).toEqual(["0.10.0", "0.2.10", "0.2.9"]);
  });
});
