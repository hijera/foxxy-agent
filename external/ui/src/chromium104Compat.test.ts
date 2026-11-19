import { spawnSync } from "node:child_process";
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { afterEach, expect, test } from "vitest";

const checker = path.resolve(import.meta.dirname, "../scripts-check-chromium104.mjs");
const fixtures: string[] = [];

afterEach(() => {
  for (const dir of fixtures.splice(0)) rmSync(dir, { recursive: true, force: true });
});

function checkChunk(code: string, name = "renderer-hash.js") {
  const dir = mkdtempSync(path.join(tmpdir(), "jcef-compat-"));
  fixtures.push(dir);
  mkdirSync(path.join(dir, "chunks/nested"), { recursive: true });
  for (const file of ["styles.css", "app.js", "events-worker.js"]) {
    writeFileSync(path.join(dir, file), "");
  }
  writeFileSync(path.join(dir, "chunks/nested", name), code);
  return spawnSync(process.execPath, [checker, dir], { encoding: "utf8" });
}

test.each(["Object.groupBy(items, key)", "URL.canParse(url)"])(
  "rejects unsupported APIs in lazy dependency chunks: %s",
  (code) => {
    const result = checkChunk(code);
    expect(result.error).toBeUndefined();
    expect(result.status).toBe(1);
    expect(result.stderr).toContain(path.join("chunks", "nested", "renderer-hash.js"));
    expect(result.stderr).toContain(code.split("(")[0]);
  },
);

test("accepts Chromium 104 code and ignores source maps", () => {
  expect(checkChunk("new URL(url)").status).toBe(0);
  expect(checkChunk("Object.groupBy(items, key)", "renderer.js.map").status).toBe(0);
});
