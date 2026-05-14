import { describe, expect, it } from "vitest";
import {
  keepLocalTranscriptIfServerEmpty,
  mergeTranscriptPreferLocalSuffix,
} from "./transcriptServerSnapshot";
import type { TranscriptItem } from "./types";

const u = (id: string, text: string): TranscriptItem => ({
  id,
  type: "user_message",
  content: text,
  createdAtUtc: "2020-01-01T00:00:00.000Z",
});

const a = (id: string, text: string, streaming?: boolean): TranscriptItem => ({
  id,
  type: "assistant_message",
  content: text,
  ...(streaming !== undefined ? { streaming } : {}),
});

describe("mergeTranscriptPreferLocalSuffix", () => {
  it("appends local tail when server is a prefix of local", () => {
    const server = [u("1", "q1"), u("2", "q2")];
    const local = [u("1", "q1"), u("2", "q2"), a("3", "partial reply", false)];
    expect(mergeTranscriptPreferLocalSuffix(server, local)).toEqual(local);
  });

  it("replaces last assistant when same length and local body is longer prefix", () => {
    const server = [u("1", "q"), a("2", "ab", false)];
    const local = [u("1", "q"), a("3", "abcd", false)];
    const out = mergeTranscriptPreferLocalSuffix(server, local);
    expect(out).toHaveLength(2);
    expect(out[1]).toMatchObject({
      type: "assistant_message",
      content: "abcd",
      streaming: false,
    });
  });

  it("returns server when prefix does not match", () => {
    const server = [u("1", "a")];
    const local = [u("1", "b"), a("2", "x", false)];
    expect(mergeTranscriptPreferLocalSuffix(server, local)).toEqual(server);
  });
});

describe("keepLocalTranscriptIfServerEmpty", () => {
  it("returns null when server has messages", () => {
    const server = [u("1", "hi")];
    const r = keepLocalTranscriptIfServerEmpty({
      serverNext: server,
      sid: "sess_a",
      viewingSid: "sess_a",
      prevShadow: [u("2", "local")],
      prevItems: [],
    });
    expect(r).toBeNull();
  });

  it("prefers non-empty shadow when server is empty", () => {
    const shadow = [u("1", "shadow")];
    const r = keepLocalTranscriptIfServerEmpty({
      serverNext: [],
      sid: "sess_a",
      viewingSid: "sess_b",
      prevShadow: shadow,
      prevItems: [u("x", "wrong session items")],
    });
    expect(r).toEqual(shadow);
  });

  it("uses on-screen items when viewing this sid and shadow empty", () => {
    const items = [u("1", "screen")];
    const r = keepLocalTranscriptIfServerEmpty({
      serverNext: [],
      sid: "sess_a",
      viewingSid: "sess_a",
      prevShadow: undefined,
      prevItems: items,
    });
    expect(r).toEqual(items);
  });

  it("returns null when server empty and no local rows for this sid", () => {
    const r = keepLocalTranscriptIfServerEmpty({
      serverNext: [],
      sid: "sess_a",
      viewingSid: "sess_b",
      prevShadow: undefined,
      prevItems: [u("1", "other session")],
    });
    expect(r).toBeNull();
  });
});

