/**
 * After window.confirm for session delete, the next pointer event can hit the
 * shell backdrop and close History. Ignore backdrop-driven drawer close briefly.
 */
export const SESSION_DELETE_POST_CONFIRM_BACKDROP_SUPPRESS_MS = 480;

export type MillisDeadlineRef = { current: number };

export function armSessionDeleteBackdropSuppressUntil(
  ref: MillisDeadlineRef,
): void {
  ref.current = Date.now() + SESSION_DELETE_POST_CONFIRM_BACKDROP_SUPPRESS_MS;
}

export function shouldSuppressShellBackdropClose(
  ref: MillisDeadlineRef,
  nowMs: number = Date.now(),
): boolean {
  return nowMs < ref.current;
}
