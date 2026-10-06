import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { afterEach, expect, test } from "vitest";
import { FOXXYCODE_UI_EFFECTS_COOKIE } from "./uiEffects";

// Runs the inline bootstrap of index.html itself - the code that decides the effects level
// before the first paint, before React or any request - for each way the page is opened.

const html = readFileSync(
  join(dirname(fileURLToPath(import.meta.url)), "../../index.html"),
  "utf8",
);
const inline = /<script>([\s\S]*?)<\/script>/.exec(html);

function boot(search: string): string | undefined {
  expect(inline, "index.html lost its inline bootstrap script").toBeTruthy();
  window.history.replaceState({}, "", `/${search}`);
  // eslint-disable-next-line @typescript-eslint/no-implied-eval
  new Function(inline![1]!)();
  return document.documentElement.dataset.effects;
}

afterEach(() => {
  document.cookie = `${FOXXYCODE_UI_EFFECTS_COOKIE}=; Path=/; Max-Age=0`;
  document.documentElement.removeAttribute("data-effects");
  delete document.documentElement.dataset.embed;
  window.history.replaceState({}, "", "/");
});

test("the IntelliJ panel starts with reduced effects", () => {
  expect(boot("?theme=light&embed=intellij&lang=ru")).toBe("reduced");
});

test("a browser tab and the VS Code panel start with full effects", () => {
  expect(boot("")).toBe("full");
  document.documentElement.removeAttribute("data-effects");
  expect(boot("?embed=vscode")).toBe("full");
});

test("a stored choice wins over the host default, both ways", () => {
  document.cookie = `${FOXXYCODE_UI_EFFECTS_COOKIE}=full; Path=/`;
  expect(boot("?embed=intellij")).toBe("full");
  document.cookie = `${FOXXYCODE_UI_EFFECTS_COOKIE}=reduced; Path=/`;
  expect(boot("")).toBe("reduced");
});

test("an unknown stored value falls back to the host default", () => {
  document.cookie = `${FOXXYCODE_UI_EFFECTS_COOKIE}=sparkles; Path=/`;
  expect(boot("?embed=intellij")).toBe("reduced");
  expect(boot("")).toBe("full");
});
