import { expect, test } from "vitest";
import { chatNotificationFromMessage } from "../src/webview/chatNotification";

test("editor notifications retain action IDs, without interpreting commands", () => {
  const message = {
    type: "foxxycode:notification",
    notification: {
      id: "one",
      title: "FoxxyCode — chat",
      body: "Allow?",
      silent: true,
      actions: [{ id: "allow", label: "Allow" }],
    },
  };
  expect(chatNotificationFromMessage(message)).toEqual(message.notification);
  expect(chatNotificationFromMessage({ ...message, type: "other" })).toBeNull();
  expect(
    chatNotificationFromMessage({
      ...message,
      notification: {
        ...message.notification,
        actions: [
          { id: "allow", label: "Allow" },
          { id: "allow", label: "Again" },
        ],
      },
    }),
  ).toBeNull();
  expect(chatNotificationFromMessage(null)).toBeNull();
});

test("oversized or malformed payloads are rejected or capped", () => {
  const base = {
    type: "foxxycode:notification",
    notification: {
      id: "one",
      title: "Chat",
      body: "Allow?",
      actions: [],
    },
  };
  expect(
    chatNotificationFromMessage({
      ...base,
      notification: { ...base.notification, id: "x".repeat(257) },
    }),
  ).toBeNull();
  expect(
    chatNotificationFromMessage({
      ...base,
      notification: {
        ...base.notification,
        actions: Array.from({ length: 6 }, (_, i) => ({
          id: `a${i}`,
          label: "A",
        })),
      },
    }),
  ).toBeNull();
  expect(
    chatNotificationFromMessage({
      ...base,
      notification: { ...base.notification, title: 42, body: "Allow?" },
    }),
  ).toBeNull();
  expect(
    chatNotificationFromMessage({
      ...base,
      notification: { id: "one", body: "Allow?", actions: [] },
    }),
  ).toBeNull();
  const capped = chatNotificationFromMessage({
    ...base,
    notification: {
      id: "one",
      title: "t".repeat(300),
      body: "b".repeat(600),
      actions: [{ id: "allow", label: "l".repeat(200) }],
    },
  });
  expect(capped?.title.length).toBe(256);
  expect(capped?.body.length).toBe(500);
  expect(capped?.actions[0].label.length).toBe(120);
});
