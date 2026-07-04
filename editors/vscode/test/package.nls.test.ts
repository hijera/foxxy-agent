import { describe, it, expect } from "vitest";
import * as fs from "fs";
import * as path from "path";

const root = path.resolve(__dirname, "..");

function loadJson(name: string): Record<string, string> {
  return JSON.parse(fs.readFileSync(path.join(root, name), "utf8")) as Record<string, string>;
}

function collectNlsKeysFromPackage(): string[] {
  const pkg = JSON.parse(fs.readFileSync(path.join(root, "package.json"), "utf8")) as {
    displayName?: string;
    description?: string;
    contributes?: unknown;
  };
  const keys = new Set<string>();
  const re = /%([a-zA-Z0-9_.]+)%/g;

  const walk = (value: unknown): void => {
    if (typeof value === "string") {
      let m: RegExpExecArray | null;
      while ((m = re.exec(value)) !== null) {
        keys.add(m[1]);
      }
      return;
    }
    if (Array.isArray(value)) {
      for (const item of value) walk(item);
      return;
    }
    if (value && typeof value === "object") {
      for (const v of Object.values(value as Record<string, unknown>)) walk(v);
    }
  };

  walk(pkg.displayName);
  walk(pkg.description);
  walk(pkg.contributes);
  return [...keys].sort();
}

describe("package.nls.json parity", () => {
  const en = loadJson("package.nls.json");
  const ru = loadJson("package.nls.ru.json");

  it("has identical key sets in en and ru bundles", () => {
    expect(Object.keys(ru).sort()).toEqual(Object.keys(en).sort());
  });

  it("defines every %key% referenced from package.json", () => {
    const referenced = collectNlsKeysFromPackage();
    for (const key of referenced) {
      expect(en[key], `missing en key ${key}`).toBeTruthy();
      expect(ru[key], `missing ru key ${key}`).toBeTruthy();
    }
  });

  it("includes Russian command title variants for dual-locale commands", () => {
    for (const base of [
      "cmd.openPanel.title",
      "cmd.restart.title",
      "cmd.reload.title",
      "cmd.openInBrowser.title",
      "cmd.openDevtools.title",
      "cmd.settings.title",
      "cmd.showLogs.title",
    ]) {
      expect(en[`${base}.ru`], `missing ${base}.ru in en`).toBeTruthy();
      expect(ru[`${base}.ru`], `missing ${base}.ru in ru`).toBeTruthy();
    }
  });
});
