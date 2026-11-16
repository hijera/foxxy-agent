import { beforeEach, expect, test } from "vitest";
import { setEnv } from "../env/remoteEnv";
import { readCollapsedSessionGroups, writeCollapsedSessionGroups } from "./collapsedSessionGroups";

beforeEach(() => {
  setEnv({ mode: "local" });
  localStorage.clear();
  for (const cookie of document.cookie.split(";")) {
    document.cookie = `${cookie.split("=")[0]?.trim()}=; Path=/; Max-Age=0`;
  }
});

test("collapsed groups survive a new IDE port without mixing remote environments", () => {
  writeCollapsedSessionGroups(new Set(["cwd:/project/a", "cwd:/project/b"]));
  localStorage.clear();
  expect([...readCollapsedSessionGroups()]).toEqual(["cwd:/project/a", "cwd:/project/b"]);
  setEnv({ mode: "remote", baseUrl: "https://other.example", token: "" });
  expect(readCollapsedSessionGroups().size).toBe(0);
  setEnv({ mode: "local" });
  expect(readCollapsedSessionGroups().has("cwd:/project/a")).toBe(true);
});
