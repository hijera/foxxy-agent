import { describe, it, expect } from "vitest";
import {
  isNoLiveTurnRelayError,
  parseSessionBusyResponse,
} from "./liveTurnRecovery";

describe("parseSessionBusyResponse", () => {
  it("reads the session that is still working", () => {
    const out = parseSessionBusyResponse(409, {
      error: {
        message: "session busy: another agent turn is in progress",
        code: "session_busy",
        sessionId: "sess_abc",
        turnActive: true,
      },
    });
    expect(out.busy).toBe(true);
    expect(out.sessionId).toBe("sess_abc");
    expect(out.message).toContain("session busy");
  });

  it("still reports busy when an older server sends only the message", () => {
    const out = parseSessionBusyResponse(409, {
      error: { message: "session busy: another agent turn is in progress" },
    });
    expect(out.busy).toBe(true);
    expect(out.sessionId).toBe("");
  });

  it("reports busy when the body cannot be parsed", () => {
    expect(parseSessionBusyResponse(409, null).busy).toBe(true);
    expect(parseSessionBusyResponse(409, "not json").busy).toBe(true);
  });

  it("leaves other 409s alone", () => {
    const out = parseSessionBusyResponse(409, {
      error: {
        message: "workspace is locked once the conversation starts",
        code: "workspace_locked",
      },
    });
    expect(out.busy).toBe(false);
  });

  it("ignores non-409 responses", () => {
    expect(parseSessionBusyResponse(200, { error: { code: "session_busy" } }))
      .toMatchObject({ busy: false });
  });
});

describe("isNoLiveTurnRelayError", () => {
  it("recognizes the relay's idle-session error code", () => {
    expect(isNoLiveTurnRelayError("no_active_stream", null)).toBe(true);
    expect(isNoLiveTurnRelayError("no_active_stream", "anything")).toBe(true);
  });

  // Servers older than the error.code field send the message alone.
  it("still recognizes the message from a server without the code", () => {
    expect(isNoLiveTurnRelayError(null, "no active composer stream")).toBe(true);
  });

  it("treats real failures as errors", () => {
    expect(isNoLiveTurnRelayError(null, "provider returned 500")).toBe(false);
    expect(isNoLiveTurnRelayError("server_error", "boom")).toBe(false);
    expect(isNoLiveTurnRelayError(null, null)).toBe(false);
    expect(isNoLiveTurnRelayError("", "")).toBe(false);
  });
});
