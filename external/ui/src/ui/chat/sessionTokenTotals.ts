/**
 * What a `GET /foxxycode/sessions/{id}/stats` payload is allowed to change while a turn
 * is streaming.
 *
 * The two numbers in that payload have different lifetimes, and treating them the same is
 * what made the token counter climb on its own:
 *
 * - `tokenUsageTotal` is the session's cumulative provider usage. The SPA uses it as the
 *   baseline that the turn's live `token_usage` deltas are added to. Reseeding it mid-turn
 *   from a document that already counts the running turn makes the next delta land on top
 *   of itself, and the stats poll runs several times a second, so the error compounds.
 * - `contextBreakdown` is a live estimate of the current model window. It is never
 *   additive, it is the only thing that reports compaction shrinking the context, and it
 *   must be applied whenever it arrives.
 */

import type { ContextBreakdownLike } from "./contextUsage";
import type { TokenUsage } from "./types";

/**
 * Generic in the breakdown so the caller's own (fully populated) shape survives the round
 * trip instead of being widened to the all-optional ContextBreakdownLike.
 */
export type SessionStatsPayload<B extends ContextBreakdownLike = ContextBreakdownLike> = {
  tokenUsageTotal?: {
    inputTokens: number;
    outputTokens: number;
    totalTokens: number;
  };
  contextBreakdown?: B;
};

export type SessionStatsApply<B extends ContextBreakdownLike = ContextBreakdownLike> = {
  /** Totals to publish and reseed the SSE baseline with; null leaves both untouched. */
  tokenUsage: TokenUsage | null;
  /** Breakdown to publish; null only when the payload carries none. */
  contextBreakdown: B | null;
};

export function planSessionStatsApply<B extends ContextBreakdownLike>(
  stats: SessionStatsPayload<B> | null | undefined,
  opts: { liveStreamAttached: boolean },
): SessionStatsApply<B> {
  const breakdown = stats?.contextBreakdown ?? null;
  const totals = stats?.tokenUsageTotal;
  if (!totals || opts.liveStreamAttached) {
    return { tokenUsage: null, contextBreakdown: breakdown };
  }
  return {
    tokenUsage: {
      inputTokens: Math.max(0, totals.inputTokens || 0),
      outputTokens: Math.max(0, totals.outputTokens || 0),
      totalTokens: Math.max(0, totals.totalTokens || 0),
    },
    contextBreakdown: breakdown,
  };
}
