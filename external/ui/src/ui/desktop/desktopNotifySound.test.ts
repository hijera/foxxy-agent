import { afterEach, expect, test, vi } from "vitest";
import {
  armNotificationSoundUnlock,
  playNotificationSound,
  previewNotificationSound,
  stopNotificationSound,
} from "./desktopNotifySound";
import { applyNotificationConfig } from "./notificationConfig";

afterEach(() => {
  stopNotificationSound();
  applyNotificationConfig({});
  delete window.foxxycodeHostPlaySound;
  delete window.foxxycodeHostStopSound;
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
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

test("native preview receives the selected bytes and can be stopped", async () => {
  const host = vi.fn().mockResolvedValue(undefined);
  const stop = vi.fn().mockResolvedValue(undefined);
  window.foxxycodeHostPlaySound = host;
  window.foxxycodeHostStopSound = stop;
  const bytes = new Uint8Array([82, 73, 70, 70]);
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      arrayBuffer: async () => bytes.buffer,
    }),
  );
  await previewNotificationSound("custom-preview.wav");
  expect(host).toHaveBeenCalledWith("UklGRg==", true);
  stopNotificationSound();
  expect(stop).toHaveBeenCalled();
});

test("unavailable custom audio reports a preview failure", async () => {
  window.foxxycodeHostPlaySound = vi.fn().mockResolvedValue(undefined);
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: false, status: 404 }));
  await expect(
    previewNotificationSound("missing-preview.wav"),
  ).rejects.toThrow();
});

test("stop cancels an in-flight custom preview before it reaches the native player", async () => {
  let complete!: (res: unknown) => void;
  window.foxxycodeHostPlaySound = vi.fn().mockResolvedValue(undefined);
  vi.stubGlobal(
    "fetch",
    vi.fn().mockReturnValue(
      new Promise((resolve) => {
        complete = resolve;
      }),
    ),
  );
  const preview = previewNotificationSound("pending-preview.wav");
  stopNotificationSound();
  complete({
    ok: true,
    status: 200,
    arrayBuffer: async () => new Uint8Array([1]).buffer,
  });
  await preview;
  expect(window.foxxycodeHostPlaySound).not.toHaveBeenCalled();
});

test("muting cancels a pending automatic custom sound", async () => {
  vi.spyOn(document, "hasFocus").mockReturnValue(false);
  applyNotificationConfig({
    ui: { notification_sound_file: "pending-notification.wav" },
  });
  let complete!: (res: unknown) => void;
  window.foxxycodeHostPlaySound = vi.fn().mockResolvedValue(undefined);
  vi.stubGlobal(
    "fetch",
    vi.fn().mockReturnValue(
      new Promise((resolve) => {
        complete = resolve;
      }),
    ),
  );
  playNotificationSound();
  applyNotificationConfig({
    ui: {
      notification_sound: false,
      notification_sound_file: "pending-notification.wav",
    },
  });
  complete({
    ok: true,
    status: 200,
    arrayBuffer: async () => new Uint8Array([1]).buffer,
  });
  await new Promise((resolve) => setTimeout(resolve, 0));
  expect(window.foxxycodeHostPlaySound).not.toHaveBeenCalled();
});
