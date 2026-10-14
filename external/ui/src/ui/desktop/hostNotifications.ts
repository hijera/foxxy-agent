import { isEditorEmbed } from "../embedShell";
import {
  getNotificationPreferences,
  onNotificationPreferences,
} from "./notificationConfig";
import {
  notificationSoundData,
  playNotificationSound,
  stopNotificationSound,
} from "./desktopNotifySound";

export type HostNotificationPayload = {
  id: string;
  title: string;
  body: string;
  silent: boolean;
  whenFocused: boolean;
  soundData?: string;
  actions: { id: string; label: string }[];
};
export type HostNotification = Omit<
  HostNotificationPayload,
  "silent" | "whenFocused" | "soundData" | "actions"
> & {
  sessionId: string;
  actions: { id: string; label: string; run: () => void }[];
};
declare global {
  interface Window {
    foxxycodeHostIsActive?: () => Promise<boolean> | boolean;
    foxxycodeHostNotify?: (
      n: HostNotificationPayload,
    ) => Promise<boolean> | boolean;
    foxxycodeHostDismiss?: (id: string) => Promise<unknown> | void;
    /** Set by hosts that play the system alert sound natively (IntelliJ). */
    foxxycodeHostSystemSound?: boolean;
  }
}

const pending = new Map<string, { n: HostNotification; open: () => void }>();
// Retain handled IDs, including foreground events, across stream replays.
const delivered = new Set<string>();

function rememberNotification(id: string): void {
  delivered.add(id);
  if (delivered.size > 1024) delivered.delete(delivered.values().next().value!);
}

async function notificationWindowIsActive(): Promise<boolean> {
  try {
    // Native focus includes title bars and owned dialogs outside the webview.
    if (window.foxxycodeHostIsActive)
      return await window.foxxycodeHostIsActive();
  } catch {
    /* Use document focus when the host focus bridge is unavailable. */
  }
  return document.visibilityState === "visible" && document.hasFocus();
}

/** Leave room for Open chat and retain Reject when the permission list is longer. */
export function limitNotificationActions<T extends { id: string }>(
  actions: T[],
): T[] {
  const choices = actions.slice(0, 4);
  const reject = actions.find((a) => a.id === "reject");
  if (reject && !choices.some((a) => a.id === "reject")) choices[3] = reject;
  return choices;
}

onNotificationPreferences(() => {
  if (!getNotificationPreferences().enabled)
    for (const id of pending.keys()) dismissHostNotification(id);
  if (
    !getNotificationPreferences().enabled ||
    !getNotificationPreferences().sound
  )
    stopNotificationSound();
});

export function dismissHostNotification(id: string): void {
  pending.delete(id);
  try {
    const result = window.foxxycodeHostDismiss?.(id);
    if (result) void result.catch(() => {});
  } catch {
    /* Host may have closed. */
  }
  if (isEditorEmbed() && window.parent !== window)
    window.parent.postMessage(
      { type: "foxxycode:dismissNotification", id },
      "*",
    );
}

export function receiveNotificationAction(id: string, action: string): void {
  const entry = pending.get(id);
  if (!entry) return;
  const choice = entry.n.actions.find((a) => a.id === action);
  if (action && !choice) return;
  dismissHostNotification(id);
  if (choice) choice.run();
  else entry.open();
}

/** Returns true when delivered or suppressed. False allows an inline fallback. */
export async function showHostNotification(
  n: HostNotification,
  open: () => void,
): Promise<boolean> {
  if (
    !getNotificationPreferences().enabled ||
    delivered.has(n.id) ||
    pending.has(n.id)
  )
    return true;
  pending.set(n.id, { n, open });
  const initiallyActive = await notificationWindowIsActive();
  const prefs = getNotificationPreferences();
  if (!pending.has(n.id) || !prefs.enabled) {
    pending.delete(n.id);
    return true;
  }
  if (initiallyActive && !prefs.whenFocused) {
    pending.delete(n.id);
    rememberNotification(n.id);
    return true;
  }
  const payload: HostNotificationPayload = {
    id: n.id,
    title: n.title,
    body: n.body.slice(0, 500),
    silent: !prefs.sound,
    whenFocused: prefs.whenFocused,
    actions: n.actions.map(({ id, label }) => ({ id, label })),
  };
  if (
    prefs.sound &&
    prefs.soundFile &&
    window.foxxycodeHostPlaySound &&
    !isEditorEmbed()
  ) {
    try {
      payload.soundData = await notificationSoundData(prefs.soundFile);
    } catch {
      /* Use the standard system sound when the file is unavailable. */
    }
    const now = getNotificationPreferences();
    if (!pending.has(n.id) || !now.enabled) {
      pending.delete(n.id);
      return true;
    }
    payload.silent = !now.sound;
    payload.whenFocused = now.whenFocused;
    if (!now.sound || now.soundFile !== prefs.soundFile)
      delete payload.soundData;
  }
  let handled = false;
  try {
    if (window.foxxycodeHostNotify)
      handled = await window.foxxycodeHostNotify(payload);
    else if (isEditorEmbed() && window.parent !== window) {
      window.parent.postMessage(
        { type: "foxxycode:notification", notification: payload },
        "*",
      );
      handled = true;
    }
  } catch {
    /* Keep the inline prompt usable if the OS notification service fails. */
  }
  const active = await notificationWindowIsActive();
  const latest = getNotificationPreferences();
  if (!pending.has(n.id) || !latest.enabled) {
    dismissHostNotification(n.id);
    rememberNotification(n.id);
    return true;
  }
  if (handled) {
    rememberNotification(n.id);
    if (pending.size > 128)
      dismissHostNotification(pending.keys().next().value!);
    // Desktop uses Windows audio and IntelliJ balloons beep natively (a JCEF
    // audio context stays suspended until an in-page gesture); VS Code embeds
    // and custom clips still play in-page through Web Audio.
    const hostOwnsSystemSound =
      !!window.foxxycodeHostSystemSound && !latest.soundFile;
    if (
      latest.sound &&
      (!active || latest.whenFocused) &&
      isEditorEmbed() &&
      !hostOwnsSystemSound
    )
      playNotificationSound();
  } else {
    pending.delete(n.id);
    if (active && !latest.whenFocused) rememberNotification(n.id);
  }
  return handled || (active && !latest.whenFocused);
}

export function dismissSessionNotifications(sessionId: string): void {
  for (const [id, entry] of pending)
    if (entry.n.sessionId === sessionId) dismissHostNotification(id);
}
