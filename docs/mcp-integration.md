# MCP Server Integration

## Overview

The agent supports connecting to external MCP (Model Context Protocol) servers, which provide
additional tools and resources. MCP servers can be configured at these levels:

1. **Global** (scope `global`) - `mcp_servers` in `config.yaml` and the user-global
   `~/.foxxycode/mcp.json` (the analogue of Cursor's `~/.cursor/mcp.json`), connected for every
   session; entries in `~/.foxxycode/mcp.json` override same-named `config.yaml` entries
2. **Local** (scope `local`) - `<workspace>/.foxxycode/mcp.json`, merged over the global list for
   sessions in that workspace; a local entry with the same name overrides the global definition
3. **Per-session** - provided by the ACP client in `session/new` parameters

Tools from all connected MCP servers are merged into the tool list passed to the LLM during
the ReAct loop (in **`agent`** and **`plan`** modes). Ask exposes only tools whose MCP definition
declares **`annotations.readOnlyHint: true`**, and hides all MCP tools when
**`tools.ask_disable_extended_tools`** is enabled.

## mcp.json (global and local)

Both mcp.json files use the same shape as Cursor's: a single `mcpServers` object keyed by
server name. `env` and `headers` are JSON objects (not the YAML name/value list), and
per-tool switches use `disabledTools`:

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "${CWD}"],
      "env": { "SOME_TOKEN": "value" },
      "disabled": false,
      "disabledTools": ["write_file"]
    }
  }
}
```

A broken `mcp.json` is logged and skipped; the session still starts with the remaining levels.

## Workspace trust for project-local servers

`<workspace>/.foxxycode/mcp.json` travels with the checkout, so the **repository** — not the
operator — picks the command, arguments, and environment of a process FoxxyCode would start
while bootstrapping a session. Nothing on the path to that spawn can make a trust decision:
it happens before the first turn exists, so there is no model request, no permission prompt,
and no `tools.permission_mode` to consult. Opening someone else's repository would otherwise
mean running whatever it declared.

Project-origin declarations therefore pass a trust gate before any spawn or connect.
`config.yaml` and `<home>/mcp.json` are operator-authored and stay ungated.

`mcp.project_trust` in `config.yaml` chooses the policy:

| Value | Behaviour |
|---|---|
| `ask` (default) | A project server stays cold until the operator approves **that exact declaration** for **that workspace**. |
| `allow` | Project servers start automatically. Use only for workspaces you already trust. |
| `deny` | Project servers are never loaded and there is no approval path. |

Override it for one process with `foxxycode acp --mcp-project-trust <value>` or
`foxxycode http --mcp-project-trust <value>`, which is what CI jobs and container entrypoints
use instead of editing the config file. An unknown value fails the launch rather than falling
back to the default.

### Approving

Three surfaces make the same decision, and all of them print the effective declaration first —
transport, the command with its arguments (or the URL), the **names** of the environment
variables and headers it carries, the workspace, and the source file:

- `foxxycode mcp list` — every merged server with its trust state; `foxxycode mcp trust <name>`
  and `foxxycode mcp untrust <name>` decide. `--cwd DIR` picks the workspace.
- `POST /foxxycode/mcp/{name}/trust` and `/untrust` over the HTTP API.
- The shield button in **Settings → MCP servers**, which renders only under `ask` (the other
  two policies leave no per-server decision to make).

Writing an entry through the management API or the UI editor approves it: the operator typed
the command themselves.

Approvals live in `<home>/mcp-trust.json`, keyed by canonical workspace path and bound to a
SHA-256 digest of the command-bearing declaration (transport, command, args, env, url,
headers). Operational switches — `disabled`, `disabledTools` — are deliberately **outside**
the digest, so toggling a tool off does not withdraw an approval, while editing the command
line does. Records are receipts of what was approved and store env and header **names** only:
their values routinely hold secrets, and the decision rests on which variables travel to the
child, not on what is in them.

Listing servers is gated too: probing an unapproved entry would start exactly the command the
approval is about, so `GET /foxxycode/mcp` reports it (`status: needs_approval`) instead.

## Enable / disable switches

Every config level supports switching off a whole server or individual tools without
removing their definitions:

- `config.yaml`: `disabled: true` and `disabled_tools: ["tool_a"]` per `mcp_servers` entry
- `~/.foxxycode/mcp.json` and `./.foxxycode/mcp.json`: `"disabled": true` and
  `"disabledTools": ["tool_a"]` per entry

Disabled servers are not connected for new sessions. Disabled tools (and all tools of a
disabled server) are hidden from the LLM's tool list and rejected at dispatch. The switches
are re-read on every agent turn, so toggling them (by editing the files or through the
HTTP API / web UI below) also applies to **already running** sessions on their next turn.

## Management API and UI

The HTTP gateway (build tag `http`) exposes the merged server list with probed tool
inventories and toggle endpoints under **`/foxxycode/mcp*`** (see `docs/http-api.md`), and the
bundled web UI shows them under **Settings -> MCP servers**: status dot per server, a
`global` / `local` scope badge, expandable tool list with per-tool switches, and a
Cursor-style JSON editor for mcp.json entries with a scope picker (global writes
`~/.foxxycode/mcp.json`, local writes `./.foxxycode/mcp.json`). Toggles persist into the file that
defines the server; `config.yaml` entries are toggle-only here and edited in Settings.

Above the list sits an **MCP discovery** fieldset carrying `mcp.project_trust`
(`POST /foxxycode/mcp/project-trust`). It lives in this tab rather than in a settings section
of its own because it governs exactly the servers listed below it, and it persists straight
into `config.yaml` instead of joining the settings-document save flow.

## Supported Transports

### stdio (supported)

The MCP server runs as a subprocess. Communication via stdin/stdout (newline-delimited
JSON-RPC 2.0).

Configuration in `session/new`:
```json
{
  "name": "my-server",
  "command": "/path/to/mcp-server",
  "args": ["--stdio"],
  "env": [
    { "name": "API_KEY", "value": "secret" }
  ]
}
```

Configuration in `config.yaml`:
```yaml
mcp_servers:
  - name: "filesystem"
    command: "npx"
    args: ["-y", "@modelcontextprotocol/server-filesystem", "/home/user/projects"]
    env: []
```

### Streamable HTTP (supported)

`type: http` connects to `url` over the
MCP streamable HTTP transport (2025-03-26 spec): JSON-RPC messages are POSTed to the
endpoint and answered as `application/json` bodies or `text/event-stream` chunks; the
`Mcp-Session-Id` issued on initialize is echoed on subsequent requests. `headers` are sent
with every request (e.g. `Authorization`). URL-only entries (no `command`, no `type`)
default to `http`. When the endpoint rejects the handshake (legacy servers answer POST
with 4xx), the client automatically falls back to the legacy SSE transport at the same
URL, mirroring Cursor and Claude Code behavior. The agent advertises
`mcpCapabilities.http: true` and `mcpCapabilities.sse: true`. In YAML the `type` value
must be `stdio`, `http`, or `sse`; mcp.json and ACP entries additionally accept the
`streamable-http` / `streamable_http` aliases for `http`.

```yaml
mcp_servers:
  - name: "remote-tools"
    type: "http"
    url: "https://mcp.example.com/mcp"
    headers:
      - name: "Authorization"
        value: "Bearer ${MCP_TOKEN}"
```

### SSE (supported, legacy)

`type: sse` forces the 2024-11-05 HTTP+SSE transport: a GET stream at `url` announces the
POST endpoint in its first event and then carries every server-to-client message. Use it
for servers that only implement the older protocol; `type: http` reaches them too via the
automatic fallback.

In `.foxxycode/mcp.json` the same entries look like Cursor's:

```json
{
  "mcpServers": {
    "remote-tools": { "url": "https://mcp.example.com/mcp" },
    "legacy-tools": { "type": "sse", "url": "https://old.example.com/sse" }
  }
}
```

## Tool Namespacing

To avoid conflicts when multiple MCP servers provide tools with the same name,
tools are namespaced using the server name:

- MCP server `filesystem` providing tool `read_file` -> available as `filesystem__read_file`
- Built-in tool `read_file` -> available as `read_file`

Because `__` separates the server and tool parts, server names must not contain `__`
(the management API rejects such names).

## How tools reach the model

MCP tools are **not** injected into the system prompt text (unlike skills, which render
into a prompt section). They join the built-in tools in the native function-calling
`tools` array of every LLM request, one definition per enabled tool: name
`server__tool`, the server's own `inputSchema`, and the description prefixed with
`[server]` so the model can tell providers apart. When the model emits a tool call whose
name contains `__`, the agent routes it to the owning server over its transport and
returns the MCP result to the model as a regular tool observation - the same loop as
built-in tools, and the same approach Claude Code, Codex, and Cursor use. The end-to-end
happy path (two servers over stdio and streamable HTTP, the model picking one by its
namespaced tool, the result landing in the final answer) is specified in
`features/mcp_tool_calls.feature` for both the OpenAI-compatible HTTP surface and the
ACP session flow.

## Permission Model

MCP tool calls are currently dispatched without the built-in permission prompts that guard
filesystem writes and shell commands; the disable switches above are the mechanism for
restricting what a server may do. Prefer running MCP servers with least-privilege
credentials and disabling tools you do not need.

## Popular MCP Servers

### Filesystem access
```yaml
mcp_servers:
  - name: "filesystem"
    command: "npx"
    args: ["-y", "@modelcontextprotocol/server-filesystem", "${CWD}"]   # session cwd when the server starts
```

### GitHub
```yaml
mcp_servers:
  - name: "github"
    command: "npx"
    args: ["-y", "@modelcontextprotocol/server-github"]
    env:
      - name: "GITHUB_PERSONAL_ACCESS_TOKEN"
        value: "${GITHUB_TOKEN}"
```

### Postgres database
```yaml
mcp_servers:
  - name: "postgres"
    command: "npx"
    args: ["-y", "@modelcontextprotocol/server-postgres", "${DATABASE_URL}"]
```

### Brave Search
```yaml
mcp_servers:
  - name: "brave-search"
    command: "npx"
    args: ["-y", "@modelcontextprotocol/server-brave-search"]
    env:
      - name: "BRAVE_API_KEY"
        value: "${BRAVE_API_KEY}"
```

## MCP Server Lifecycle

1. On `session/new` (and on `session/load`, and on a workspace switch), the agent starts
   connecting every enabled server from the merged config.yaml + `~/.foxxycode/mcp.json` +
   `./.foxxycode/mcp.json` list, then any ACP client-supplied servers. Configured servers
   connect **in the background and in parallel**, so creating or loading a session never waits
   on them — over HTTP that call sits on a request an editor panel is blocked on, and a cold
   `npx` server that downloads its package on first run used to hold it for minutes. Each
   handshake (transport setup + `initialize` + `tools/list`) is bounded at **30 s**.
2. A prompt turn — the only thing that needs the tools — waits for that connect to settle
   before it builds its tool set. While it waits, HTTP clients get `event: mcp_phase`
   (`connecting`, then `ready`) so the panel can say what is holding the turn up; a session
   whose servers are already connected sends nothing and starts immediately.
3. The agent calls `tools/list` on each server and registers the tools
4. Saving settings with a changed `mcp_servers` list reconnects the configured
   servers for every active session; ACP client-supplied per-session servers
   stay connected. The reconnect is a **fresh trust evaluation**, not a replay of
   what the session started with: an unapproved project declaration stays cold,
   and one whose approval was withdrawn in the meantime does not come back. A
   session with a **turn in flight** is not swapped mid-turn — that turn already
   handed the model a tool list, so the reload is parked and applied the moment
   the turn releases its lock. All sessions share one deadline per save, and a
   dial it cuts short is discarded rather than installed: a server hanging in
   the session reconnected first must not strip the others of their tools. Those
   sessions keep the servers they have and retry on their next turn
5. During the ReAct loop, when LLM calls an MCP tool, the agent forwards the call
   (unless the tool or its server has been disabled since)
6. Results are returned to the LLM as tool observations
7. On session end or `session/cancel`, MCP server connections are cleaned up

## Error Handling

- If an MCP server fails to start, the session still proceeds with a warning
- Failed MCP tool calls return an error observation to the LLM
- The LLM can decide to retry, use alternative tools, or inform the user
