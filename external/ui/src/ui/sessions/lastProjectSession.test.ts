import { describe, expect, it } from "vitest";

import type { ParsedAppHash } from "../scheduler/hashRoute";
import { shouldRestoreLastSession } from "./lastProjectSession";

describe("shouldRestoreLastSession", () => {
  const branches: Array<ParsedAppHash["branch"]> = [
    "none",
    "session",
    "draft",
    "history",
    "scheduler",
    "settings",
  ];

  it("restores only on the bare route inside an editor embed", () => {
    for (const branch of branches) {
      expect(shouldRestoreLastSession({ embed: true, branch })).toBe(
        branch === "none",
      );
    }
  });

  it("never restores outside an editor embed", () => {
    for (const branch of branches) {
      expect(shouldRestoreLastSession({ embed: false, branch })).toBe(false);
    }
  });
});
