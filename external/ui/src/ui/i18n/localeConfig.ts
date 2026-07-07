import { setLocale } from "./i18n";
import {
  mapSystemLocaleToSupported,
  readNavigatorLanguage,
  type UiLocale,
} from "./localeCookie";
import { applyUiLocale, readUiLocaleFromUrl } from "./uiLocale";

export type UiLocalePreference = "" | UiLocale;

function asUiObject(doc: Record<string, unknown>): Record<string, unknown> {
  const ui = doc.ui;
  if (ui && typeof ui === "object" && !Array.isArray(ui)) {
    return ui as Record<string, unknown>;
  }
  return {};
}

/** Read ui.locale from a config document ("" | "en" | "ru"). */
export function readUiLocaleFromConfigDoc(
  doc: Record<string, unknown> | null | undefined,
): UiLocalePreference {
  if (!doc) {
    return "";
  }
  const raw = asUiObject(doc).locale;
  if (raw === "en" || raw === "ru") {
    return raw;
  }
  return "";
}

/** Resolve stored preference to an active UiLocale. */
export function resolveEffectiveUiLocale(pref: UiLocalePreference): UiLocale {
  if (pref === "en" || pref === "ru") {
    return pref;
  }
  return mapSystemLocaleToSupported(readNavigatorLanguage());
}

/** Apply locale from config preference without persisting. */
export function applyUiLocalePreference(pref: UiLocalePreference): UiLocale {
  const effective = resolveEffectiveUiLocale(pref);
  setLocale(effective);
  return effective;
}

/**
 * Apply config ui.locale once at startup without stomping the bootstrap
 * choice: an explicit ?lang= in the URL wins over config, and an empty
 * ("auto") preference keeps the already-bootstrapped locale (cookie or
 * navigator.language) instead of re-resolving it.
 */
export function applyStartupUiLocaleFromConfig(pref: UiLocalePreference): void {
  if (readUiLocaleFromUrl() !== null) {
    return;
  }
  if (pref !== "en" && pref !== "ru") {
    return;
  }
  setLocale(pref);
}

/** Persist ui.locale to config.yaml via PUT /foxxycode/config. */
export async function persistUiLocalePreference(
  pref: UiLocalePreference,
): Promise<void> {
  applyUiLocalePreference(pref);
  try {
    const res = await fetch("/foxxycode/config");
    if (!res.ok) {
      return;
    }
    const doc = (await res.json()) as Record<string, unknown>;
    const next = {
      ...doc,
      ui: {
        ...asUiObject(doc),
        locale: pref,
      },
    };
    await fetch("/foxxycode/config", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(next),
    });
  } catch {
    // Best-effort persistence; cookie still holds the active locale.
  }
}

/** Apply document.lang for bootstrap before React mounts. */
export function bootstrapUiLocaleFromConfigDoc(
  doc: Record<string, unknown> | null | undefined,
): UiLocale {
  const pref = readUiLocaleFromConfigDoc(doc);
  const effective = resolveEffectiveUiLocale(pref);
  applyUiLocale(effective);
  return effective;
}
