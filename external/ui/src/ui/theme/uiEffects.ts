import { editorEmbedId } from "../embedShell";

// Visual effects level: <html data-effects="full|reduced">.
//
// "reduced" stops the infinite animations and turns the frosted-glass panels into
// solid ones (styles.css, guarded by reducedEffectsCss.test.ts). One setting for
// every client, switched in Appearance; only the default differs by host: reduced in
// the IntelliJ panel, where JCEF renders off-screen and copies every frame into the
// IDE on its UI thread, so an effect that repaints continuously slows the whole IDE -
// badly without a GPU - and full everywhere else.
//
// The choice lives in config.yaml (ui.effects, written by the switch). The cookie
// caches it for the inline bootstrap in index.html, which runs before the first
// paint and before any request; App re-applies the config value at startup
// (applyUiEffectsFromConfigDoc). JCEF keeps cookies in its persistent cache, and a
// cookie ignores the port, so the backend's random port does not lose it.

export const FOXXYCODE_UI_EFFECTS_COOKIE = "foxxycode_ui_effects";

export type UiEffects = "full" | "reduced";

const MAX_AGE_SECONDS = 365 * 24 * 60 * 60;

function isUiEffects(v: string | undefined | null): v is UiEffects {
  return v === "full" || v === "reduced";
}

/** The level a host starts with when the user has not chosen one. */
export function defaultUiEffects(embedId: string): UiEffects {
  return embedId === "intellij" ? "reduced" : "full";
}

export function readUiEffectsCookie(): UiEffects | null {
  if (typeof document === "undefined") {
    return null;
  }
  for (const part of document.cookie.split(";")) {
    const s = part.trim();
    if (!s.startsWith(`${FOXXYCODE_UI_EFFECTS_COOKIE}=`)) {
      continue;
    }
    const v = decodeURIComponent(s.slice(FOXXYCODE_UI_EFFECTS_COOKIE.length + 1).trim());
    return isUiEffects(v) ? v : null;
  }
  return null;
}

function writeUiEffectsCookie(v: UiEffects): void {
  if (typeof document === "undefined") {
    return;
  }
  const secure =
    typeof window !== "undefined" && window.location.protocol === "https:"
      ? "; Secure"
      : "";
  document.cookie = `${FOXXYCODE_UI_EFFECTS_COOKIE}=${encodeURIComponent(v)}; Path=/; Max-Age=${MAX_AGE_SECONDS}; SameSite=Lax${secure}`;
}

/** The stored choice, else the host's default. */
export function resolveUiEffects(): UiEffects {
  return readUiEffectsCookie() ?? defaultUiEffects(editorEmbedId());
}

export function applyUiEffects(v: UiEffects): void {
  if (typeof document === "undefined") {
    return;
  }
  document.documentElement.dataset.effects = v;
}

export function readAppliedUiEffects(): UiEffects {
  if (typeof document === "undefined") {
    return "full";
  }
  const v = document.documentElement.dataset.effects;
  return isUiEffects(v) ? v : resolveUiEffects();
}

/** Stores the user's choice and applies it at once. */
export function setUiEffects(v: UiEffects): void {
  writeUiEffectsCookie(v);
  applyUiEffects(v);
}

export function bootstrapUiEffects(): UiEffects {
  const v = resolveUiEffects();
  applyUiEffects(v);
  return v;
}

function asUiObject(doc: Record<string, unknown>): Record<string, unknown> {
  const ui = doc.ui;
  if (ui && typeof ui === "object" && !Array.isArray(ui)) {
    return ui as Record<string, unknown>;
  }
  return {};
}

/** ui.effects from a config document: true = full, false = reduced, else unset. */
export function readUiEffectsFromConfigDoc(
  doc: Record<string, unknown> | null | undefined,
): UiEffects | null {
  if (!doc) {
    return null;
  }
  const v = asUiObject(doc).effects;
  if (v === true) return "full";
  if (v === false) return "reduced";
  return null;
}

/**
 * Applies the stored config value at startup, in every client. config.yaml is where
 * the choice lives; the cookie only caches it so the next load's first paint is
 * already right. An unset key leaves the cached choice (or the host default) alone.
 */
export function applyUiEffectsFromConfigDoc(
  doc: Record<string, unknown> | null | undefined,
): void {
  const v = readUiEffectsFromConfigDoc(doc);
  if (v !== null) {
    setUiEffects(v);
  }
}

/** Persists the choice as ui.effects via GET + merge + PUT /foxxycode/config. */
export async function persistUiEffectsPreference(v: UiEffects): Promise<void> {
  try {
    const res = await fetch("/foxxycode/config");
    if (!res.ok) {
      return;
    }
    const doc = (await res.json()) as Record<string, unknown>;
    const next = {
      ...doc,
      ui: { ...asUiObject(doc), effects: v === "full" },
    };
    await fetch("/foxxycode/config", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(next),
    });
  } catch {
    // Best-effort: the cookie still holds the choice for this client.
  }
}

/** useSyncExternalStore subscription to the <html data-effects> marker. */
export function subscribeUiEffects(onStoreChange: () => void): () => void {
  const obs = new MutationObserver(onStoreChange);
  obs.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ["data-effects"],
  });
  return () => obs.disconnect();
}
