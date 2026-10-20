import { describe, it, expect } from "vitest";
import { InlineHighlightTracker } from "../src/diff/inlineHighlights";
import { LineFragment } from "../src/diff/lineFragments";

// The Go demo project of the 2026-09-15 report: a 64-line todo.go.
const ORIGINAL = [
  "package todo", //                                                     1
  "",
  "import (",
  '\t"sync"',
  ")",
  "",
  "// Todo is one item on the list.",
  "type Todo struct {",
  "\tID    int",
  "\tTitle string", //                                                  10
  "\tDone  bool",
  "}",
  "",
  "// Store keeps todos in memory.",
  "type Store struct {",
  "\tmu     sync.Mutex",
  "\tnextID int",
  "\titems  map[int]*Todo",
  "}",
  "", //                                                                20
  "// NewStore returns an empty store.",
  "func NewStore() *Store {",
  "\treturn &Store{nextID: 1, items: map[int]*Todo{}}",
  "}",
  "",
  "// Len reports how many todos the store holds.",
  "func (s *Store) Len() int {",
  "\ts.mu.Lock()",
  "\tdefer s.mu.Unlock()",
  "\treturn len(s.items)", //                                           30
  "}",
  "// CreateTodo adds a todo and returns it.",
  "func (s *Store) CreateTodo(title string) (*Todo, error) {",
  "\ts.mu.Lock()",
  "\tdefer s.mu.Unlock()",
  "\tt := &Todo{ID: s.nextID, Title: title}",
  "\ts.items[t.ID] = t",
  "\ts.nextID++",
  "\treturn t, nil",
  "}", //                                                               40
  "",
  "// Get returns the todo with the given id.",
  "func (s *Store) Get(id int) (*Todo, error) {",
  "\ts.mu.Lock()",
  "\tdefer s.mu.Unlock()",
  "\tt, ok := s.items[id]",
  "\tif !ok {",
  "\t\treturn nil, ErrNotFound",
  "\t}",
  "\treturn t, nil", //                                                 50
  "}",
  "",
  "// Complete marks a todo as done.",
  "func (s *Store) Complete(id int) error {",
  "\tt, err := s.Get(id)",
  "\tif err != nil {",
  "\t\treturn err",
  "\t}",
  "\tt.Done = true",
  "\treturn nil", //                                                    60
  "}",
  "",
  '// ErrNotFound is returned for an unknown id.',
  'var ErrNotFound = errors.New("todo not found")',
].join("\n");

// Edit 1 replaces the import block and adds two lines near the top.
const AFTER_EDIT_1 = ORIGINAL.replace('import (\n\t"sync"\n)', 'import (\n\t"errors"\n\t"strings"\n\t"sync"\n)');

// Edit 2 replaces the CreateTodo header with 12 new lines above it: a const, a
// var and a 4-line validation block. They land on lines 34-45 of the file.
const EDIT_2_INSERTED = [
  "",
  "// maxTitleLen caps the length of a todo title.",
  "const maxTitleLen = 200",
  "",
  'var errBadTitle = errors.New("title is empty or too long")',
  "",
  "func validTitle(title string) error {",
  '\tif strings.TrimSpace(title) == "" || len(title) > maxTitleLen {',
  "\t\treturn errBadTitle",
  "\t}",
  "\treturn nil",
  "}",
];
const AFTER_EDIT_2 = AFTER_EDIT_1.replace(
  "// CreateTodo adds a todo and returns it.",
  [...EDIT_2_INSERTED, "// CreateTodo adds a todo and returns it."].join("\n"),
);

const lines = (s: string): string[] => s.split(/\r?\n/);

/** A text document the way VS Code holds an open, unmodified file: it keeps
 *  the text it read until its file watcher notices a write, then reloads
 *  through `ModelService._computeEdits`, which replaces the lines between the
 *  common prefix and suffix with one `replaceMove`. A decoration that starts
 *  at the edit point is forced below the inserted text; one inside the
 *  replaced lines collapses to its end. Tracked here at line granularity. */
class OpenDocument {
  decorations: LineFragment[] = [];
  constructor(public text: string) {}

  reloadFromDisk(next: string): void {
    const a = lines(this.text);
    const b = lines(next);
    let prefix = 0;
    while (prefix < a.length && prefix < b.length && a[prefix] === b[prefix]) prefix++;
    let suffix = 0;
    while (
      suffix < a.length - prefix &&
      suffix < b.length - prefix &&
      a[a.length - 1 - suffix] === b[b.length - 1 - suffix]
    ) {
      suffix++;
    }
    const oldEnd = a.length - suffix;
    const newEnd = b.length - suffix;
    const move = (line: number): number =>
      line < prefix ? line : line >= oldEnd ? line + (newEnd - oldEnd) : newEnd;
    this.decorations = this.decorations
      .map((f) => ({ ...f, startLine: move(f.startLine), endLine: move(f.endLine - 1) + 1 }))
      .map((f) => (f.startLine >= f.endLine ? null : f))
      .filter((f): f is LineFragment => f !== null && f.endLine > f.startLine);
    this.text = next;
  }

  /** 1-based inclusive line spans, the way the editor gutter shows them. */
  highlighted(): string[] {
    return this.decorations.map((f) => `${f.startLine + 1}-${f.endLine}`);
  }
}

const KEY = "h:/demo/todo.go";

/** What `IdeDiffService` does when an `edit_applied` event arrives: register
 *  the event, then draw on the document once the editor is open. */
function editApplied(
  tracker: InlineHighlightTracker,
  doc: OpenDocument,
  before: string,
  after: string,
): void {
  const token = tracker.begin(KEY, { before, after, useAfterRanges: true });
  const ranges = tracker.place(KEY, token, doc.text);
  if (ranges) doc.decorations = ranges;
}

/** What `IdeDiffService` does on `onDidChangeTextDocument` for the file. */
function documentChanged(tracker: InlineHighlightTracker, doc: OpenDocument, dirty = false): void {
  const ranges = tracker.documentChanged(KEY, doc.text, dirty);
  if (ranges) doc.decorations = ranges;
}

describe("InlineHighlightTracker", () => {
  it("fixture matches the report", () => {
    expect(lines(ORIGINAL)).toHaveLength(64);
    expect(lines(AFTER_EDIT_1)).toHaveLength(66);
    expect(lines(AFTER_EDIT_2)).toHaveLength(78);
    expect(lines(AFTER_EDIT_2).slice(33, 45)).toEqual(EDIT_2_INSERTED);
  });

  it("highlights the second edit of a file on its own lines, not below them", () => {
    const tracker = new InlineHighlightTracker();

    // Edit 1 opens the file, so the document is read after the write.
    const doc = new OpenDocument(AFTER_EDIT_1);
    editApplied(tracker, doc, ORIGINAL, AFTER_EDIT_1);
    expect(doc.highlighted()).toEqual(["4-5"]);

    // Edit 2: the file is open now, and the event outruns VS Code's reload.
    editApplied(tracker, doc, AFTER_EDIT_1, AFTER_EDIT_2);
    doc.reloadFromDisk(AFTER_EDIT_2);
    documentChanged(tracker, doc);
    expect(doc.highlighted()).toEqual(["34-45"]);
  });

  it("highlights an edit on its own lines when the event outruns the reload of an open file", () => {
    const tracker = new InlineHighlightTracker();
    const doc = new OpenDocument(ORIGINAL);
    editApplied(tracker, doc, ORIGINAL, AFTER_EDIT_1);
    doc.reloadFromDisk(AFTER_EDIT_1);
    documentChanged(tracker, doc);
    expect(doc.highlighted()).toEqual(["4-5"]);
  });

  it("draws at once when the document already holds the written text", () => {
    const tracker = new InlineHighlightTracker();
    const doc = new OpenDocument(AFTER_EDIT_2);
    editApplied(tracker, doc, AFTER_EDIT_1, AFTER_EDIT_2);
    expect(doc.highlighted()).toEqual(["34-45"]);
  });

  it("leaves highlights to the editor once the document caught up", () => {
    const tracker = new InlineHighlightTracker();
    const doc = new OpenDocument(AFTER_EDIT_1);
    editApplied(tracker, doc, AFTER_EDIT_1, AFTER_EDIT_2);
    doc.reloadFromDisk(AFTER_EDIT_2);
    documentChanged(tracker, doc);
    // The user types above the edit; VS Code moves the decorations itself.
    expect(tracker.documentChanged(KEY, "// note\n" + AFTER_EDIT_2, true)).toBeNull();
  });

  it("ignores a slower render of an edit a newer event for the same file replaced", () => {
    // Two parallel tool calls: both events arrive, the handlers resume in reverse order.
    const tracker = new InlineHighlightTracker();
    const first = tracker.begin(KEY, { before: ORIGINAL, after: AFTER_EDIT_1, useAfterRanges: true });
    const second = tracker.begin(KEY, { before: AFTER_EDIT_1, after: AFTER_EDIT_2, useAfterRanges: true });
    expect(tracker.place(KEY, second, AFTER_EDIT_2)).toEqual([{ startLine: 33, endLine: 45, kind: "add" }]);
    expect(tracker.place(KEY, first, AFTER_EDIT_2)).toBeNull();
  });

  it("places the lines it can find when the document has unsaved changes", () => {
    const tracker = new InlineHighlightTracker();
    // The user added a line at the top before the write, so VS Code will not reload.
    const docText = "// scratch\n" + AFTER_EDIT_2;
    const token = tracker.begin(KEY, { before: AFTER_EDIT_1, after: AFTER_EDIT_2, useAfterRanges: true });
    expect(tracker.place(KEY, token, docText)).toEqual([{ startLine: 34, endLine: 46, kind: "add" }]);
    // Dirty document: this placement is final, later keystrokes are the editor's.
    expect(tracker.documentChanged(KEY, docText, true)).toEqual([{ startLine: 34, endLine: 46, kind: "add" }]);
    expect(tracker.documentChanged(KEY, "x\n" + docText, true)).toBeNull();
  });

  it("matches a legacy-encoded file whose event text carries replacement characters", () => {
    // A Windows-1251 file reaches the event stream as invalid UTF-8, one U+FFFD per letter.
    const tracker = new InlineHighlightTracker();
    const after = "// \uFFFD\uFFFD\uFFFD\uFFFD\nconst a = 1\nconst b = 2";
    const before = "// \uFFFD\uFFFD\uFFFD\uFFFD\nconst a = 1";
    const token = tracker.begin(KEY, { before, after, useAfterRanges: true });
    expect(tracker.place(KEY, token, "\uFEFF// тест\r\nconst a = 1\r\nconst b = 2")).toEqual([
      { startLine: 2, endLine: 3, kind: "add" },
    ]);
    expect(tracker.documentChanged(KEY, "// тест\r\nconst a = 1\r\nconst b = 2", false)).toBeNull();
  });
});
