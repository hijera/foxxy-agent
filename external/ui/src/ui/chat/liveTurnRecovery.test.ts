import { describe, it, test, expect } from "vitest";
import {
  isNoLiveTurnRelayError,
  parseSessionBusyResponse,
  ACTIVITY_FAIL_MAX,
  DISK_FALLBACK_MAX_MS,
  DISK_FALLBACK_MIN_MS,
  DISK_FALLBACK_QUIET_TICKS,
  LIVE_RECONNECT_MAX_DELAY_MS,
  diskFallbackDelayMs,
  liveReconnectDelayMs,
  shouldKeepWatching,
  shouldReloadTranscript,
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

describe("watching a turn that outlives its stream", () => {
  test("reconnect delays back off but stop growing", () => {
    expect(liveReconnectDelayMs(0)).toBe(400);
    expect(liveReconnectDelayMs(1)).toBe(800);
    expect(liveReconnectDelayMs(3)).toBe(3200);
    // A half-hour turn must not schedule a retry an hour out.
    expect(liveReconnectDelayMs(20)).toBe(LIVE_RECONNECT_MAX_DELAY_MS);
  });

  test("watching ends on repeated probe failures, not on a retry count", () => {
    expect(shouldKeepWatching(0)).toBe(true);
    // Four failures in a row is still worth another try; the old rule gave up
    // after five attempts however healthy the server was.
    expect(shouldKeepWatching(ACTIVITY_FAIL_MAX - 1)).toBe(true);
    expect(shouldKeepWatching(ACTIVITY_FAIL_MAX)).toBe(false);
  });

  test("the poll slows down only while nothing is being written", () => {
    expect(diskFallbackDelayMs(0)).toBe(DISK_FALLBACK_MIN_MS);
    expect(diskFallbackDelayMs(DISK_FALLBACK_QUIET_TICKS - 1)).toBe(
      DISK_FALLBACK_MIN_MS,
    );
    expect(diskFallbackDelayMs(DISK_FALLBACK_QUIET_TICKS)).toBe(
      DISK_FALLBACK_MAX_MS,
    );
  });

  test("the transcript is re-read only when it grew", () => {
    const base = { firstTick: false, turnActive: true } as const;
    expect(
      shouldReloadTranscript({ ...base, messageSeq: 12, lastMessageSeq: 11 }),
    ).toBe(true);
    expect(
      shouldReloadTranscript({ ...base, messageSeq: 12, lastMessageSeq: 12 }),
    ).toBe(false);
    // Nothing is known yet on the first tick.
    expect(
      shouldReloadTranscript({
        firstTick: true,
        turnActive: true,
        messageSeq: 12,
        lastMessageSeq: 12,
      }),
    ).toBe(true);
    // The tick that sees the turn end is where the final answer lands.
    expect(
      shouldReloadTranscript({
        firstTick: false,
        turnActive: false,
        messageSeq: 12,
        lastMessageSeq: 12,
      }),
    ).toBe(true);
    // A server that reports no messageSeq (older build, or a session not live
    // in that process) keeps the old behaviour: reload every tick.
    expect(
      shouldReloadTranscript({
        ...base,
        messageSeq: undefined,
        lastMessageSeq: 12,
      }),
    ).toBe(true);
  });
});
