import { expect, test } from "vitest";

import {
  parseCoddyPermissionPayload,
  permissionBodyText,
} from "./permissionTypes";

test("parseCoddyPermissionPayload accepts composer SSE shape", () => {
  const p = parseCoddyPermissionPayload({
    sessionId: "sess_abc",
    toolCall: {
      toolCallId: "call_1",
      title: "Run: run_command",
      content: [
        {
          type: "content",
          content: { type: "text", text: "Execute: echo hi" },
        },
      ],
    },
    options: [
      { optionId: "allow", name: "Allow", kind: "allow_once" },
      { optionId: "reject", name: "Reject", kind: "reject_once" },
    ],
  });
  expect(p).toBeTruthy();
  expect(p?.toolCall.toolCallId).toBe("call_1");
  expect(permissionBodyText(p!)).toBe("Execute: echo hi");
});
