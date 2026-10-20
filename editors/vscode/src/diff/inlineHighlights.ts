import {
  computeLineFragments,
  editorLines,
  LineFragment,
  mapFragmentsToDocument,
  sameLines,
} from "./lineFragments";

export interface HighlightSource {
  before: string;
  after: string;
  useAfterRanges: boolean;
}

interface Entry {
  token: number;
  /** Fragments in the coordinates of `targetLines`. */
  fragments: LineFragment[];
  /** The text the fragments describe: `after` for an applied edit, `before` for a proposal. */
  targetLines: string[];
  /** Placed on a document that holds the target text (or never will); from
   *  here on the editor moves the decorations with the user's own edits. */
  settled: boolean;
}

/** Decides which lines of an open document carry the highlights of the latest
 *  agent edit to each file. Pure: keys are normalized paths, texts are passed in.
 *
 *  An edit event carries whole-file snapshots, but the editor does not
 *  necessarily hold the `after` snapshot yet: a file that is already open is
 *  re-read only when VS Code's file watcher notices the write, and the event
 *  stream usually wins that race. Line numbers drawn on the stale text are
 *  then pushed down by the reload itself (it inserts the new lines at the
 *  point where the highlight starts), so the second edit of a file lit up the
 *  lines below its insertion. The tracker therefore places fragments on the
 *  text the document really has and asks to be redrawn on every change until
 *  the document catches up. */
export class InlineHighlightTracker {
  private readonly entries = new Map<string, Entry>();
  private seq = 0;

  /** Registers the newest edit event for a file and returns its token; an
   *  earlier event for the same file stops being drawn. */
  begin(key: string, src: HighlightSource): number {
    const token = ++this.seq;
    this.entries.set(key, {
      token,
      fragments: computeLineFragments(src.before, src.after, src.useAfterRanges),
      targetLines: editorLines(src.useAfterRanges ? src.after : src.before),
      settled: false,
    });
    return token;
  }

  /** Lines to highlight for event `token` in a document holding `docText`, or
   *  null when a newer event for the file replaced it and nothing may be drawn. */
  place(key: string, token: number, docText: string): LineFragment[] | null {
    const entry = this.entries.get(key);
    if (!entry || entry.token !== token) return null;
    return this.placeEntry(entry, docText, false);
  }

  /** The document changed. Returns the lines to redraw, or null when the
   *  current highlights already stand: no event, or one that settled. `final`
   *  says no reload is coming (the document has unsaved changes), so this
   *  placement is the last one. */
  documentChanged(key: string, docText: string, final: boolean): LineFragment[] | null {
    const entry = this.entries.get(key);
    if (!entry || entry.settled) return null;
    return this.placeEntry(entry, docText, final);
  }

  forget(key: string): void {
    this.entries.delete(key);
  }

  forgetAll(): void {
    this.entries.clear();
  }

  private placeEntry(entry: Entry, docText: string, final: boolean): LineFragment[] {
    const docLines = editorLines(docText);
    if (final || sameLines(entry.targetLines, docLines)) entry.settled = true;
    return mapFragmentsToDocument(entry.fragments, entry.targetLines, docLines);
  }
}
