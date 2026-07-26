import { describe, expect, it } from "vitest";

import { localizeLoopGuardNotice } from "./loopGuardNotice";

// Stand-in for t(): echoes the key plus any interpolated params so the test
// asserts on routing, not on the wording of a dictionary entry.
function fakeT(key: string, params?: Record<string, string | number>): string {
  return params ? `${key}:${JSON.stringify(params)}` : key;
}

describe("localizeLoopGuardNotice", () => {
  it("maps the streamed-output notice", () => {
    expect(
      localizeLoopGuardNotice(
        "stopped: the model kept repeating the same output instead of finishing the task",
        fakeT,
      ),
    ).toBe("messages.loopGuardStream");
  });

  it("maps the reasoning notice", () => {
    expect(
      localizeLoopGuardNotice(
        "stopped: the model kept repeating the same reasoning without reaching an answer",
        fakeT,
      ),
    ).toBe("messages.loopGuardReasoning");
  });

  it("maps the tool notice and keeps the tool name", () => {
    expect(
      localizeLoopGuardNotice(
        "stopped: the model kept requesting the same read call with identical arguments",
        fakeT,
      ),
    ).toBe('messages.loopGuardTool:{"tool":"read"}');
  });

  it("tolerates surrounding whitespace", () => {
    expect(
      localizeLoopGuardNotice(
        "  stopped: the model kept repeating the same output instead of finishing the task\n",
        fakeT,
      ),
    ).toBe("messages.loopGuardStream");
  });

  it("leaves every other error to render verbatim", () => {
    expect(localizeLoopGuardNotice("dial tcp: connection refused", fakeT)).toBe(
      null,
    );
    expect(localizeLoopGuardNotice("", fakeT)).toBe(null);
    // A tool notice with an empty tool name is not one of ours.
    expect(
      localizeLoopGuardNotice(
        "stopped: the model kept requesting the same  call with identical arguments",
        fakeT,
      ),
    ).toBe(null);
  });
});
