export type NotificationPreferences = {
  enabled: boolean;
  sound: boolean;
  whenFocused: boolean;
  soundFile: string;
};
let current: NotificationPreferences = {
  enabled: true,
  sound: true,
  whenFocused: false,
  soundFile: "",
};
const listeners = new Set<() => void>();

export function readNotificationConfig(
  doc: Record<string, unknown>,
): NotificationPreferences {
  const ui = (doc.ui ?? {}) as Record<string, unknown>;
  return {
    enabled: ui.notifications !== false,
    sound: ui.notification_sound !== false,
    whenFocused: ui.notifications_when_focused === true,
    soundFile:
      typeof ui.notification_sound_file === "string"
        ? ui.notification_sound_file
        : "",
  };
}
export function applyNotificationConfig(doc: Record<string, unknown>): void {
  current = readNotificationConfig(doc);
  for (const cb of listeners) cb();
}
export const getNotificationPreferences = () => current;
export function onNotificationPreferences(cb: () => void): () => void {
  listeners.add(cb);
  return () => {
    listeners.delete(cb);
  };
}
