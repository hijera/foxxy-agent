// @vitest-environment node
import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import { DEMO_SURFACES, pickShot, shots, type Shot } from "../src/app/demos";

const dir = resolve(__dirname, "../public/screenshots");

// A WebP file starts with RIFF....WEBP, then a VP8/VP8L/VP8X chunk that carries the canvas size.
function webpSize(path: string): [number, number] {
  const b = readFileSync(path);
  expect(b.toString("ascii", 0, 4) + b.toString("ascii", 8, 12)).toBe("RIFFWEBP");
  const chunk = b.toString("ascii", 12, 16);
  if (chunk === "VP8X") return [1 + b.readUIntLE(24, 3), 1 + b.readUIntLE(27, 3)];
  if (chunk === "VP8L") {
    const bits = b.readUInt32LE(21);
    return [1 + (bits & 0x3fff), 1 + ((bits >> 14) & 0x3fff)];
  }
  return [b.readUInt16LE(26) & 0x3fff, b.readUInt16LE(28) & 0x3fff];
}

describe("demo screenshots", () => {
  const listed = DEMO_SURFACES.flatMap((s) => shots[s]);

  it("point at files that exist, with their real size, and nothing unlisted ships", () => {
    for (const shot of listed) {
      const path = resolve(dir, shot.file);
      expect(existsSync(path), shot.file).toBe(true);
      expect(webpSize(path), shot.file).toEqual([shot.width, shot.height]);
      expect(statSync(path).size, `${shot.file} is heavier than 400 KB`).toBeLessThan(400 * 1024);
    }
    expect(readdirSync(dir).sort()).toEqual(listed.map((s) => s.file).sort());
  });

  it("cover every surface in both languages", () => {
    for (const surface of DEMO_SURFACES) {
      const langs = new Set(shots[surface].map((s) => s.lang));
      expect([...langs].sort(), surface).toEqual(["en", "ru"]);
    }
  });

  it("fall back to the same language in the other theme before another language", () => {
    const list: Shot[] = [
      { file: "a", theme: "dark", lang: "en", width: 1, height: 1 },
      { file: "b", theme: "dark", lang: "ru", width: 1, height: 1 },
    ];
    expect(pickShot(list, "light", "ru")?.file).toBe("b");
    expect(pickShot(list, "dark", "en")?.file).toBe("a");
    expect(pickShot([], "dark", "en")).toBeNull();
  });
});
