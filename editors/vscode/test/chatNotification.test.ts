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
