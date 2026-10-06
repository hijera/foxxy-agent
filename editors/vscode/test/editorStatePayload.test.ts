import { describe, it, expect } from "vitest";
import {
  buildEditorStateSnapshot,
  buildSelectionPayload,
  sameSnapshot,
  editorStateRequestBody,
  MAX_SELECTION_BYTES,
} from "../src/ide/editorStatePayload";

describe("buildEditorStateSnapshot", () => {
  it("trims, drops blanks, and de-duplicates while keeping order", () => {
    const snap = buildEditorStateSnapshot(
      [" /ws/a.go ", "", "/ws/b.go", "/ws/a.go", null, undefined],
      undefined,
    );
    expect(snap.openFiles).toEqual(["/ws/a.go", "/ws/b.go"]);
    expect(snap.activeFile).toBe("");
  });

  it("puts the active file first in openFiles", () => {
    const snap = buildEditorStateSnapshot(["/ws/a.go", "/ws/b.go"], "/ws/b.go");
    expect(snap.activeFile).toBe("/ws/b.go");
    expect(snap.openFiles).toEqual(["/ws/b.go", "/ws/a.go"]);
  });

  it("includes an active file even when not in the open list", () => {
    const snap = buildEditorStateSnapshot([], "/ws/x.go");
    expect(snap.openFiles).toEqual(["/ws/x.go"]);
  });
});

describe("buildSelectionPayload", () => {
  it("converts 0-based lines to a 1-based inclusive range", () => {
    const sel = buildSelectionPayload("/ws/a.go", 20, 30, 5, "x := 1");
    expect(sel).toEqual({ file: "/ws/a.go", startLine: 21, endLine: 31, text: "x := 1" });
  });

  it("excludes a final line the selection only touches at column 0", () => {
    const sel = buildSelectionPayload("/ws/a.go", 20, 31, 0, "x\ny\n");
    expect(sel).toEqual({ file: "/ws/a.go", startLine: 21, endLine: 31, text: "x\ny\n" });
  });

  it("returns undefined for empty, whitespace-only, or file-less selections", () => {
    expect(buildSelectionPayload("/ws/a.go", 1, 1, 5, "")).toBeUndefined();
    expect(buildSelectionPayload("/ws/a.go", 1, 1, 5, "   \n ")).toBeUndefined();
    expect(buildSelectionPayload("", 1, 1, 5, "text")).toBeUndefined();
    // Caret sitting at column 0 of the same line selects nothing.
    expect(buildSelectionPayload("/ws/a.go", 3, 3, 0, "")).toBeUndefined();
  });

  it("caps the selection text, keeping the tail", () => {
    const long = "y".repeat(MAX_SELECTION_BYTES + 500);
    const sel = buildSelectionPayload("/ws/a.go", 0, 400, 3, long);
    expect(sel?.text.length).toBe(MAX_SELECTION_BYTES);
    expect(long.endsWith(sel!.text)).toBe(true);
  });
});

describe("sameSnapshot", () => {
  it("detects equal and differing snapshots", () => {
    const a = buildEditorStateSnapshot(["/ws/a.go"], "/ws/a.go");
    const b = buildEditorStateSnapshot(["/ws/a.go"], "/ws/a.go");
    const c = buildEditorStateSnapshot(["/ws/a.go", "/ws/b.go"], "/ws/a.go");
    expect(sameSnapshot(a, b)).toBe(true);
    expect(sameSnapshot(a, c)).toBe(false);
  });

  it("compares selections", () => {
    const sel = buildSelectionPayload("/ws/a.go", 1, 2, 3, "x");
    const a = buildEditorStateSnapshot(["/ws/a.go"], "/ws/a.go", sel);
    const b = buildEditorStateSnapshot(["/ws/a.go"], "/ws/a.go", sel);
    const c = buildEditorStateSnapshot(
      ["/ws/a.go"],
      "/ws/a.go",
      buildSelectionPayload("/ws/a.go", 1, 5, 3, "x\ny"),
    );
    const d = buildEditorStateSnapshot(["/ws/a.go"], "/ws/a.go");
    expect(sameSnapshot(a, b)).toBe(true);
    expect(sameSnapshot(a, c)).toBe(false);
    expect(sameSnapshot(a, d)).toBe(false);
    expect(sameSnapshot(d, a)).toBe(false);
  });
});

describe("editorStateRequestBody", () => {
  it("serializes to the backend request shape", () => {
    const snap = buildEditorStateSnapshot(["/ws/a.go"], "/ws/a.go");
    expect(JSON.parse(editorStateRequestBody(snap))).toEqual({
      openFiles: ["/ws/a.go"],
      activeFile: "/ws/a.go",
    });
  });

  it("includes the selection only when present", () => {
    const sel = buildSelectionPayload("/ws/a.go", 20, 30, 5, "x := 1");
    const snap = buildEditorStateSnapshot(["/ws/a.go"], "/ws/a.go", sel);
    expect(JSON.parse(editorStateRequestBody(snap))).toEqual({
      openFiles: ["/ws/a.go"],
      activeFile: "/ws/a.go",
      selection: { file: "/ws/a.go", startLine: 21, endLine: 31, text: "x := 1" },
    });
  });
});
