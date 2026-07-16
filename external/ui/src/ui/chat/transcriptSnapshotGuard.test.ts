import { describe, expect, it } from "vitest";
import { shouldApplyTranscriptSnapshot } from "./transcriptSnapshotGuard";

describe("shouldApplyTranscriptSnapshot", () => {
  it("accepts a current snapshot while the session is idle", () => {
    expect(
      shouldApplyTranscriptSnapshot({
        requestEpoch: 3,
        currentEpoch: 3,
        activeComposer: false,
      }),
    ).toBe(true);
  });

  it("rejects a response started before a newer stream epoch", () => {
    expect(
      shouldApplyTranscriptSnapshot({
        requestEpoch: 2,
        currentEpoch: 3,
        activeComposer: false,
      }),
    ).toBe(false);
  });

  it("does not replace an active live transcript by default", () => {
    expect(
      shouldApplyTranscriptSnapshot({
        requestEpoch: 3,
        currentEpoch: 3,
        activeComposer: true,
      }),
    ).toBe(false);
  });

  it("allows the final same-epoch reconciliation explicitly", () => {
    expect(
      shouldApplyTranscriptSnapshot({
        requestEpoch: 3,
        currentEpoch: 3,
        activeComposer: true,
        allowWhileActive: true,
      }),
    ).toBe(true);
  });
});
