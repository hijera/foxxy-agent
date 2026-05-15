import { afterEach, describe, expect, test, vi } from "vitest";
import {
  SESSION_DELETE_POST_CONFIRM_BACKDROP_SUPPRESS_MS,
  armSessionDeleteBackdropSuppressUntil,
  shouldSuppressShellBackdropClose,
} from "./sessionDeleteBackdropSuppress";

describe("sessionDeleteBackdropSuppress", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  test("arm sets deadline and suppress clears when time passes", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(5_000_000));
    const ref = { current: 0 };
    armSessionDeleteBackdropSuppressUntil(ref);
    expect(ref.current).toBe(
      5_000_000 + SESSION_DELETE_POST_CONFIRM_BACKDROP_SUPPRESS_MS,
    );
    expect(shouldSuppressShellBackdropClose(ref)).toBe(true);
    vi.setSystemTime(new Date(ref.current));
    expect(shouldSuppressShellBackdropClose(ref)).toBe(false);
  });
});
