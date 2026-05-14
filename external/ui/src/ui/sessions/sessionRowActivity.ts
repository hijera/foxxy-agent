import type { SessionRow } from "./types";

export function sessionRowShowsSpinner(
  row: SessionRow,
  currentSessionId: string,
): boolean {
  return !!row.turnActive && row.id !== currentSessionId;
}

export function sessionRowShowsUnreadDot(
  row: SessionRow,
  currentSessionId: string,
): boolean {
  return !!row.unreadComplete && row.id !== currentSessionId;
}

