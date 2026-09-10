/**
 * Changed-files card preference (ui.session_changes), mirroring statusLineConfig.ts.
 *
 * When off, the card under the transcript is not rendered and the session change set
 * is never fetched. The Changes button in the IntelliJ and VS Code plugins is a
 * separate surface and is deliberately not affected: it is part of the IDE chrome,
 * not of the SPA transcript.
 *
 * The active value lives in a module-level store so the card can subscribe via
 * useSyncExternalStore and update live, without threading a prop through ChatScreen.
 * App reads the config document once at startup and calls setSessionChangesEnabled;
 * the General settings picker persists changes back to config.
 */

export const DEFAULT_SESSION_CHANGES = true;

function asUiObject(doc: Record<string, unknown>): Record<string, unknown> {
  const ui = doc.ui;
  if (ui && typeof ui === "object" && !Array.isArray(ui)) {
    return ui as Record<string, unknown>;
  }
  return {};
}

/**
 * Read ui.session_changes from a config document. Only an explicit `false` hides the
 * card — a missing key must not disable the feature for every existing config.
 */
export function readSessionChangesFromConfigDoc(
  doc: Record<string, unknown> | null | undefined,
): boolean {
  if (!doc) {
    return DEFAULT_SESSION_CHANGES;
  }
  return asUiObject(doc).session_changes !== false;
}

let current: boolean = DEFAULT_SESSION_CHANGES;
const listeners = new Set<() => void>();

/** Current preference (used by useSyncExternalStore getSnapshot). */
export function getSessionChangesEnabled(): boolean {
  return current;
}

/** Update the preference and notify subscribers. */
export function setSessionChangesEnabled(enabled: boolean): void {
  if (enabled === current) {
    return;
  }
  current = enabled;
  for (const cb of listeners) {
    cb();
  }
}

/** Subscribe to preference changes (useSyncExternalStore subscribe). */
export function onSessionChangesChange(cb: () => void): () => void {
  listeners.add(cb);
  return () => {
    listeners.delete(cb);
  };
}

/** Persist ui.session_changes to config.yaml via PUT /foxxycode/config. */
export async function persistSessionChangesPreference(
  enabled: boolean,
): Promise<void> {
  setSessionChangesEnabled(enabled);
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
        session_changes: enabled,
      },
    };
    await fetch("/foxxycode/config", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(next),
    });
  } catch {
    // Best-effort persistence; the in-memory store still holds the active value.
  }
}
