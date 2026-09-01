import { expect, test } from "vitest";
import {
  MCP_SERVER_TEMPLATE,
  parseServerEntryJson,
  serverRowToEntryJson,
  validateMCPServerName,
} from "./mcpServerJson";

test("template parses as a valid entry", () => {
  const { entry, error } = parseServerEntryJson(MCP_SERVER_TEMPLATE);
  expect(error).toBeUndefined();
  expect(entry?.command).toBeTruthy();
});

test("name validation mirrors the backend rules", () => {
  expect(validateMCPServerName("files")).toBeNull();
  expect(validateMCPServerName("my-server")).toBeNull();
  expect(validateMCPServerName("")).toBe(
    "settings.mcp.validation.nameRequired",
  );
  expect(validateMCPServerName("  ")).toBe(
    "settings.mcp.validation.nameRequired",
  );
  // "__" is the server/tool namespace separator.
  expect(validateMCPServerName("a__b")).toBe(
    "settings.mcp.validation.nameNamespace",
  );
  expect(validateMCPServerName("a b")).toBe("settings.mcp.validation.namePath");
  expect(validateMCPServerName("a/b")).toBe("settings.mcp.validation.namePath");
});

test("entry must be a JSON object with command or url", () => {
  expect(parseServerEntryJson("{broken").error).toBe(
    "settings.mcp.validation.invalidJson",
  );
  expect(parseServerEntryJson("[]").error).toBe(
    "settings.mcp.validation.objectRequired",
  );
  expect(parseServerEntryJson('"str"').error).toBe(
    "settings.mcp.validation.objectRequired",
  );
  expect(parseServerEntryJson("{}").error).toBe(
    "settings.mcp.validation.commandOrUrl",
  );
  expect(parseServerEntryJson('{"command":"npx"}').entry?.command).toBe("npx");
  expect(parseServerEntryJson('{"url":"https://x"}').entry?.url).toBe(
    "https://x",
  );
});

test("args must be strings, env must be a string map", () => {
  expect(parseServerEntryJson('{"command":"x","args":[1]}').error).toBeTruthy();
  expect(
    parseServerEntryJson('{"command":"x","env":{"A":1}}').error,
  ).toBeTruthy();
  const ok = parseServerEntryJson(
    '{"command":"x","args":["-y"],"env":{"A":"1"},"disabledTools":["t"]}',
  );
  expect(ok.error).toBeUndefined();
  expect(ok.entry?.args).toEqual(["-y"]);
  expect(ok.entry?.disabledTools).toEqual(["t"]);
});

test("serverRowToEntryJson round-trips through the parser", () => {
  const text = serverRowToEntryJson({
    name: "files",
    source: "local",
    origin: "project",
    transport: "stdio",
    command: "npx",
    args: ["-y", "pkg"],
    env: { TOKEN: "v" },
    enabled: false,
    status: "disabled",
    tools: [],
    disabled_tools: ["write"],
  });
  const { entry, error } = parseServerEntryJson(text);
  expect(error).toBeUndefined();
  expect(entry?.command).toBe("npx");
  expect(entry?.disabled).toBe(true);
  expect(entry?.disabledTools).toEqual(["write"]);
});

test("serverRowToEntryJson keeps remote transport, url, and headers", () => {
  const text = serverRowToEntryJson({
    name: "remote",
    source: "global",
    origin: "home",
    transport: "http",
    url: "https://mcp.example.com/mcp",
    headers: { Authorization: "Bearer tok" },
    enabled: true,
    status: "connected",
    tools: [],
  });
  const { entry, error } = parseServerEntryJson(text);
  expect(error).toBeUndefined();
  expect(entry?.type).toBe("http");
  expect(entry?.url).toBe("https://mcp.example.com/mcp");
  // Editing a remote server must not silently drop its auth headers.
  expect(entry?.headers).toEqual({ Authorization: "Bearer tok" });
});

test("originLabel names the owning file", async () => {
  const { originLabel } = await import("./mcpServerJson");
  expect(originLabel("config")).toBe("config.yaml");
  expect(originLabel("home")).toBe("~/.foxxycode/mcp.json");
  expect(originLabel("project")).toBe("./.foxxycode/mcp.json");
});

test("insecureSkipVerify parses as a boolean and is rejected otherwise", () => {
  const { entry, error } = parseServerEntryJson(
    '{"url":"https://selfsigned.local/mcp","insecureSkipVerify":true}',
  );
  expect(error).toBeUndefined();
  expect(entry?.insecureSkipVerify).toBe(true);

  expect(
    parseServerEntryJson('{"url":"https://x/mcp","insecureSkipVerify":"yes"}')
      .error,
  ).toBe("settings.mcp.validation.insecureSkipVerifyBoolean");
});

test("serverRowToEntryJson keeps insecure_skip_verify so editing cannot erase it", () => {
  const row = {
    name: "remote",
    source: "global" as const,
    origin: "home" as const,
    transport: "http",
    url: "https://selfsigned.local/mcp",
    insecure_skip_verify: true,
    enabled: true,
    status: "connected" as const,
    tools: [],
  };
  const { entry } = parseServerEntryJson(serverRowToEntryJson(row));
  expect(entry?.insecureSkipVerify).toBe(true);

  // An unset flag stays out of the file rather than becoming an explicit false.
  const { entry: plain } = parseServerEntryJson(
    serverRowToEntryJson({ ...row, insecure_skip_verify: false }),
  );
  expect(plain?.insecureSkipVerify).toBeUndefined();
});

test("serverRowToEntry returns the object the checkbox handler edits", async () => {
  const { serverRowToEntry } = await import("./mcpServerJson");
  const entry = serverRowToEntry({
    name: "remote",
    source: "global",
    origin: "home",
    transport: "http",
    url: "https://selfsigned.local/mcp",
    headers: { Authorization: "Bearer tok" },
    enabled: true,
    status: "connected",
    tools: [],
  });
  expect(entry.url).toBe("https://selfsigned.local/mcp");
  expect(entry.headers).toEqual({ Authorization: "Bearer tok" });
});

test("a server that skips certificate verification says so before approval", async () => {
  const { declarationFacts } = await import("./mcpServerJson");
  const facts = declarationFacts({
    name: "remote",
    source: "local",
    origin: "project",
    transport: "http",
    url: "https://selfsigned.local/mcp",
    insecure_skip_verify: true,
    enabled: true,
    status: "connected",
    tools: [],
  });
  expect(facts.some((f) => f.labelKey === "settings.mcp.trust.fact.tls")).toBe(
    true,
  );
});
