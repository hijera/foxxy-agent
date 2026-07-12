// Effective UI locale for the extension, driven by the backend config.
//
// The single language switcher for the whole application lives in the SPA
// (Settings → General) and persists `ui.locale` into the backend config.yaml.
// The extension has no language setting of its own: it fetches `ui.locale`
// once the server is ready, and receives live updates from the embedded SPA
// via the `{ type: "foxxycode:locale", locale }` webview message (see
// webview/panel.ts and external/ui/src/ui/embedLocaleBridge.ts).
//
// `null` means "auto": follow the host (VS Code display language).

import { httpGet } from "../util/http";
import type { Locale } from "./bundle";

let effectiveLocale: Locale | null = null;
let persist: ((locale: Locale | null) => void) | null = null;

/**
 * Seed the state from the globalState cache and register the persister.
 * Called once from activate() so pre-boot strings ("Starting FoxxyCode…")
 * match the last known backend choice.
 */
export function initLocaleState(
  cached: unknown,
  persistFn: (locale: Locale | null) => void,
): void {
  effectiveLocale = cached === "en" || cached === "ru" ? cached : null;
  persist = persistFn;
}

/** Set the backend-driven locale ("en"/"ru") or null for auto. */
export function setEffectiveLocale(locale: Locale | null): void {
  effectiveLocale = locale;
  persist?.(locale);
}

/** Resolve the locale to display: explicit backend choice, else host language. */
export function resolveLocale(envLanguage: string): Locale {
  if (effectiveLocale) {
    return effectiveLocale;
  }
  return (envLanguage || "en").toLowerCase().startsWith("ru") ? "ru" : "en";
}

/** Parse `ui.locale` out of a GET /foxxycode/config body; null = auto/invalid. */
export function localeFromConfigJson(text: string): Locale | null {
  try {
    const doc = JSON.parse(text) as { ui?: { locale?: unknown } };
    const raw = doc?.ui?.locale;
    return raw === "en" || raw === "ru" ? raw : null;
  } catch {
    return null;
  }
}

/**
 * Fetch `ui.locale` from the backend config and adopt it. Errors are
 * swallowed (state keeps its previous value) — locale must never block or
 * break the start flow.
 */
export async function fetchBackendLocale(baseUrl: string): Promise<void> {
  try {
    const res = await httpGet(baseUrl + "foxxycode/config", 3000);
    if (res.status < 200 || res.status > 299) {
      return;
    }
    setEffectiveLocale(localeFromConfigJson(res.body));
  } catch {
    // Backend not reachable / endpoint missing: keep the current state.
  }
}
