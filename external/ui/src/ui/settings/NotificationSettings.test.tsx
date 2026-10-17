import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, test, vi } from "vitest";
import { NotificationSettings } from "./NotificationSettings";
import {
  applyNotificationConfig,
  getNotificationPreferences,
} from "../desktop/notificationConfig";

afterEach(() => {
  delete window.foxxycodeHostPlaySound;
  cleanup();
  vi.unstubAllGlobals();
  applyNotificationConfig({});
});

test("muting sound persists independently and keeps unrelated configuration", async () => {
  const saved: Record<string, unknown>[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url, init) => {
      if (init?.method === "PUT") {
        saved.push(JSON.parse(init.body));
        return { ok: true, json: async () => ({ ok: true }) };
      }
      return {
        ok: true,
        json: async () => ({
          providers: [{ name: "keep" }],
          ui: { locale: "ru", notifications: true },
        }),
      };
    }),
  );
  function Harness() {
    const [doc, setDoc] = useState<Record<string, unknown>>({ ui: {} });
    return <NotificationSettings doc={doc} setDoc={setDoc} />;
  }
  render(<Harness />);
  fireEvent.click(screen.getByTestId("settings-notification_sound"));
  await waitFor(() => expect(saved).toHaveLength(1));
  expect(saved[0]).toEqual({
    providers: [{ name: "keep" }],
    ui: { locale: "ru", notifications: true, notification_sound: false },
  });
  await waitFor(() =>
    expect(getNotificationPreferences()).toEqual({
      enabled: true,
      sound: false,
      whenFocused: false,
      soundFile: "",
    }),
  );
});

test("foreground alerts default to off and can be persisted without changing sound", async () => {
  const saved: Record<string, unknown>[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url, init) => {
      if (init?.method === "PUT") {
        saved.push(JSON.parse(init.body));
        return { ok: true, json: async () => ({ ok: true }) };
      }
      return {
        ok: true,
        json: async () => ({ ui: { notification_sound: false, locale: "ru" } }),
      };
    }),
  );
  function Harness() {
    const [doc, setDoc] = useState<Record<string, unknown>>({
      ui: { notification_sound: false },
    });
    return <NotificationSettings doc={doc} setDoc={setDoc} />;
  }
  render(<Harness />);
  const control = screen.getByTestId("settings-notifications_when_focused");
  expect(control).not.toBeChecked();
  fireEvent.click(control);
  await waitFor(() => expect(control).toBeChecked());
  expect(saved[0]).toEqual({
    ui: {
      notification_sound: false,
      locale: "ru",
      notifications_when_focused: true,
    },
  });
  expect(getNotificationPreferences()).toEqual({
    enabled: true,
    sound: false,
    whenFocused: true,
    soundFile: "",
  });
});

test("a failed save keeps the previous preference and reports the failure", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url, init) =>
      init?.method === "PUT"
        ? { ok: false, status: 500, json: async () => ({ ok: false }) }
        : { ok: true, json: async () => ({ ui: {} }) },
    ),
  );
  render(<NotificationSettings doc={{ ui: {} }} setDoc={vi.fn()} />);
  fireEvent.click(screen.getByTestId("settings-notifications"));
  expect(await screen.findByRole("alert")).toBeInTheDocument();
  expect(getNotificationPreferences().enabled).toBe(true);
});

test("a selected sound uploads, persists and can be reset independently", async () => {
  window.foxxycodeHostPlaySound = vi.fn().mockResolvedValue(undefined);
  const saved: Record<string, unknown>[] = [];
  const path = "C:/home/notification-sounds/alert.wav";
  const fetcher = vi.fn(async (url, init) => {
    if (init?.method === "POST")
      return { ok: true, json: async () => ({ file: path }) };
    if (init?.method === "PUT") {
      saved.push(JSON.parse(init.body));
      return { ok: true, json: async () => ({ ok: true }) };
    }
    return {
      ok: true,
      json: async () => ({
        ui: { notification_sound: false, notifications_when_focused: true },
      }),
    };
  });
  vi.stubGlobal("fetch", fetcher);
  function Harness() {
    const [doc, setDoc] = useState<Record<string, unknown>>({
      ui: { notification_sound: false },
    });
    return <NotificationSettings doc={doc} setDoc={setDoc} />;
  }
  render(<Harness />);
  const file = new File(["RIFF audio"], "alert.wav", { type: "audio/wav" });
  fireEvent.change(screen.getByTestId("notification-sound-file"), {
    target: { files: [file] },
  });
  await waitFor(() => expect(saved).toHaveLength(1));
  expect(fetcher).toHaveBeenCalledWith(
    "/foxxycode/notification-sound?filename=alert.wav",
    expect.objectContaining({ method: "POST", body: file }),
  );
  expect(saved[0]).toEqual({
    ui: {
      notification_sound: false,
      notifications_when_focused: true,
      notification_sound_file: path,
    },
  });
  expect(screen.getByText("alert.wav")).toBeInTheDocument();
  await waitFor(() =>
    expect(screen.getByRole("radio", { name: "Custom sound" })).toBeChecked(),
  );
  fireEvent.click(
    screen.getByRole("radio", { name: "Windows system sound (ding)" }),
  );
  await waitFor(() => expect(saved).toHaveLength(2));
  expect(
    (saved[1]!.ui as Record<string, unknown>).notification_sound_file,
  ).toBe("");
  await waitFor(() =>
    expect(
      screen.getByRole("radio", { name: "Windows system sound (ding)" }),
    ).toBeChecked(),
  );
  fireEvent.click(screen.getByRole("radio", { name: "Custom sound" }));
  await waitFor(() => expect(saved).toHaveLength(3));
  expect(
    (saved[2]!.ui as Record<string, unknown>).notification_sound_file,
  ).toBe(path);
  expect(screen.getByRole("radio", { name: "Custom sound" })).toBeChecked();
});

test("the Windows default sound is selected when no custom file is configured", () => {
  window.foxxycodeHostPlaySound = vi.fn().mockResolvedValue(undefined);
  render(<NotificationSettings doc={{ ui: {} }} setDoc={vi.fn()} />);
  expect(
    screen.getByRole("radio", { name: "Windows system sound (ding)" }),
  ).toBeChecked();
  expect(screen.getByRole("radio", { name: "Custom sound" })).not.toBeChecked();
});
