export type ChatNotification = {
  id: string;
  title: string;
  body: string;
  silent: boolean;
  actions: { id: string; label: string }[];
};

/** The bridge accepts text and opaque action IDs, never commands or URLs. */
export function chatNotificationFromMessage(
  message: unknown,
): ChatNotification | null {
  if (!message || typeof message !== "object") return null;
  const msg = message as { type?: unknown; notification?: unknown };
  if (
    msg.type !== "foxxycode:notification" ||
    !msg.notification ||
    typeof msg.notification !== "object"
  )
    return null;
  const n = msg.notification as ChatNotification;
  if (
    typeof n.id !== "string" ||
    !n.id ||
    n.id.length > 256 ||
    typeof n.title !== "string" ||
    typeof n.body !== "string" ||
    !Array.isArray(n.actions) ||
    n.actions.length > 5
  )
    return null;
  if (
    n.actions.some(
      (a) =>
        !a || typeof a.id !== "string" || !a.id || typeof a.label !== "string",
    )
  )
    return null;
  if (new Set(n.actions.map((a) => a.id)).size !== n.actions.length)
    return null;
  return {
    id: n.id,
    title: n.title.slice(0, 256),
    body: n.body.slice(0, 500),
    silent: n.silent === true,
    actions: n.actions.map((a) => ({ id: a.id, label: a.label.slice(0, 120) })),
  };
}
