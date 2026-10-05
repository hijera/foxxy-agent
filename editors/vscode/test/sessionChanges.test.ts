import { describe, it, expect } from "vitest";
import { parseChangedFiles, statLabel } from "../src/changes/sessionChanges";
import { isOpenChanges, parseEditEvent } from "../src/diff/editEvent";

describe("parseChangedFiles", () => {
  it("reads the change set the backend sends", () => {
    const body = JSON.stringify({
      object: "foxxycode.session_changes",
      files: [
        {
          path: "src/a.ts",
          status: "modified",
          additions: 3,
          deletions: 1,
          binary: false,
          truncated: false,
          before: "old\n",
          after: "new\n",
        },
      ],
      totals: { files: 1, additions: 3, deletions: 1 },
    });
    expect(parseChangedFiles(body)).toEqual([
      {
        path: "src/a.ts",
        status: "modified",
        additions: 3,
        deletions: 1,
        binary: false,
        before: "old\n",
        after: "new\n",
      },
    ]);
  });

  it("empties the view instead of throwing on an unexpected body", () => {
    // A plugin talking to an older backend must degrade, not break the tree.
    expect(parseChangedFiles("not json")).toEqual([]);
    expect(parseChangedFiles("{}")).toEqual([]);
    expect(parseChangedFiles('{"files":null}')).toEqual([]);
  });

  it("fills in missing fields rather than propagating undefined", () => {
    const [file] = parseChangedFiles('{"files":[{"path":"x"}]}');
    expect(file).toEqual({
      path: "x",
      status: "",
      additions: 0,
      deletions: 0,
      binary: false,
      before: "",
      after: "",
    });
  });
});

describe("statLabel", () => {
  const base = {
    path: "x",
    status: "modified",
    additions: 8,
    deletions: 2,
    binary: false,
    before: "",
    after: "",
  };

  it("shows the line counts for a text file", () => {
    expect(statLabel(base, "binary")).toBe("+8 −2");
  });

  it("says binary instead of counts that do not exist", () => {
    expect(statLabel({ ...base, binary: true, additions: 0, deletions: 0 }, "binary")).toBe(
      "binary",
    );
  });
});

describe("open_changes event", () => {
  it("is recognised and carries only the session", () => {
    const ev = parseEditEvent('{"type":"open_changes","sessionId":"sess_1","path":""}');
    expect(ev).not.toBeNull();
    expect(isOpenChanges(ev!)).toBe(true);
    expect(ev!.sessionId).toBe("sess_1");
  });

  it("keeps the clicked file so the view can open its diff", () => {
    const ev = parseEditEvent(
      '{"type":"open_changes","sessionId":"sess_1","path":"vue.config.js"}',
    );
    expect(isOpenChanges(ev!)).toBe(true);
    expect(ev!.path).toBe("vue.config.js");
  });

  it("does not claim the other event types", () => {
    for (const type of ["edit_proposed", "edit_applied", "open_file", "reveal_file"]) {
      const ev = parseEditEvent(JSON.stringify({ type, path: "/x" }));
      expect(isOpenChanges(ev!)).toBe(false);
    }
  });
});
