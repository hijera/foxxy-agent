/**
 * Live status line preference (ui.status_line), mirroring i18n/sendModeConfig.ts.
 *
 * When off, the typing dots render exactly as they did before the status line existed:
 * three animated dots, no label, no elapsed counter.
 *
 * The active value lives in a tiny module-level store so MessageList can subscribe via
 * useSyncExternalStore and update live, without threading a prop through ChatScreen. App
 * reads the config document once at startup and calls setStatusLineEnabled; the General
 * settings picker persists changes back to config.
 */

export const DEFAULT_STATUS_LINE = true;

function asUiObject(doc: Record<string, unknown>): Record<string, unknown> {
  const ui = doc.ui;
  if (ui && typeof ui === "object" && !Array.isArray(ui)) {
    return ui as Record<string, unknown>;
  }
  return {};
}

/**
 * Read ui.status_line from a config document. Only an explicit `false` turns the status
 * line off — a missing key must not disable the feature for every existing config.
 */
export function readStatusLineFromConfigDoc(
  doc: Record<string, unknown> | null | undefined,
): boolean {
  if (!doc) {
    return DEFAULT_STATUS_LINE;
  }
  return asUiObject(doc).status_line !== false;
}

let current: boolean = DEFAULT_STATUS_LINE;
const listeners = new Set<() => void>();

/** Current preference (used by useSyncExternalStore getSnapshot). */
export function getStatusLineEnabled(): boolean {
  return current;
}

/** Update the preference and notify subscribers. */
export function setStatusLineEnabled(enabled: boolean): void {
  if (enabled === current) {
    return;
  }
  current = enabled;
  for (const cb of listeners) {
    cb();
  }
}

/** Subscribe to preference changes (useSyncExternalStore subscribe). */
export function onStatusLineChange(cb: () => void): () => void {
  listeners.add(cb);
  return () => {
    listeners.delete(cb);
  };
}

/** Persist ui.status_line to config.yaml via PUT /foxxycode/config. */
export async function persistStatusLinePreference(
  enabled: boolean,
): Promise<void> {
  setStatusLineEnabled(enabled);
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
        status_line: enabled,
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
