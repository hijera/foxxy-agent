import { describe, it, expect } from "vitest";
import {
  appendBounded,
  sameTerminalSnapshot,
  stripAnsi,
  terminalStateRequestBody,
  type TerminalSnapshot,
} from "../src/ide/terminalStatePayload";

const ESC = String.fromCharCode(0x1b);
const BEL = String.fromCharCode(0x07);

describe("appendBounded", () => {
  it("keeps everything under the cap", () => {
    expect(appendBounded("ab", "cd", 10)).toBe("abcd");
  });
  it("keeps only the tail when over the cap", () => {
    const out = appendBounded("x".repeat(20), "END", 10);
    expect(out.length).toBe(10);
    expect(out.endsWith("END")).toBe(true);
  });
});

describe("stripAnsi", () => {
  it("removes CSI colour codes", () => {
    expect(stripAnsi(`${ESC}[31mred${ESC}[0m`)).toBe("red");
  });
  it("removes shell-integration OSC 633 markers", () => {
    const s = `${ESC}]633;C${BEL}ok${ESC}]633;D;0${BEL}`;
    expect(stripAnsi(s)).toBe("ok");
  });
  it("leaves plain text untouched", () => {
    expect(stripAnsi("plain line\n")).toBe("plain line\n");
  });
});

describe("sameTerminalSnapshot", () => {
  const base: TerminalSnapshot = {
    terminals: [{ id: "1", name: "zsh", output: "ok\n", active: true }],
  };
  it("detects equal snapshots", () => {
    const copy: TerminalSnapshot = {
      terminals: [{ id: "1", name: "zsh", output: "ok\n", active: true }],
    };
    expect(sameTerminalSnapshot(base, copy)).toBe(true);
  });
  it("detects output/active/name/count changes", () => {
    expect(
      sameTerminalSnapshot(base, {
        terminals: [{ id: "1", name: "zsh", output: "different\n", active: true }],
      }),
    ).toBe(false);
    expect(
      sameTerminalSnapshot(base, {
        terminals: [{ id: "1", name: "zsh", output: "ok\n", active: false }],
      }),
    ).toBe(false);
    expect(sameTerminalSnapshot(base, { terminals: [] })).toBe(false);
  });
});

describe("terminalStateRequestBody", () => {
  it("serializes to the backend request shape", () => {
    const snap: TerminalSnapshot = {
      terminals: [
        { id: "1", name: "zsh", output: "ok\n", active: true, lastCommand: "ls" },
      ],
    };
    expect(JSON.parse(terminalStateRequestBody(snap))).toEqual({
      terminals: [
        { id: "1", name: "zsh", output: "ok\n", active: true, lastCommand: "ls" },
      ],
    });
  });
});
