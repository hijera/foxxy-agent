import { afterEach, expect, test, vi } from "vitest";
import {
  armNotificationSoundUnlock,
  playNotificationSound,
} from "./desktopNotifySound";

afterEach(() => {
  vi.unstubAllGlobals();
});

test("playNotificationSound is a no-op when AudioContext is unavailable", () => {
  // jsdom has no AudioContext; the call must not throw.
  vi.stubGlobal("AudioContext", undefined);
  expect(() => playNotificationSound()).not.toThrow();
});

test("armNotificationSoundUnlock does not throw and only arms once", () => {
  expect(() => {
    armNotificationSoundUnlock();
    armNotificationSoundUnlock();
  }).not.toThrow();
});
