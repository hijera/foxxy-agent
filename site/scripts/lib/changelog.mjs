// @ts-check
// Parses the plugin changelogs (editors/{intellij,vscode}/CHANGELOG.md and their English twins
// CHANGELOG.en.md) into data the changelog page renders. Output is plain data - text, strong,
// code and link tokens - never HTML, so nothing from a changelog reaches the page as markup.
//
// The heading regexes are the ones editors/intellij/changelog_test.go and build.gradle.kts use.

/** @typedef {{ type: "text" | "strong" | "code", text: string } | { type: "link", text: string, href: string }} Inline */
/** @typedef {{ type: "p", inlines: Inline[] } | { type: "ul", items: Inline[][] }} Block */
/** @typedef {{ title: Inline[] | null, key: string, blocks: Block[] }} Entry */
/** @typedef {{ version: string | null, unreleased: boolean, date: string, entries: Entry[] }} Section */

const HEADING = /^##\s+(\d+\.\d+\.\d+)\s*[—-]\s*(\d{4}-\d{2}-\d{2})\s*$/;
const UNRELEASED = /^##\s+Unreleased\s*[—-]\s*(\d{4}-\d{2}-\d{2})\s*$/;
const TITLE = /^\*\*(.+?)\*\*\s*$/;
const BULLET = /^[-*]\s+(.*)$/;

/**
 * Splits a changelog into sections; anything above the first `##` heading is ignored.
 * @param {string} text
 * @returns {Section[]}
 */
export function parseChangelog(text) {
  /** @type {{ version: string | null, unreleased: boolean, date: string, lines: string[] }[]} */
  const raw = [];
  for (const line of text.replace(/<!--[\s\S]*?-->/g, "").split(/\r?\n/)) {
    const released = line.trim().match(HEADING);
    const unreleased = line.trim().match(UNRELEASED);
    if (released) {
      raw.push({ version: released[1] ?? null, unreleased: false, date: released[2] ?? "", lines: [] });
    } else if (unreleased) {
      raw.push({ version: null, unreleased: true, date: unreleased[1] ?? "", lines: [] });
    } else if (raw.length > 0) {
      raw[raw.length - 1]?.lines.push(line);
    }
  }
  return raw.map((s) => ({
    version: s.version,
    unreleased: s.unreleased,
    date: s.date,
    entries: parseEntries(s.lines),
  }));
}

/**
 * @param {string[]} lines
 * @returns {Entry[]}
 */
function parseEntries(lines) {
  /** @type {{ title: string | null, body: string[] }[]} */
  const groups = [];
  for (const line of lines) {
    const title = line.trim().match(TITLE);
    if (title) {
      groups.push({ title: title[1] ?? "", body: [] });
    } else {
      if (groups.length === 0) groups.push({ title: null, body: [] });
      groups[groups.length - 1]?.body.push(line);
    }
  }
  return groups
    .map((g) => ({
      title: g.title === null ? null : parseInline(g.title),
      key: normalize(`${g.title ?? ""}\n${g.body.join("\n")}`),
      blocks: parseBlocks(g.body),
    }))
    .filter((e) => e.title !== null || e.blocks.length > 0);
}

/**
 * Paragraphs are runs of non-blank lines joined with spaces; `- ` lines form a bullet list, and
 * an indented line continues the previous bullet.
 * @param {string[]} lines
 * @returns {Block[]}
 */
function parseBlocks(lines) {
  /** @type {Block[]} */
  const blocks = [];
  /** @type {string[]} */
  let paragraph = [];
  /** @type {string[][] | null} */
  let list = null;
  const flushParagraph = () => {
    if (paragraph.length > 0) blocks.push({ type: "p", inlines: parseInline(paragraph.join(" ")) });
    paragraph = [];
  };
  const flushList = () => {
    if (list) blocks.push({ type: "ul", items: list.map((item) => parseInline(item.join(" "))) });
    list = null;
  };
  for (const line of lines) {
    const trimmed = line.trim();
    if (trimmed === "") {
      flushParagraph();
      flushList();
      continue;
    }
    const bullet = trimmed.match(BULLET);
    if (bullet) {
      flushParagraph();
      if (!list) list = [];
      list.push([bullet[1] ?? ""]);
    } else if (list && /^\s+/.test(line)) {
      list[list.length - 1]?.push(trimmed);
    } else {
      flushList();
      paragraph.push(trimmed);
    }
  }
  flushParagraph();
  flushList();
  return blocks;
}

const INLINE = /`([^`]+)`|\*\*(.+?)\*\*|\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)/g;

/**
 * @param {string} text
 * @returns {Inline[]}
 */
export function parseInline(text) {
  /** @type {Inline[]} */
  const out = [];
  let last = 0;
  for (const m of text.matchAll(INLINE)) {
    const index = m.index ?? 0;
    if (index > last) out.push({ type: "text", text: text.slice(last, index) });
    if (m[1] !== undefined) out.push({ type: "code", text: m[1] });
    else if (m[2] !== undefined) out.push({ type: "strong", text: m[2] });
    else if (m[3] !== undefined && m[4] !== undefined) out.push({ type: "link", text: m[3], href: m[4] });
    last = index + m[0].length;
  }
  if (last < text.length) out.push({ type: "text", text: text.slice(last) });
  return out;
}

/** @param {string} s */
function normalize(s) {
  return s.replace(/\s+/g, " ").trim();
}

/**
 * @param {string} a
 * @param {string} b
 */
export function compareSemver(a, b) {
  const pa = a.split(".").map(Number);
  const pb = b.split(".").map(Number);
  for (let i = 0; i < 3; i++) {
    const d = (pa[i] ?? 0) - (pb[i] ?? 0);
    if (d !== 0) return d;
  }
  return 0;
}

/** @typedef {"intellij" | "vscode"} Surface */
/** @typedef {{ ru: Section[], en: Section[] }} ChangelogPair */
/**
 * @typedef {{
 *   surfaces: Surface[],
 *   title: { ru: Inline[] | null, en: Inline[] | null },
 *   blocks: { ru: Block[], en: Block[] | null },
 * }} MergedEntry
 */
/** @typedef {{ version: string, date: string, entries: MergedEntry[], translated: boolean }} MergedVersion */

/**
 * Gives the `Unreleased` section its version. On main, an Unreleased section on top is the one
 * the latest release stamped - but only if that release really carried it, which the caller
 * checks against the file at the release tag. Otherwise the section is not released yet and is
 * dropped until the next build.
 * @param {Section[]} sections
 * @param {string | null} version the version to give it, or null to drop it
 * @returns {Section[]}
 */
export function resolveUnreleased(sections, version) {
  // A numbered section for that version already exists when the Unreleased one is newer than the
  // release: those notes are not out yet.
  const taken = version !== null && sections.some((s) => !s.unreleased && s.version === version);
  return sections.flatMap((s) => {
    if (!s.unreleased) return [s];
    return version && !taken ? [{ ...s, version, unreleased: false }] : [];
  });
}

/**
 * True when `atTag` (the changelog as the release tag had it) carries the same Unreleased section
 * as `current`, so the release that tag names is the one that shipped it.
 * @param {Section[]} current
 * @param {Section[]} atTag
 */
export function sameUnreleased(current, atTag) {
  const a = current.find((s) => s.unreleased);
  const b = atTag.find((s) => s.unreleased);
  if (!a || !b) return false;
  return a.date === b.date && a.entries.map((e) => e.key).join("\n") === b.entries.map((e) => e.key).join("\n");
}

/**
 * Merges the two plugins' changelogs into one list of versions. An entry both plugins carry word
 * for word appears once with both surfaces; the same title with different text (each plugin in
 * its own words) stays as two entries. Russian and English sections pair by version and their
 * entries by position, which the changelog tests keep aligned.
 * @param {Record<Surface, ChangelogPair>} sources
 * @returns {MergedVersion[]}
 */
export function mergeChangelogs(sources) {
  /** @type {Map<string, MergedVersion>} */
  const versions = new Map();
  /** @type {Map<string, Map<string, MergedEntry>>} */
  const byKey = new Map();
  for (const surface of /** @type {Surface[]} */ (["intellij", "vscode"])) {
    const { ru, en } = sources[surface];
    const enByVersion = new Map(en.filter((s) => s.version).map((s) => [s.version, s]));
    for (const section of ru) {
      if (!section.version) continue;
      let version = versions.get(section.version);
      if (!version) {
        version = { version: section.version, date: section.date, entries: [], translated: true };
        versions.set(section.version, version);
        byKey.set(section.version, new Map());
      }
      const keys = /** @type {Map<string, MergedEntry>} */ (byKey.get(section.version));
      const twin = enByVersion.get(section.version);
      section.entries.forEach((entry, i) => {
        const existing = keys.get(entry.key);
        if (existing) {
          if (!existing.surfaces.includes(surface)) existing.surfaces.push(surface);
          return;
        }
        const translation = twin?.entries[i];
        if (!translation) version.translated = false;
        /** @type {MergedEntry} */
        const merged = {
          surfaces: [surface],
          title: { ru: entry.title, en: translation ? translation.title : null },
          blocks: { ru: entry.blocks, en: translation ? translation.blocks : null },
        };
        keys.set(entry.key, merged);
        version.entries.push(merged);
      });
    }
  }
  return [...versions.values()].sort((a, b) => compareSemver(b.version, a.version));
}

/**
 * The newest `limit` versions, never below `floor` (the oldest version both English twins
 * translate), so both languages list the same releases.
 * @param {MergedVersion[]} versions newest first
 * @param {{ limit: number, floor: string | null }} options
 */
export function selectVersions(versions, { limit, floor }) {
  return versions.filter((v) => !floor || compareSemver(v.version, floor) >= 0).slice(0, limit);
}

/**
 * The oldest released version a twin translates, or null when it translates none.
 * @param {Section[]} en
 */
export function translationFloor(en) {
  const released = en.filter((s) => s.version).map((s) => /** @type {string} */ (s.version));
  if (released.length === 0) return null;
  return released.reduce((min, v) => (compareSemver(v, min) < 0 ? v : min));
}
