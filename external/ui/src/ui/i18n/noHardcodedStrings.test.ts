import { readFileSync, readdirSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

// Guard against user-visible English creeping back into the markup: every literal that reaches the
// screen must go through t(). Ports from upstream land as English JSX, and without a check the miss
// is invisible in an English UI (see UPSTREAM_SYNC.md — the env-chip menu stayed English for a
// whole release). Add a genuinely untranslatable string to ALLOWED_* below, with a reason.

const uiRoot = join(dirname(fileURLToPath(import.meta.url)), "..");

/** Files whose English text is deliberate. */
const ALLOWED_FILES = new Set([
  // The crash screen must render even when localization itself is the crash cause.
  "AppErrorBoundary.tsx",
]);

/** Literals that are keys, terms or brand names rather than prose. */
const ALLOWED_TEXT = new Set([
  "Esc", // keyboard key cap
  "PDF", // format name inside the file-type SVG badge
]);

const ALLOWED_ATTR_VALUES = new Set([
  // Syntax templates the user types verbatim — identical in every locale.
  "provider",
  "provider/model-id",
  "https://box.example:12345",
  "https://api.example.com/v1",
  "socks5h://127.0.0.1:1080",
]);

const SCANNED_ATTRS = [
  "title",
  "aria-label",
  "placeholder",
  "alt",
  "tooltip",
  "ariaLabel",
];

function tsxFiles(dir: string): string[] {
  const out: string[] = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = join(dir, entry.name);
    if (entry.isDirectory()) {
      out.push(...tsxFiles(full));
      continue;
    }
    if (!entry.name.endsWith(".tsx") || entry.name.includes(".test.")) {
      continue;
    }
    if (ALLOWED_FILES.has(entry.name)) {
      continue;
    }
    out.push(full);
  }
  return out;
}

/** Blanks a region while keeping every offset and line break, so line numbers stay honest. */
function blank(source: string, pattern: RegExp): string {
  return source.replace(pattern, (m) => m.replace(/[^\n]/g, " "));
}

/**
 * Drops the parts of a file that legitimately hold English: comments, and <code> elements, whose
 * content is a shell command or a config key that is the same in every locale.
 */
function scannable(source: string): string {
  let out = blank(source, /\/\*[\s\S]*?\*\//g);
  out = blank(out, /(^|[^:])\/\/[^\n]*/g);
  out = blank(out, /<code[^>]*>[\s\S]*?<\/code>/g);
  return out;
}

/** Latin prose — Cyrillic text, punctuation and identifiers are not what this guard is about. */
function looksLikeEnglishProse(text: string): boolean {
  return (
    /^[A-Za-z][A-Za-z .,'’…!?&+-]*$/.test(text) && /[A-Za-z]{3,}/.test(text)
  );
}

function lineOf(source: string, index: number): number {
  return source.slice(0, index).split("\n").length;
}

function findings(): string[] {
  const hits: string[] = [];
  for (const file of tsxFiles(uiRoot)) {
    const rel = relative(uiRoot, file).replace(/\\/g, "/");
    const source = scannable(readFileSync(file, "utf8"));

    // JSX text nodes, including the multi-line form produced by the formatter. The (?<!=) guard
    // skips arrow functions, whose "=>" would otherwise open a bogus text node over TS generics.
    for (const m of source.matchAll(/(?<!=)>([^<>{}]+)</g)) {
      const text = (m[1] ?? "").trim();
      if (!text || ALLOWED_TEXT.has(text) || !looksLikeEnglishProse(text)) {
        continue;
      }
      hits.push(`${rel}:${lineOf(source, m.index)} JSX text: ${text}`);
    }

    for (const attr of SCANNED_ATTRS) {
      for (const m of source.matchAll(
        new RegExp(`(?:^|\\s)${attr}="([^"]+)"`, "g"),
      )) {
        const value = m[1] ?? "";
        if (ALLOWED_ATTR_VALUES.has(value)) {
          continue;
        }
        hits.push(`${rel}:${lineOf(source, m.index)} ${attr}="${value}"`);
      }
    }
  }
  return hits.sort();
}

describe("no hardcoded user-visible strings", () => {
  it("routes JSX text and label attributes through t()", () => {
    expect(findings()).toEqual([]);
  });

  it("actually scans the SPA (guard against a broken file walk)", () => {
    expect(tsxFiles(uiRoot).length).toBeGreaterThan(30);
  });
});
