/**
 * Composer mirror overlay segments: active **`/name`** draft at caret, completed **`@path`** mentions elsewhere.
 */

import type { ComposerSlashSegment } from "./segmentComposerSlashSpans";
import {
  atMenuDraftAtCaret,
  draftExtendsFailedAtPrefix,
  listAtPathSpans,
} from "./draftAt";
import {
  draftExtendsFailedSlashPrefix,
  slashMenuDraftAtCaret,
} from "./draftSlash";

export type ComposerMirrorSegment =
  | ComposerSlashSegment
  | { type: "at"; literal: string; pathRel: string };

/**
 * Chips every completed workspace **`@`** path; other characters stay plain (**`/`** skills only chip while caret is inside that slash draft).
 */
function segmentStaticAtOnly(text: string): ComposerMirrorSegment[] {
  if (text === "") {
    return [{ type: "text", value: "" }];
  }
  const spans = listAtPathSpans(text);
  const out: ComposerMirrorSegment[] = [];
  let p = 0;
  for (const sp of spans) {
    if (sp.start > p) {
      const gap = text.slice(p, sp.start);
      if (gap !== "") {
        out.push({ type: "text", value: gap });
      }
    }
    out.push({
      type: "at",
      literal: text.slice(sp.start, sp.end),
      pathRel: sp.path,
    });
    p = sp.end;
  }
  if (p < text.length) {
    const tail = text.slice(p);
    if (tail !== "") {
      out.push({ type: "text", value: tail });
    }
  }
  if (out.length === 0) {
    return [{ type: "text", value: "" }];
  }
  return out;
}

/**
 * Mirrors the textarea for display only. At-token chip takes precedence over slash when both could apply at the caret.
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

    const leftSegs =
      left === ""
        ? ([] as ComposerMirrorSegment[])
        : segmentStaticAtOnly(left);

    let midSeg: ComposerMirrorSegment[];
    if (atNoMatch != null && draftExtendsFailedAtPrefix(atDraft, atNoMatch)) {
      midSeg = [{ type: "text", value: mid }];
    } else {
      const expected = `@${prefix}`;
      midSeg =
        mid === expected
          ? [{ type: "at", literal: mid, pathRel: prefix }]
          : [{ type: "text", value: mid }];
    }

    const rightSegs =
      right === ""
        ? ([] as ComposerMirrorSegment[])
        : segmentStaticAtOnly(right);
    return [...leftSegs, ...midSeg, ...rightSegs];
  }

  const slashDraft = slashMenuDraftAtCaret(value, caret);
  if (!slashDraft.open) {
    return segmentStaticAtOnly(value);
  }

  const { slashIdx, prefix } = slashDraft;
  const tokenEnd = slashIdx + 1 + prefix.length;
  const left = value.slice(0, slashIdx);
  const mid = value.slice(slashIdx, tokenEnd);
  const right = value.slice(tokenEnd);

  const leftSegs =
    left === "" ? ([] as ComposerMirrorSegment[]) : segmentStaticAtOnly(left);

  let midSeg: ComposerMirrorSegment[];
  if (
    slashNoMatch != null &&
    draftExtendsFailedSlashPrefix(slashDraft, slashNoMatch)
  ) {
    midSeg = [{ type: "text", value: mid }];
  } else {
    const expected = `/${prefix}`;
    midSeg =
      mid === expected
        ? [{ type: "slash", literal: mid, name: prefix }]
        : [{ type: "text", value: mid }];
  }

  const rightSegs =
    right === ""
      ? ([] as ComposerMirrorSegment[])
      : segmentStaticAtOnly(right);
  return [...leftSegs, ...midSeg, ...rightSegs];
}
