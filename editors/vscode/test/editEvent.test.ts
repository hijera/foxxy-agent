import { describe, it, expect } from "vitest";
import {
  parseEditEvent,
  isProposed,
  isApplied,
  isOpenFile,
  isRevealFile,
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

  // An exported transcript lands on disk because this panel's webview cannot
  // save a blob; the plugin only has to put the user in front of the file.
  it("parses reveal_file and keeps it distinct from open_file", () => {
    const ev = parseEditEvent(
      `{"type":"reveal_file","sessionId":"s-1","path":"/tmp/foxxycode/exports/s-1/Chat.pdf"}`,
    );
    expect(ev).not.toBeNull();
    expect(isRevealFile(ev!)).toBe(true);
    expect(isOpenFile(ev!)).toBe(false);
    expect(isProposed(ev!)).toBe(false);
    expect(isApplied(ev!)).toBe(false);
    expect(ev!.path).toBe("/tmp/foxxycode/exports/s-1/Chat.pdf");
  });

  it("does not treat open_file or edit events as reveal_file", () => {
    expect(isRevealFile(parseEditEvent(`{"type":"open_file","path":"p"}`)!)).toBe(
      false,
    );
    expect(
      isRevealFile(parseEditEvent(`{"type":"edit_applied","path":"p"}`)!),
    ).toBe(false);
  });
});
