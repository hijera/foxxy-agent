import { existsSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import en from "../src/i18n/en.json";
import ru from "../src/i18n/ru.json";

type Json = string | number | boolean | null | Json[] | { [key: string]: Json };

const repo = resolve(__dirname, "../..");
const CYRILLIC = /[Ѐ-ӿ]/;
// Keys whose values are identifiers or shared names, the same in both languages.
const SHARED_KEYS = new Set(["chips", "id", "doc", "github", "site", "coddySite", "title"]);

function shape(value: Json): Json {
  if (Array.isArray(value)) return value.map(shape);
  if (value !== null && typeof value === "object") {
    return Object.fromEntries(Object.entries(value).map(([k, v]) => [k, shape(v)]));
  }
  return typeof value;
}

function strings(value: Json, path: string[] = []): { path: string; key: string; text: string }[] {
  if (typeof value === "string") return [{ path: path.join("."), key: path.at(-1) ?? "", text: value }];
  if (Array.isArray(value)) return value.flatMap((v, i) => strings(v, [...path, String(i)]));
  if (value !== null && typeof value === "object") {
    return Object.entries(value).flatMap(([k, v]) => strings(v, [...path, k]));
  }
  return [];
}

const slots = (s: string) => [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();

describe("site dictionaries", () => {
  it("have the same keys, arrays of the same length and the same {slots}", () => {
    expect(shape(ru as Json)).toEqual(shape(en as Json));
    const ruStrings = strings(ru as Json);
    strings(en as Json).forEach((entry, i) => {
      expect(slots(ruStrings[i]!.text), entry.path).toEqual(slots(entry.text));
    });
  });

  it("have no empty strings", () => {
    for (const { path, text } of [...strings(en as Json), ...strings(ru as Json)]) {
      expect(text.trim(), path).not.toBe("");
    }
  });

  it("keep English free of Cyrillic", () => {
    for (const { path, text } of strings(en as Json)) {
      expect(CYRILLIC.test(text), `en.${path}`).toBe(false);
    }
  });

  it("translate the Russian copy and use агентный, never the wrong adjective", () => {
    // .claude/rules/russian-wording.md; spelled with escapes so `git grep` for the word stays empty.
    const wrong = new RegExp(String.fromCharCode(0x430, 0x433, 0x435, 0x43d, 0x442, 0x441, 0x43a), "i");
    for (const { path, key, text } of strings(ru as Json)) {
      expect(wrong.test(text), `ru.${path}`).toBe(false);
      const isNameOrCode = SHARED_KEYS.has(key) || /^[\w./@`#:+ -]+$/.test(text) || /^\/[\w-]+$/.test(text);
      if (!isNameOrCode && text.length > 24) {
        expect(CYRILLIC.test(text), `ru.${path} looks untranslated: ${text}`).toBe(true);
      }
    }
  });

  it("link feature cards to files that exist", () => {
    for (const group of en.features.groups) {
      for (const item of group.items) {
        expect(existsSync(resolve(repo, item.doc)), item.doc).toBe(true);
      }
    }
    expect(ru.features.groups.flatMap((g) => g.items.map((i) => i.doc))).toEqual(
      en.features.groups.flatMap((g) => g.items.map((i) => i.doc)),
    );
  });
});
