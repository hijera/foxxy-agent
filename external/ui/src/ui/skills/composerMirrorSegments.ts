/**
 * Composer mirror overlay segments: active **`/name`** draft at caret, completed **`@path`** mentions elsewhere.
 */

import type { ComposerSlashSegment } from "./segmentComposerSlashSpans";
import { segmentSlashKnownSpans } from "./segmentComposerSlashSpans";
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
 * Chips completed workspace **`@`** paths and, when **`knownSlashNames`** is provided,
 * chips **`/name`** tokens whose name appears in the set.
 */
function segmentStaticAtAndSlash(
  text: string,
  knownSlashNames?: Set<string>,
): ComposerMirrorSegment[] {
  if (text === "") {
    return [{ type: "text", value: "" }];
  }
  const atSpans = listAtPathSpans(text);
  const out: ComposerMirrorSegment[] = [];
  let p = 0;

  for (const sp of atSpans) {
    if (sp.start > p) {
      const gap = text.slice(p, sp.start);
      if (gap !== "") {
        appendSlashChipsOrText(out, gap, knownSlashNames);
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
      appendSlashChipsOrText(out, tail, knownSlashNames);
    }
  }
  if (out.length === 0) {
    return [{ type: "text", value: "" }];
  }
  return out;
}

/** Appends slash chips (for known names) or plain text segments for a text region. */
function appendSlashChipsOrText(
  out: ComposerMirrorSegment[],
  text: string,
  knownSlashNames?: Set<string>,
): void {
  if (!knownSlashNames || knownSlashNames.size === 0) {
    out.push({ type: "text", value: text });
    return;
  }
  const segs = segmentSlashKnownSpans(text, knownSlashNames);
  for (const seg of segs) {
    out.push(seg as ComposerMirrorSegment);
  }
}

/**
 * Mirrors the textarea for display only. At-token chip takes precedence over slash when both could apply at the caret.
 * Pass **`knownSlashNames`** to chip completed **`/name`** tokens whose name is in the set (skills confirmed from API).
 */
export function segmentComposerMirrorSpans(
  value: string,
  caret: number,
  slashNoMatch: { slashIdx: number; prefix: string } | null,
  atNoMatch: { atIdx: number; prefix: string } | null,
  knownSlashNames?: Set<string>,
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
        : segmentStaticAtAndSlash(left, knownSlashNames);

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
        : segmentStaticAtAndSlash(right, knownSlashNames);
    return [...leftSegs, ...midSeg, ...rightSegs];
  }

  const slashDraft = slashMenuDraftAtCaret(value, caret);
  if (!slashDraft.open) {
    return segmentStaticAtAndSlash(value, knownSlashNames);
  }

  const { slashIdx, prefix } = slashDraft;
  const tokenEnd = slashIdx + 1 + prefix.length;
  const left = value.slice(0, slashIdx);
  const mid = value.slice(slashIdx, tokenEnd);
  const right = value.slice(tokenEnd);

  const leftSegs =
    left === ""
      ? ([] as ComposerMirrorSegment[])
      : segmentStaticAtAndSlash(left, knownSlashNames);

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
      : segmentStaticAtAndSlash(right, knownSlashNames);
  return [...leftSegs, ...midSeg, ...rightSegs];
}
