import { describe, it, expect } from "vitest";
import {
  buildEditorStateSnapshot,
  sameSnapshot,
  editorStateRequestBody,
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

describe("sameSnapshot", () => {
  it("detects equal and differing snapshots", () => {
    const a = buildEditorStateSnapshot(["/ws/a.go"], "/ws/a.go");
    const b = buildEditorStateSnapshot(["/ws/a.go"], "/ws/a.go");
    const c = buildEditorStateSnapshot(["/ws/a.go", "/ws/b.go"], "/ws/a.go");
    expect(sameSnapshot(a, b)).toBe(true);
    expect(sameSnapshot(a, c)).toBe(false);
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
});
