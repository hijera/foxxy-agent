/**
 * Composer send-mode preference (ui.send_mode), mirroring localeConfig.ts.
 *
 * Controls how the main chat composer submits a message:
 *  - "enter": plain Enter sends (Shift/Ctrl+Enter insert a newline)
 *  - "ctrl_enter": Ctrl/Cmd+Enter sends (plain Enter inserts a newline)
 *  - "off": keyboard send disabled (Send button only)
 *
 * The active value lives in a tiny module-level store so the Composer can
 * subscribe via useSyncExternalStore and update live, without threading a prop
 * through ChatScreen. App reads the config document once at startup and calls
 * setSendMode; the General settings picker persists changes back to config.
 */

export type SendMode = "enter" | "ctrl_enter" | "off";

export const DEFAULT_SEND_MODE: SendMode = "enter";

function coerceSendMode(raw: unknown): SendMode {
  if (raw === "enter" || raw === "ctrl_enter" || raw === "off") {
    return raw;
  }
  return DEFAULT_SEND_MODE;
}

function asUiObject(doc: Record<string, unknown>): Record<string, unknown> {
  const ui = doc.ui;
  if (ui && typeof ui === "object" && !Array.isArray(ui)) {
    return ui as Record<string, unknown>;
  }
  return {};
}

/** Read ui.send_mode from a config document (defaults to "enter"). */
export function readSendModeFromConfigDoc(
  doc: Record<string, unknown> | null | undefined,
): SendMode {
  if (!doc) {
    return DEFAULT_SEND_MODE;
  }
  return coerceSendMode(asUiObject(doc).send_mode);
}

let current: SendMode = DEFAULT_SEND_MODE;
const listeners = new Set<() => void>();

/** Current active send mode (used by useSyncExternalStore getSnapshot). */
export function getSendMode(): SendMode {
  return current;
}

/** Update the active send mode and notify subscribers. */
export function setSendMode(mode: SendMode): void {
  if (mode === current) {
    return;
  }
  current = mode;
  for (const cb of listeners) {
    cb();
  }
}

/** Subscribe to send-mode changes (useSyncExternalStore subscribe). */
export function onSendModeChange(cb: () => void): () => void {
  listeners.add(cb);
  return () => {
    listeners.delete(cb);
  };
}

/** Persist ui.send_mode to config.yaml via PUT /foxxycode/config. */
export async function persistSendModePreference(mode: SendMode): Promise<void> {
  setSendMode(mode);
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
        send_mode: mode,
      },
    };
    await fetch("/foxxycode/config", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(next),
    });
  } catch {
    // Best-effort persistence; the in-memory store still holds the active mode.
  }
}
