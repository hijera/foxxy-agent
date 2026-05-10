/**
 * Composer mirror overlay segments: the active slash **`/name`** or **`@path`** draft at the caret becomes a styled chip.
 */

import type { ComposerSlashSegment } from "./segmentComposerSlashSpans";
import { atMenuDraftAtCaret, draftExtendsFailedAtPrefix } from "./draftAt";
import {
  draftExtendsFailedSlashPrefix,
  slashMenuDraftAtCaret,
} from "./draftSlash";

export type ComposerMirrorSegment =
  | ComposerSlashSegment
  | { type: "at"; literal: string; pathRel: string };

/**
 * Mirrors the textarea for display only. At-token chip takes precedence over slash when both could apply.
 */
export function segmentComposerMirrorSpans(
  value: string,
  caret: number,
  slashNoMatch: { slashIdx: number; prefix: string } | null,
  atNoMatch: { atIdx: number; prefix: string } | null,
): ComposerMirrorSegment[] {
  const atDraft = atMenuDraftAtCaret(value, caret);
  if (atDraft.open) {
    const { atIdx, prefix } = atDraft;
    const tokenEnd = atIdx + 1 + prefix.length;
    const left = value.slice(0, atIdx);
    const mid = value.slice(atIdx, tokenEnd);
    const right = value.slice(tokenEnd);

    const out: ComposerMirrorSegment[] = [];
    if (left !== "") {
      out.push({ type: "text", value: left });
    }

    if (atNoMatch != null && draftExtendsFailedAtPrefix(atDraft, atNoMatch)) {
      out.push({ type: "text", value: mid });
    } else {
      const expected = `@${prefix}`;
      if (mid === expected) {
        out.push({ type: "at", literal: mid, pathRel: prefix });
      } else {
        out.push({ type: "text", value: mid });
      }
    }

    if (right !== "") {
      out.push({ type: "text", value: right });
    }
    if (out.length === 0) {
      return [{ type: "text", value: "" }];
    }
    return out;
  }

  const draft = slashMenuDraftAtCaret(value, caret);
  if (!draft.open) {
    return [{ type: "text", value }];
  }

  const { slashIdx, prefix } = draft;
  const tokenEnd = slashIdx + 1 + prefix.length;
  const left = value.slice(0, slashIdx);
  const mid = value.slice(slashIdx, tokenEnd);
  const right = value.slice(tokenEnd);

  const out: ComposerMirrorSegment[] = [];
  if (left !== "") {
    out.push({ type: "text", value: left });
  }

  if (
    slashNoMatch != null &&
    draftExtendsFailedSlashPrefix(draft, slashNoMatch)
  ) {
    out.push({ type: "text", value: mid });
  } else {
    const expected = `/${prefix}`;
    if (mid === expected) {
      out.push({ type: "slash", literal: mid, name: prefix });
    } else {
      out.push({ type: "text", value: mid });
    }
  }

  if (right !== "") {
    out.push({ type: "text", value: right });
  }
  if (out.length === 0) {
    return [{ type: "text", value: "" }];
  }
  return out;
}
