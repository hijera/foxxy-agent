const HDR = "X-FoxxyCode-Session-ID";

/**
 * POST a permission choice to the backend. Shared by the inline permission card
 * ({@link PermissionPromptSection}) and the desktop notification toast so both
 * resolve a prompt the same way. Network errors are swallowed by the caller —
 * the transcript is always unblocked locally regardless.
 */
export async function submitPermissionChoice(
  sessionId: string,
  toolCallId: string,
  optionId: string,
): Promise<void> {
  const sid = sessionId.trim();
  const tcid = toolCallId.trim();
  await fetch(`/foxxycode/sessions/${encodeURIComponent(sid)}/permission`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      [HDR]: sid,
    },
    body: JSON.stringify({ toolCallId: tcid, optionId }),
  });
}
