import { afterEach, expect, test, vi } from "vitest";
import {
  FOXXYCODE_UI_EFFECTS_COOKIE,
  applyUiEffects,
  applyUiEffectsFromConfigDoc,
  persistUiEffectsPreference,
  readUiEffectsFromConfigDoc,
  bootstrapUiEffects,
  defaultUiEffects,
  readAppliedUiEffects,
  readUiEffectsCookie,
  resolveUiEffects,
  setUiEffects,
} from "./uiEffects";

function setEmbed(id: string | null): void {
  if (id === null) {
    delete document.documentElement.dataset.embed;
  } else {
    document.documentElement.dataset.embed = id;
  }
}

afterEach(() => {
  document.cookie = `${FOXXYCODE_UI_EFFECTS_COOKIE}=; Max-Age=0; Path=/`;
  document.documentElement.removeAttribute("data-effects");
  setEmbed(null);
  window.sessionStorage.clear();
});

// The IntelliJ panel renders off-screen and copies every frame into the IDE, so it
// starts without the effects; every other host keeps them.
test("the IntelliJ panel defaults to reduced effects, every other host to full", () => {
  expect(defaultUiEffects("intellij")).toBe("reduced");
  expect(defaultUiEffects("vscode")).toBe("full");
  expect(defaultUiEffects("")).toBe("full");
});

test("without a stored choice the host decides", () => {
  setEmbed("intellij");
  expect(resolveUiEffects()).toBe("reduced");
  setEmbed("vscode");
  expect(resolveUiEffects()).toBe("full");
  setEmbed(null);
  expect(resolveUiEffects()).toBe("full");
});

test("a stored choice wins over the host default, both ways", () => {
  setEmbed("intellij");
  document.cookie = `${FOXXYCODE_UI_EFFECTS_COOKIE}=full; Path=/`;
  expect(resolveUiEffects()).toBe("full");

  setEmbed(null);
  document.cookie = `${FOXXYCODE_UI_EFFECTS_COOKIE}=reduced; Path=/`;
  expect(resolveUiEffects()).toBe("reduced");
});

test("an unknown stored value is ignored", () => {
  document.cookie = `${FOXXYCODE_UI_EFFECTS_COOKIE}=sparkles; Path=/`;
  expect(readUiEffectsCookie()).toBeNull();
  setEmbed("intellij");
  expect(resolveUiEffects()).toBe("reduced");
});

test("applyUiEffects marks <html> and readAppliedUiEffects reads it back", () => {
  applyUiEffects("reduced");
  expect(document.documentElement.dataset.effects).toBe("reduced");
  expect(readAppliedUiEffects()).toBe("reduced");
  applyUiEffects("full");
  expect(document.documentElement.dataset.effects).toBe("full");
  expect(readAppliedUiEffects()).toBe("full");
});

test("setUiEffects stores the choice and applies it at once", () => {
  setEmbed("intellij");
  setUiEffects("full");
  expect(document.documentElement.dataset.effects).toBe("full");
  expect(readUiEffectsCookie()).toBe("full");
  expect(document.cookie).toContain(`${FOXXYCODE_UI_EFFECTS_COOKIE}=full`);
});

// config.yaml (ui.effects) is where the choice lives, shared by every client; the
// cookie only caches it so the first paint of the next load is already right.
test("ui.effects reads as full, reduced or unset", () => {
  expect(readUiEffectsFromConfigDoc({ ui: { effects: true } })).toBe("full");
  expect(readUiEffectsFromConfigDoc({ ui: { effects: false } })).toBe("reduced");
  expect(readUiEffectsFromConfigDoc({ ui: {} })).toBeNull();
  expect(readUiEffectsFromConfigDoc({})).toBeNull();
  expect(readUiEffectsFromConfigDoc(null)).toBeNull();
});

test("the config value wins over the cookie and is cached in it", () => {
  setEmbed("intellij");
  document.cookie = `${FOXXYCODE_UI_EFFECTS_COOKIE}=reduced; Path=/`;
  applyUiEffectsFromConfigDoc({ ui: { effects: true } });
  expect(document.documentElement.dataset.effects).toBe("full");
  expect(readUiEffectsCookie()).toBe("full");
});

test("an unset config value leaves the cached choice alone", () => {
  setEmbed("intellij");
  document.cookie = `${FOXXYCODE_UI_EFFECTS_COOKIE}=full; Path=/`;
  applyUiEffects("full");
  applyUiEffectsFromConfigDoc({ ui: {} });
  expect(document.documentElement.dataset.effects).toBe("full");
});

// One setting for every client: turning the effects off anywhere turns them off in
// the browser and VS Code too; only the unset default differs per host.
test("the config value applies in every client, not only the IntelliJ panel", () => {
  setEmbed("vscode");
  applyUiEffects("full");
  applyUiEffectsFromConfigDoc({ ui: { effects: false } });
  expect(document.documentElement.dataset.effects).toBe("reduced");
  expect(readUiEffectsCookie()).toBe("reduced");

  setEmbed(null);
  applyUiEffectsFromConfigDoc({ ui: { effects: true } });
  expect(document.documentElement.dataset.effects).toBe("full");
});

test("persistUiEffectsPreference merges ui.effects into config.yaml", async () => {
  const calls: Array<{ url: string; init: RequestInit | undefined }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      calls.push({ url, init });
      if (!init || init.method !== "PUT") {
        return new Response(JSON.stringify({ ui: { locale: "ru" }, agent: { model: "m" } }));
      }
      return new Response("{}");
    }),
  );
  await persistUiEffectsPreference("full");
  const put = calls.find((c) => c.init?.method === "PUT");
  expect(put?.url).toBe("/foxxycode/config");
  const body = JSON.parse(String(put!.init!.body));
  expect(body.ui).toEqual({ locale: "ru", effects: true });
  expect(body.agent).toEqual({ model: "m" });
  vi.unstubAllGlobals();
});

test("bootstrapUiEffects applies the resolved value", () => {
  setEmbed("intellij");
  expect(bootstrapUiEffects()).toBe("reduced");
  expect(document.documentElement.dataset.effects).toBe("reduced");
});
