import { describe, expect, it } from "vitest";
import { projectBasename } from "./projectApi";

describe("projectBasename", () => {
  it("handles windows and posix separators", () => {
    expect(projectBasename("H:\\PycharmProjects\\foxxy-agent")).toBe(
      "foxxy-agent",
    );
    expect(projectBasename("/home/user/proj")).toBe("proj");
    expect(projectBasename("/home/user/proj/")).toBe("proj");
    expect(projectBasename("plain")).toBe("plain");
    expect(projectBasename("")).toBe("");
  });
});
