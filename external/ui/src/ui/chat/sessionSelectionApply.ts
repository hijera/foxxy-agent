/**
 * Whether the composer should adopt the selection (Mode, Model, Reasoning) that
 * a `GET /foxxycode/sessions/{id}/messages` response carried.
 *
 * `loadMessages` is not only "open a session": it also runs after every turn,
 * after a permission answer, on relay reconnect and on background refresh. Each
 * response carries the session's stored selection, so re-applying it every time
 * reverts a pick the user made a moment earlier — the model snaps back to the
 * configured default while they are looking at it. The selection is therefore
 * adopted once per session open, and a session the user has already touched is
 * left alone entirely.
 *
 * A server-side switch does not come through here: it arrives as the SSE
 * `event: mode` frame, which is explicit and cannot be confused with a stale
 * snapshot.
 */
export function shouldApplySessionSelection(opts: {
  /** Session the snapshot belongs to. */
  stashSid: string;
  /** Session actually on screen right now. */
  viewedSid: string;
  /** Session whose selection was last applied, or "" when none was. */
  appliedSid: string;
  /** Session whose Mode/Model the user has changed by hand, or "". */
  userTouchedSid: string;
}): boolean {
  const stash = opts.stashSid.trim();
  if (!stash) {
    return false;
  }
  // A response that landed after the viewer moved on belongs to another chat.
  if (stash !== opts.viewedSid.trim()) {
    return false;
  }
  // Already restored once for this session: later reconciles are transcript
  // refreshes, not re-opens.
  if (stash === opts.appliedSid.trim()) {
    return false;
  }
  // The user picked something for this session before the first snapshot
  // landed; their choice outranks what the server had stored.
  if (stash === opts.userTouchedSid.trim()) {
    return false;
  }
  return true;
}

/**
 * Whether a mode the server reports after a turn should be adopted.
 *
 * Belt and braces for the `event: mode` frame: if the SPA missed it (a dropped
 * stream, a reconnect past the relay's replay window) the post-turn transcript
 * read still lines the composer up with the session. It defers to the user —
 * a Mode switched while the turn was running is a deliberate choice for the
 * *next* turn, so the server's value must not overwrite it.
 */
export function shouldAdoptServerModeAfterTurn(opts: {
  /** Mode the messages payload reported. */
  serverMode: string;
  /** Mode the composer is showing. */
  currentMode: string;
  /** Modes the composer accepts. */
  knownModes: readonly string[];
  /** User mode-edit counter when the turn started, and now. */
  editSeqAtTurnStart: number;
  editSeqNow: number;
}): boolean {
  const next = opts.serverMode.trim();
  if (!next || !opts.knownModes.includes(next)) {
    return false;
  }
  if (next === opts.currentMode.trim()) {
    return false;
  }
  return opts.editSeqAtTurnStart === opts.editSeqNow;
}
