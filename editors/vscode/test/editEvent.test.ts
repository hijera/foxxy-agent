import { describe, it, expect } from "vitest";
import {
  parseEditEvent,
  isProposed,
  isApplied,
  isOpenFile,
} from "../src/diff/editEvent";

describe("parseEditEvent", () => {
  it("parses a well-formed edit_proposed payload", () => {
    const ev = parseEditEvent(
      JSON.stringify({
        type: "edit_proposed",
        toolCallId: "tc-1",
        sessionId: "s-1",
        path: "/tmp/file.go",
        before: "package main\n",
        after: "package main\n// hi\n",
      }),
    );
    expect(ev).not.toBeNull();
    expect(ev!.type).toBe("edit_proposed");
    expect(ev!.toolCallId).toBe("tc-1");
    expect(ev!.sessionId).toBe("s-1");
    expect(ev!.path).toBe("/tmp/file.go");
    expect(isProposed(ev!)).toBe(true);
    expect(isApplied(ev!)).toBe(false);
  });

  it("parses edit_applied", () => {
    const ev = parseEditEvent(`{"type":"edit_applied","toolCallId":"t","sessionId":"s","path":"p","before":"","after":"x"}`);
    expect(isApplied(ev!)).toBe(true);
    expect(isProposed(ev!)).toBe(false);
  });

  it("returns null on invalid JSON", () => {
    expect(parseEditEvent("not json")).toBeNull();
  });

  it("returns null when type is missing", () => {
    expect(parseEditEvent(`{"toolCallId":"t"}`)).toBeNull();
  });

  it("defaults missing string fields to empty", () => {
    const ev = parseEditEvent(`{"type":"edit_proposed"}`);
    expect(ev).not.toBeNull();
    expect(ev!.toolCallId).toBe("");
    expect(ev!.before).toBe("");
    expect(ev!.after).toBe("");
  });

  // "Show in IDE" on a plan card: only path and sessionId are set, and the file
  // lives in the session bundle outside the workspace.
  it("parses open_file and keeps it distinct from the edit events", () => {
    const ev = parseEditEvent(
      `{"type":"open_file","sessionId":"s-1","path":"/home/me/.foxxycode/sessions/s-1/plans/demo.plan.md"}`,
    );
    expect(ev).not.toBeNull();
    expect(isOpenFile(ev!)).toBe(true);
    expect(isProposed(ev!)).toBe(false);
    expect(isApplied(ev!)).toBe(false);
    expect(ev!.path).toBe(
      "/home/me/.foxxycode/sessions/s-1/plans/demo.plan.md",
    );
  });

  it("does not treat edit events as open_file", () => {
    const ev = parseEditEvent(`{"type":"edit_applied","path":"p"}`);
    expect(isOpenFile(ev!)).toBe(false);
  });
});
