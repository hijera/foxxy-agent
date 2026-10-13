import { afterEach, expect, test, vi } from "vitest";
import {
  dismissHostNotification,
  receiveNotificationAction,
  showHostNotification,
  limitNotificationActions,
} from "./hostNotifications";
import { applyNotificationConfig } from "./notificationConfig";
import {
  notificationSoundData,
  playNotificationSound,
} from "./desktopNotifySound";

vi.mock("./desktopNotifySound", () => ({
  playNotificationSound: vi.fn(),
  stopNotificationSound: vi.fn(),
  notificationSoundData: vi.fn(),
}));

afterEach(() => {
  delete window.foxxycodeHostNotify;
  delete window.foxxycodeHostIsActive;
  delete window.foxxycodeHostPlaySound;
  delete document.documentElement.dataset.embed;
  applyNotificationConfig({});
  vi.clearAllMocks();
  vi.restoreAllMocks();
});

test("native notifications receive the selected custom audio", async () => {
  window.foxxycodeHostIsActive = vi.fn().mockResolvedValue(false);
  window.foxxycodeHostPlaySound = vi.fn().mockResolvedValue(undefined);
  window.foxxycodeHostNotify = vi.fn().mockResolvedValue(true);
  vi.mocked(notificationSoundData).mockResolvedValue("UklGRg==");
  applyNotificationConfig({ ui: { notification_sound_file: "custom.wav" } });
  await showHostNotification(
    {
      id: "custom-native",
      sessionId: "s",
      title: "Chat",
      body: "Done",
      actions: [],
    },
    vi.fn(),
  );
  expect(notificationSoundData).toHaveBeenCalledWith("custom.wav");
  expect(window.foxxycodeHostNotify).toHaveBeenCalledWith(
    expect.objectContaining({ soundData: "UklGRg==", silent: false }),
  );
});

test("muting during custom audio loading sends a silent native notification", async () => {
  window.foxxycodeHostIsActive = vi.fn().mockResolvedValue(false);
  window.foxxycodeHostPlaySound = vi.fn().mockResolvedValue(undefined);
  window.foxxycodeHostNotify = vi.fn().mockResolvedValue(true);
  applyNotificationConfig({ ui: { notification_sound_file: "mute-race.wav" } });
  vi.mocked(notificationSoundData).mockImplementation(async () => {
    applyNotificationConfig({
      ui: {
        notification_sound: false,
        notification_sound_file: "mute-race.wav",
      },
    });
    return "UklGRg==";
  });
  await showHostNotification(
    {
      id: "custom-native-muted",
      sessionId: "s",
      title: "Chat",
      body: "Done",
      actions: [],
    },
    vi.fn(),
  );
  expect(window.foxxycodeHostNotify).toHaveBeenCalledWith(
    expect.objectContaining({ silent: true }),
  );
  expect(
    vi.mocked(window.foxxycodeHostNotify).mock.calls[0]?.[0]?.soundData,
  ).toBeUndefined();
});

test("foreground notifications can be enabled with actions and sound", async () => {
  vi.spyOn(document, "hasFocus").mockReturnValue(true);
  window.foxxycodeHostIsActive = vi.fn().mockResolvedValue(true);
  const host = vi.fn().mockResolvedValue(true);
  window.foxxycodeHostNotify = host;
  document.documentElement.dataset.embed = "intellij";
  applyNotificationConfig({ ui: { notifications_when_focused: true } });
  const action = vi.fn();
  expect(
    await showHostNotification(
      {
        id: "foreground-enabled",
        sessionId: "s",
        title: "Chat",
        body: "Done",
        actions: [{ id: "open", label: "Open", run: action }],
      },
      vi.fn(),
    ),
  ).toBe(true);
  expect(host).toHaveBeenCalledWith(
    expect.objectContaining({ whenFocused: true, silent: false }),
  );
  expect(playNotificationSound).toHaveBeenCalledTimes(1);
  receiveNotificationAction("foreground-enabled", "open");
  expect(action).toHaveBeenCalledTimes(1);
});

test("global disable wins over foreground delivery preference", async () => {
  vi.spyOn(document, "hasFocus").mockReturnValue(true);
  const host = vi.fn().mockResolvedValue(true);
  window.foxxycodeHostNotify = host;
  applyNotificationConfig({
    ui: { notifications: false, notifications_when_focused: true },
  });
  expect(
    await showHostNotification(
      {
        id: "foreground-disabled",
        sessionId: "s",
        title: "Chat",
        body: "Done",
        actions: [],
      },
      vi.fn(),
    ),
  ).toBe(true);
  expect(host).not.toHaveBeenCalled();
  expect(playNotificationSound).not.toHaveBeenCalled();
});

test("foreground delivery failures still allow the enabled inline fallback", async () => {
  vi.spyOn(document, "hasFocus").mockReturnValue(true);
  window.foxxycodeHostNotify = vi
    .fn()
    .mockRejectedValue(new Error("unavailable"));
  applyNotificationConfig({
    ui: { notifications_when_focused: true, notification_sound: false },
  });
  expect(
    await showHostNotification(
      {
        id: "foreground-fallback",
        sessionId: "s",
        title: "Chat",
        body: "Permission",
        actions: [],
      },
      vi.fn(),
    ),
  ).toBe(false);
  expect(playNotificationSound).not.toHaveBeenCalled();
});

test("a prompt resolved during the native focus query never delivers a stale alert", async () => {
  const host = vi.fn().mockResolvedValue(true);
  window.foxxycodeHostNotify = host;
  window.foxxycodeHostIsActive = async () => {
    dismissHostNotification("resolved-during-focus");
    return false;
  };
  expect(
    await showHostNotification(
      {
        id: "resolved-during-focus",
        sessionId: "s",
        title: "Chat",
        body: "Permission",
        actions: [],
      },
      vi.fn(),
    ),
  ).toBe(true);
  expect(host).not.toHaveBeenCalled();
});

test("a foreground chat suppresses delivery, sound and later stream replay", async () => {
  vi.spyOn(document, "hasFocus").mockReturnValue(true);
  const host = vi.fn().mockResolvedValue(true);
  window.foxxycodeHostNotify = host;
  document.documentElement.dataset.embed = "intellij";
  const n = {
    id: "foreground",
    sessionId: "s",
    title: "Chat",
    body: "Done",
    actions: [],
  };
  expect(await showHostNotification(n, vi.fn())).toBe(true);
  expect(host).not.toHaveBeenCalled();
  expect(playNotificationSound).not.toHaveBeenCalled();
  vi.spyOn(document, "hasFocus").mockReturnValue(false);
  expect(await showHostNotification(n, vi.fn())).toBe(true);
  expect(host).not.toHaveBeenCalled();
});

test("a minimized chat delivers even if the webview retains document focus", async () => {
  vi.spyOn(document, "hasFocus").mockReturnValue(true);
  vi.spyOn(document, "visibilityState", "get").mockReturnValue("hidden");
  const host = vi.fn().mockResolvedValue(true);
  window.foxxycodeHostNotify = host;
  expect(
    await showHostNotification(
      {
        id: "minimized",
        sessionId: "s",
        title: "Chat",
        body: "Done",
        actions: [],
      },
      vi.fn(),
    ),
  ).toBe(true);
  expect(host).toHaveBeenCalledTimes(1);
});

test("native window focus also suppresses notifications while a host dialog has focus", async () => {
  vi.spyOn(document, "hasFocus").mockReturnValue(false);
  const host = vi.fn().mockResolvedValue(true);
  window.foxxycodeHostNotify = host;
  window.foxxycodeHostIsActive = vi.fn().mockResolvedValue(true);
  expect(
    await showHostNotification(
      {
        id: "host-dialog",
        sessionId: "s",
        title: "Chat",
        body: "Permission",
        actions: [],
      },
      vi.fn(),
    ),
  ).toBe(true);
  expect(host).not.toHaveBeenCalled();
});

test("native background state overrides stale webview focus", async () => {
  vi.spyOn(document, "hasFocus").mockReturnValue(true);
  const host = vi.fn().mockResolvedValue(true);
  window.foxxycodeHostNotify = host;
  window.foxxycodeHostIsActive = vi.fn().mockResolvedValue(false);
  expect(
    await showHostNotification(
      {
        id: "native-background",
        sessionId: "s",
        title: "Chat",
        body: "Done",
        actions: [],
      },
      vi.fn(),
    ),
  ).toBe(true);
  expect(host).toHaveBeenCalledTimes(1);
});

test("returning to an editor chat during delivery suppresses its chime", async () => {
  const focus = vi.spyOn(document, "hasFocus").mockReturnValue(false);
  document.documentElement.dataset.embed = "intellij";
  window.foxxycodeHostNotify = vi.fn().mockImplementation(async () => {
    focus.mockReturnValue(true);
    return true;
  });
  expect(
    await showHostNotification(
      {
        id: "sound-focus-return",
        sessionId: "s",
        title: "Chat",
        body: "Done",
        actions: [],
      },
      vi.fn(),
    ),
  ).toBe(true);
  expect(playNotificationSound).not.toHaveBeenCalled();
  dismissHostNotification("sound-focus-return");
});

test("returning to the chat during failed delivery suppresses the inline fallback", async () => {
  const focus = vi.spyOn(document, "hasFocus").mockReturnValue(false);
  window.foxxycodeHostNotify = vi.fn().mockImplementation(async () => {
    focus.mockReturnValue(true);
    throw new Error("notification service unavailable");
  });
  expect(
    await showHostNotification(
      {
        id: "focus-return",
        sessionId: "s",
        title: "Chat",
        body: "Permission",
        actions: [],
      },
      vi.fn(),
    ),
  ).toBe(true);
  expect(playNotificationSound).not.toHaveBeenCalled();
});

test("the native button limit never removes Reject from a long permission list", () => {
  const actions = [
    "allow",
    "allow_always",
    "program",
    "bypass",
    "accept_edits",
    "reject",
  ].map((id) => ({ id }));
  expect(limitNotificationActions(actions).map((a) => a.id)).toEqual([
    "allow",
    "allow_always",
    "program",
    "reject",
  ]);
});

test("desktop delivers native actions once and ignores stale actions", async () => {
  const host = vi.fn().mockResolvedValue(true);
  window.foxxycodeHostNotify = host;
  const choose = vi.fn();
  const open = vi.fn();
  await showHostNotification(
    {
      id: "p1",
      sessionId: "s1",
      title: "Chat",
      body: "Permission",
      actions: [{ id: "allow", label: "Allow", run: choose }],
    },
    open,
  );
  expect(host).toHaveBeenCalledWith(
    expect.objectContaining({
      id: "p1",
      silent: false,
      actions: [{ id: "allow", label: "Allow" }],
    }),
  );
  receiveNotificationAction("p1", "allow");
  receiveNotificationAction("p1", "allow");
  expect(choose).toHaveBeenCalledTimes(1);
  await showHostNotification(
    { id: "p2", sessionId: "s1", title: "Chat", body: "Question", actions: [] },
    open,
  );
  dismissHostNotification("p2");
  receiveNotificationAction("p2", "");
  expect(open).not.toHaveBeenCalled();
});

test("disabled notifications suppress delivery; muted notifications retain actions", async () => {
  const host = vi.fn().mockResolvedValue(true);
  window.foxxycodeHostNotify = host;
  const n = {
    id: "mute",
    sessionId: "s",
    title: "Chat",
    body: "Done",
    actions: [],
  };
  applyNotificationConfig({ ui: { notifications: false } });
  await showHostNotification(n, vi.fn());
  expect(host).not.toHaveBeenCalled();
  applyNotificationConfig({ ui: { notification_sound: false } });
  await showHostNotification(n, vi.fn());
  expect(host).toHaveBeenCalledWith(expect.objectContaining({ silent: true }));
});
