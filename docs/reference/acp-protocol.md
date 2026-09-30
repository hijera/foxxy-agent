# ACP Protocol Reference

## Overview

FoxxyCode implements ACP as the **wire contract for the harness**. ACP standardizes how clients (for example editors, scripts, or orchestrators) talk to an agent process. The stock configuration presents a **coding agent**, but transports and RPC methods are generic harness surface area - initialize, session lifecycle, `session/prompt`, permission flows, and MCP-related options.

Reference: https://agentclientprotocol.com/protocol/overview

## Transport

All messages are newline-delimited JSON objects sent via **stdin/stdout**.

```
stdin  -> messages from Client to Agent
stdout -> messages from Agent to Client (responses + notifications)
stderr -> agent logs (not protocol messages)
```

## Message Types

### Request (Client to Agent)

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "session/prompt",
  "params": { ... }
}
```

### Response (Agent to Client)

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": { ... }
}
```

### Error Response

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "code": -32600,
    "message": "Invalid request"
  }
}
```

### Notification (Agent to Client, no response expected)

```json
{
  "jsonrpc": "2.0",
  "method": "session/update",
  "params": { ... }
}
```

## Stdio clients (FoxxyCode-specific)

Hand-written scripts that drive **`foxxycode acp`** over a pipe should implement the following behaviors. Reference harness: **`examples/acp/acp_e2e_todo.py`**.

1. **Nil `result` and `omitempty`** - JSON-RPC success payloads are produced with `result` omitted when the Go handler returns a **`nil`** pointer (for example **`session/set_mode`**). A response line may contain only **`jsonrpc`**, **`id`**, and neither **`result`** nor **`error`**. Treat any object with a matching **`id`** and no **`method`** as the completion of your outstanding request.
2. **Interleaved `session/update`** - After **`session/prompt`**, the agent streams many notifications before the final response. Read stdout line by line until the line for your request **`id`** arrives; handle **`session/request_permission`** or **`session/request_question`** in between by writing a client response with the same **`id`**.
3. **Stdout buffering** - When stdout is not a TTY, output can be block-buffered. Wrap the binary with **`stdbuf -oL -eL`** (or equivalent) so lines appear as they are written.
4. **Concurrent request handlers** - Outstanding requests are dispatched asynchronously. Do not send a second RPC until you have consumed the response for the previous one if your client assumes strict ordering.

## Protocol Flow

```
Client                          Agent
  |                               |
  |-------- initialize ---------->|
  |<------- initialize resp ------|
  |                               |
  |-------- session/new --------->|
  |<------- session/new resp -----|
  |                               |
  |-------- session/prompt ------>|
  |<------- session/update -------|  (notifications: plan, available_commands_update, chunks, tool_calls)
  |<------- session/update -------|
  |<------- session/update -------|
  |<------- session/prompt resp --|  (stopReason: end_turn)
  |                               |
```

## Methods

### `initialize`

Negotiate protocol version and exchange capabilities.

**Request params:**
```json
{
  "protocolVersion": 1,
  "clientCapabilities": {
    "fs": {
      "readTextFile": true,
      "writeTextFile": true
    },
    "terminal": true
  },
  "clientInfo": {
    "name": "example-acp-client",
    "title": "Example ACP Client",
    "version": "1.0.0"
  }
}
```

**Response result:**
```json
{
  "protocolVersion": 1,
  "agentCapabilities": {
    "loadSession": true,
    "sessionCapabilities": {
      "list": {}
    },
    "promptCapabilities": {
      "image": false,
      "audio": false,
      "embeddedContext": true
    },
    "mcpCapabilities": {
      "http": true,
      "sse": false
    }
  },
  "agentInfo": {
    "name": "foxxy-agent",
    "title": "FoxxyCode Agent",
    "version": "0.1.0"
  },
  "authMethods": []
}
```

### `session/new`

Create a new conversation session.

**Request params:**
```json
{
  "cwd": "/home/user/project",
  "mcpServers": [
    {
      "name": "my-mcp",
      "command": "/path/to/mcp-server",
      "args": ["--stdio"],
      "env": []
    }
  ]
}
```

**Response result:**

FoxxyCode returns both **Session Config Options** (preferred by modern ACP clients) and the legacy **`modes`** field for compatibility. Clients that support `configOptions` should use them for mode and model selection.

```json
{
  "sessionId": "sess_abc123def456",
  "configOptions": [
    {
      "id": "mode",
      "name": "Session mode",
      "description": "Agent executes tasks; Plan designs; Docs maintains markdown; Ask answers questions read-only; Debug diagnoses before fixing.",
      "category": "mode",
      "type": "select",
      "currentValue": "agent",
      "options": [
        {
          "value": "agent",
          "name": "Agent",
          "description": "Execute tasks with full tool access"
        },
        {
          "value": "plan",
          "name": "Plan",
          "description": "Plan and design without code execution"
        },
        {
          "value": "docs",
          "name": "Docs",
          "description": "Generate and update project documentation"
        },
        {
          "value": "ask",
          "name": "Ask",
          "description": "Answer questions with read-only research tools"
        },
        {
          "value": "debug",
          "name": "Debug",
          "description": "Diagnose issues systematically before fixing them"
        }
      ]
    },
    {
      "id": "model",
      "name": "Model",
      "description": "LLM used for this session.",
      "category": "model",
      "type": "select",
      "currentValue": "openai/gpt-4o",
      "options": [
        {
          "value": "openai/gpt-4o",
          "name": "gpt-4o",
          "description": "openai"
        }
      ]
    },
    {
      "id": "permission_mode",
      "name": "Permission mode",
      "description": "Controls when the agent asks for user approval before running tools.",
      "category": "permissions",
      "type": "select",
      "currentValue": "ask",
      "options": [
        {
          "value": "ask",
          "name": "Ask",
          "description": "Always ask before running commands or writing files"
        },
        {
          "value": "accept_edits",
          "name": "Accept edits",
          "description": "Auto-approve file writes; ask before running commands"
        },
        {
          "value": "bypass",
          "name": "Bypass",
          "description": "Never ask for permission"
        }
      ]
    }
  ],
  "modes": {
    "currentModeId": "agent",
    "availableModes": [
      {
        "id": "agent",
        "name": "Agent",
        "description": "Execute tasks with full tool access"
      },
      {
        "id": "plan",
        "name": "Plan",
        "description": "Plan and design without code execution"
      },
      {
        "id": "docs",
        "name": "Docs",
        "description": "Generate and update project documentation"
      },
      {
        "id": "ask",
        "name": "Ask",
        "description": "Answer questions with read-only research tools"
      },
      {
        "id": "debug",
        "name": "Debug",
        "description": "Diagnose issues systematically before fixing them"
      }
    ]
  }
}
```

The `model` option is present only when the `models` list in the agent config is non-empty. The effective default model is `agent.model` until the user picks another model in the client, as described in [Configuration](../getting-started/configuration.md). Each listed `value` matches the YAML `models[].model` string (`provider_name/api_model_id`).

### `session/load`

Reloads a persisted session by `sessionId`. The agent restores `session.json` and `messages.json`, rebuilds skills and MCP connections from the request, replays prior user and assistant turns (and tool call summaries) via `session/update`, sends a `plan` update if `todos/active.md` exists, and sends `available_commands_update` once the response is on the wire. A replayed user message reads as it was typed: the attachments its mentions brought are collapsed back to those mentions (`@src/app.go:3-5`), never their bodies.

The replay precedes the response here, as ACP requires, and that is safe because the client named the session itself. Reopening a bundle through **`session/new`** (`foxxycode acp --session-id <id>`) is the other way round: the client only learns the id from the response, so the replay waits for it. Anything written earlier would arrive for a session the client has not registered.

**Request params** (per ACP, `cwd`, `sessionId`, and `mcpServers` are required):

```json
{
  "sessionId": "sess_abc123def456",
  "cwd": "/home/user/project",
  "mcpServers": []
}
```

**Response result:** `modes` and `configOptions` like `session/new`.

### `session/list`

Lists persisted sessions found under the configured sessions root (see README), newest first. The response includes `sessionId`, `cwd`, `title`, and `updatedAt` per entry; child sessions of subagent runs and scheduler runs are omitted.

The optional `cwd` narrows the list to one workspace. It names a folder, not a string: the path is cleaned, its symlinks are resolved when the folder exists, and Windows and macOS compare it case-insensitively. A session keeps the cwd its client gave it, and two clients often spell the same folder differently - the console stores the logical `$PWD` of a symlinked checkout while an editor sends the physical path it resolved, and a Windows client may lower-case the drive letter - so a session created by one is still listed for the other. Sessions of a parent or a child folder are not included: a console started in a subfolder of the workspace is not among the workspace's sessions. `foxxycode sessions list --cwd` and `GET /foxxycode/sessions?cwd=` apply the same rule.

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "session/list",
  "params": { "cwd": "/home/user/project" }
}
```

### Disk layout (FoxxyCode)

When the process is started with a writable sessions root (default **`$FOXXYCODE_HOME/sessions`**), each bundle is `<root>/<sessionId>/` with:

- `session.json` - id, cwd, mode, model override, permission mode override (`permissionMode`), agent memory, derived or pinned title (`titlePinned`), timestamps, optional **`activitySeq`** / **`readActivitySeq`** for composer unread sync across HTTP surfaces
- `messages.json` - LLM message history (roles user, assistant, tool)
- `assets/` - reserved for future session-scoped files
- `todos/active.md` - current todo checklist synced from plan tools
- `todos/archive/todo-<nanos>.md` - archived list when a completed list is replaced
- `plans/<slug>.plan.md` - design plan files (YAML frontmatter + markdown body), written in plan mode via **`plan_write`**

The server always advertises **`loadSession`** when a store is configured (`foxxycode acp` and **`foxxycode http`** open a **`FileStore`** at startup).

### Design plans (plan mode)

Plan mode uses standard ACP only. The agent saves files with tools **`plan_write`** / **`plan_list`** (not JSON-RPC extensions).

After **`plan_write`**, FoxxyCode publishes:

- **`session/update`** with `sessionUpdate: "plan"` and checklist **`entries`** from the file frontmatter (preview for any ACP client)
- **`_meta`** on that update: `foxxycode.dev/planSlug`, `foxxycode.dev/planKind: "design"` (opt-in; distinguishes design plans from live todo checklist updates)
- A persisted **`plan_document`** row in **`messages.json`** for the bundled UI (also visible as assistant markdown in chat when the model summarizes the plan)

**Run plan** (start implementation) without a custom `_foxxycode/*` method:

1. **FoxxyCode-aware** - `session/prompt` with `_meta`:

```json
{
  "sessionId": "sess_…",
  "prompt": [{ "type": "text", "text": "Implement the plan." }],
  "_meta": { "foxxycode.dev/runPlanSlug": "my-feature" }
}
```

FoxxyCode switches to **agent** mode, injects the plan body into the system prompt, and runs the turn. Session todo (`todos/active.md`) is **not** auto-filled from the design plan. In **ask** mode the hook is refused with an error and a `@plans/<slug>.plan.md` mention is only inlined as reading material: switch the mode first, then run.

2. **Portable** - client sets `mode` to **agent**, then `session/prompt` referencing `@plans/<slug>.plan.md` or text like *implement the plan my-feature*.

HTTP **`POST /v1/responses`** accepts the same hook via JSON **`metadata.runPlanSlug`** (bundled UI). CRUD for plan files is HTTP-only under **`/foxxycode/sessions/{id}/plans`** (not part of core ACP).

### `session/prompt`

Send a user message, starts the ReAct loop.

**Request params:**
```json
{
  "sessionId": "sess_abc123def456",
  "prompt": [
    {
      "type": "text",
      "text": "Refactor the auth module to use JWT"
    }
  ],
  "_meta": {
    "foxxycode.dev/runPlanSlug": "optional-slug-for-run-plan"
  }
}
```

**Response result:**
```json
{
  "stopReason": "end_turn"
}
```

Stop reasons: `end_turn` | `max_tokens` | `max_turns` | `agent_refused` | `cancelled`

**Mentions and context blocks.** FoxxyCode advertises `promptCapabilities.embeddedContext`, so a client such as Zed sends what its mention menu picked as content blocks next to the text:

- a `resource` with `text` is attached as sent (Zed includes unsaved edits). A `file://` one is named by its path - relative when it lies in the session's `cwd` - and a line fragment (`#L10-20`, `#L10:20`, `#L10-L20`, `#L10`) labels the attachment with those lines; a query such as Zed's `?symbol=` is dropped;
- a `resource` without `text` is read from disk, anywhere the client names it: a file, a line range of it, or a folder's listing. A missing file or lines past the end fail the prompt, since the client asked for them explicitly;
- a `resource_link` (the block every ACP agent must accept) to a local file or folder is read like a mention of it; a link FoxxyCode cannot open itself - an editor-internal URI, a web address - reaches the model as a line naming it.

`@` mentions typed into a `text` block are resolved the way every surface resolves them - files anywhere on disk, folders, line ranges, `@session:<id>`, `@rule:<name>`, `@agent:<name>`, web pages - into attachments of the same user message ([Mentions](../features/mentions.md)). A mention never fails the prompt: one that names nothing stays prose.

### Subagent runs and child sessions (FoxxyCode-specific)

Nothing protocol-level changes when the agent delegates to a subagent (`docs/features/subagents.md`). The parent's `tool_call` / `tool_call_update` rows carry the `spawn_agent` call and its result; the child runs in its own session and **its updates never reach the ACP client**: the child's progress goes to the background task's output log, so an editor is never sent `session/update` for a session id it did not create. The one message a client can receive on a child's behalf is a `session/request_permission` while the spawning turn is still in flight; it arrives with the **parent's** `sessionId` and a `toolCall.title` prefixed `[subagent <name>]`, and is answered like any other. After that turn has returned, a child's requests are denied without reaching the client.

Child sessions are read-only transcripts. `session/list` omits them, `session/load` replays one like any other bundle, and `session/prompt` against a child's id returns an error naming the parent (`subagent sessions are read-only transcripts: sess_… belongs to sess_…`); the run-plan `_meta` hook is covered by the same guard. Nothing about the id says which is which - the bundle's `subagentRun` metadata does.

### `session/cancel`

Cancel an ongoing prompt turn (notification).

```json
{
  "jsonrpc": "2.0",
  "method": "session/cancel",
  "params": {
    "sessionId": "sess_abc123def456"
  }
}
```

For a writable session bundle, FoxxyCode also writes a small on-disk cancel signal so another **`foxxycode`** process (for example **`foxxycode http`** while **`foxxycode acp`** runs the turn) can observe cooperative cancellation between poll ticks during the turn. The in-process turn still ends via the same **`TurnCtx`** cancel hook when the session is loaded in this process.

### `session/set_mode`

Switch between agent modes (legacy API). Prefer `session/set_config_option` with `configId` `mode` when the client supports Session Config Options.

**Request params:**
```json
{
  "sessionId": "sess_abc123def456",
  "modeId": "plan"
}
```

**Response result:** `null`

When the mode changes, the agent also sends a `session/update` with `config_option_update` so clients using config options stay in sync (for example, the displayed default model may change if no session model override is set).

### `session/set_config_option`

Change a session configuration option (ACP Session Config Options). Supported options: **`mode`**, **`model`**, **`permission_mode`**.

**Request params:**
```json
{
  "sessionId": "sess_abc123def456",
  "configId": "permission_mode",
  "value": "accept_edits"
}
```

Valid `permission_mode` values: `ask` | `accept_edits` | `bypass`. The override is session-scoped and persisted in `session.json`; it takes precedence over the `tools.permission_mode` config file value.

**Response result:** full `configOptions` array with updated `currentValue` fields:

```json
{
  "configOptions": [ ... ]
}
```

Unknown `configId` or a `value` not listed under that option yields a JSON-RPC error (invalid params).

## Notifications (Agent -> Client)

All sent via `session/update` method with a `sessionUpdate` discriminator field.

### `plan` - Agent execution plan

```json
{
  "sessionUpdate": "plan",
  "entries": [
    { "content": "Read auth module", "priority": "high", "status": "pending" },
    { "content": "Design JWT structure", "priority": "high", "status": "pending" },
    { "content": "Implement changes", "priority": "medium", "status": "pending" }
  ]
}
```

### `available_commands_update` - Slash commands from skills

After **`session/new`** and **`session/load`**, FoxxyCode derives slash commands from the same **`ListSkills`** pipeline as **`GET /foxxycode/slash-commands`**. The built-in commands lead the list (**`compact`** while compaction is enabled, **`export`**, **`plugin`**; the same rows as **`GET /foxxycode/commands`**), followed by the skills. The response that registers the session is written before this notification, so clients do not discard the catalog as an update for an unknown session. Rows use ACP **`name`** and **`description`** only (matches [slash commands](https://agentclientprotocol.com/protocol/slash-commands); optional **`input.hint`** is omitted in this MVP). The agent may repeat this notification whenever the catalog changes.

```json
{
  "sessionUpdate": "available_commands_update",
  "availableCommands": [
    { "name": "demo", "description": "Runs the demo checklist" }
  ]
}
```

### `agent_message_chunk` - Text response chunk

```json
{
  "sessionUpdate": "agent_message_chunk",
  "content": {
    "type": "text",
    "text": "I'll start by reading the current auth module..."
  }
}
```

### `tool_call` - Tool call started

```json
{
  "sessionUpdate": "tool_call",
  "toolCallId": "call_001",
  "title": "Reading auth.go",
  "kind": "read",
  "status": "pending"
}
```

### `tool_call_update` - Tool call status update

```json
{
  "sessionUpdate": "tool_call_update",
  "toolCallId": "call_001",
  "status": "completed",
  "content": [
    {
      "type": "content",
      "content": { "type": "text", "text": "File contents: ..." }
    }
  ]
}
```

Tool call statuses: `pending` | `in_progress` | `completed` | `failed` | `cancelled`

### `memory_run` - Memory subagent run

When `memory.enable` is true, every user turn starts a **memory subagent**: a child agent run in the background task pool with its own session bundle and task log ([Long-term memory](../features/memory.md)). This update reports that run and nothing of its text: `started` once the task is launched, `finished` once it settled while the turn was still running (a run that outlives the turn sends nothing more), `skipped` when no run could start. It is neither persisted nor replayed by `session/load`; the run's record is the task (kind `agent`, `agent.system` true) and the child transcript it names.

| Field | Meaning |
|---|---|
| `status` | `started`, `finished` or `skipped` |
| `taskId`, `childSessionId` | the pool task and the child session of the run; empty on a skip |
| `taskStatus` | the pool's verdict on `finished`: `succeeded`, `failed`, `timed_out`, `stopped` |
| `durationMs` | how long the run took, on `finished` |
| `delivered` | whether a non-empty report reached the main model in this turn, in the turn context block of the first request or of a later step |
| `reason` | why the run was skipped, or the error a failed run ended with |

```json
{"sessionUpdate": "memory_run", "status": "started", "taskId": "bg_3", "childSessionId": "sess_9f1c2a7d4e5b6c8d9e0f1a2b"}
{"sessionUpdate": "memory_run", "status": "finished", "taskId": "bg_3", "childSessionId": "sess_9f1c2a7d4e5b6c8d9e0f1a2b", "taskStatus": "succeeded", "durationMs": 3210, "delivered": true}
{"sessionUpdate": "memory_run", "status": "finished", "taskId": "bg_4", "childSessionId": "sess_0a1b2c3d4e5f60718293a4b5", "taskStatus": "failed", "durationMs": 1200, "reason": "402 Payment Required: subscription expired"}
{"sessionUpdate": "memory_run", "status": "skipped", "reason": "memory runs in flight for this session: 2 of 2"}
```

A client shows a `Working with memory` phrase between `started` and `finished`, and drops it when the turn ends: it must not wait for `finished`, because a run that outlives the turn never sends it. The console prints one line when the run settles inside the turn. The updates `memory_phase` and `memory_message_chunk` of earlier releases no longer exist.

### `debug` - Diagnostics trace event

Emitted only when the diagnostics layer is on (**`debug.enable`**, the **`--debug`** flag, or a runtime toggle through **`PUT /foxxycode/config`**). One record per boundary in the ReAct loop, so a client can render a live debug timeline. The HTTP bridge forwards these as SSE **`event: debug`**; the same records are persisted to **`<session>/debug_trace.jsonl`** and served by **`GET /foxxycode/sessions/{id}/debug`**.

**`phase`** is one of **`turn_start`**, **`llm_request`**, **`llm_response`**, **`tool_start`**, **`tool_finish`**. **`title`** names the subject of the phase where there is one (the session mode on **`turn_start`**, the tool name on **`tool_start`** / **`tool_finish`**). **`_meta`** carries lightweight per-phase metadata (mode, model, message and tool counts, token usage, stop reason, tool call id and status) — never the raw LLM bodies, which go to the process log instead. Tracing is best-effort and never affects the turn.

```json
{
  "sessionUpdate": "debug",
  "phase": "llm_response",
  "_meta": {
    "stop_reason": "tool_use",
    "input_tokens": 13003,
    "output_tokens": 112,
    "tool_calls": 1
  }
}
```

Full guide: **[docs/operate/debugging.md](../operate/debugging.md)**.

### `background_wake` - A turn nobody typed

A task the model started with `notify_on_finish` wakes the agent when it ends ([Background tasks](../features/background-tasks.md#waking-the-agent-when-a-task-finishes)). Under `foxxycode acp` the woken turn runs with the client as its sender: its `session/update` notifications arrive outside any `session/prompt` the client sent, and a gated tool inside it is asked with `session/request_permission` like any other. The turn opens with this update, sent before its first message is persisted and in its place: the message is the instruction the model reads, and it is not sent as a `user_message_chunk`, live or on `session/load`.

| Field | Meaning |
|---|---|
| `tasks` | every task the turn reports, in the order they finished |
| `tasks[].id` | the task id (`bg_3`) |
| `tasks[].kind` | `command` or `agent` |
| `tasks[].label` | the command, or the description of a subagent run |
| `tasks[].agent` | the subagent definition behind an agent run |
| `tasks[].status` | `succeeded`, `failed`, `timed_out` or `stopped` |
| `tasks[].exitCode` | the exit code of a command; an agent run's is the pool's and says nothing |
| `tasks[].durationMs` | how long the task ran |
| `tasks[].error` | what went wrong, when the pool recorded something |

```json
{"sessionUpdate": "background_wake", "tasks": [{"id": "bg_3", "kind": "command", "label": "make test", "status": "failed", "exitCode": 2, "durationMs": 90000, "error": "exit status 2"}]}
```

An editor that renders only the standard updates would show an answer nobody asked for, so `foxxycode acp` follows the update with the same wake as a quoted `agent_message_chunk` at the head of the answer, live and on `session/load`:

```text
> Woken by a finished background task: bg_3 make test, failed, exit 2, 1m 30s
```

`foxxycode acp --remote` replays a woken turn of the server the same way on `session/load`; it does not follow the server's woken turns live.

### `current_mode_update` - Mode changed

```json
{
  "sessionUpdate": "current_mode_update",
  "currentModeId": "agent"
}
```

### `config_option_update` - Session config options changed

Sent after `session/set_config_option`, after `session/set_mode`, or whenever the agent updates session config options to match runtime state (so UI stays aligned).

```json
{
  "sessionUpdate": "config_option_update",
  "configOptions": [
    {
      "id": "mode",
      "name": "Session mode",
      "category": "mode",
      "type": "select",
      "currentValue": "agent",
      "options": [ ... ]
    },
    {
      "id": "model",
      "name": "Model",
      "category": "model",
      "type": "select",
      "currentValue": "openai/gpt-4o",
      "options": [ ... ]
    },
    {
      "id": "permission_mode",
      "name": "Permission mode",
      "category": "permissions",
      "type": "select",
      "currentValue": "accept_edits",
      "options": [ ... ]
    }
  ]
}
```

## Permission Requests (Agent -> Client, expects response)

These requests are sent only when `permission_mode` is `ask` (commands and writes) or `accept_edits` (commands only). When `permission_mode` is `bypass`, the agent never sends `session/request_permission`. Set the mode via `session/set_config_option` or `tools.permission_mode` in `config.yaml`.


```json
{
  "jsonrpc": "2.0",
  "id": 10,
  "method": "session/request_permission",
  "params": {
    "sessionId": "sess_abc123def456",
    "toolCall": {
      "toolCallId": "call_002",
      "title": "Run: go build ./...",
      "kind": "run_command",
      "status": "pending",
      "content": [
        { "type": "text", "text": "Execute: go build ./..." }
      ]
    },
    "options": [
      { "optionId": "allow", "name": "Allow", "kind": "allow_once" },
      { "optionId": "allow_always", "name": "Allow always", "kind": "allow_always" },
      { "optionId": "reject", "name": "Reject", "kind": "reject_once" }
    ]
  }
}
```

**Response:** the protocol nests the outcome in its own object, which is what editors such as Zed send:

```json
{
  "jsonrpc": "2.0",
  "id": 10,
  "result": {
    "outcome": { "outcome": "selected", "optionId": "allow" }
  }
}
```

A dismissed request answers `{ "outcome": { "outcome": "cancelled" } }`. FoxxyCode also accepts the flat form its own surfaces and some editor extensions send (`{"outcome": "selected", "optionId": "allow"}`), and reads both the same way: the call proceeds unless the outcome is `cancelled` or the chosen `optionId` is `reject`. Picking `allow_always` (or the program-wide `allow_always_<program>` option) also stores a session grant, so the same command does not ask again.

## Question Requests (Agent -> Client, expects response)

Used by the **`question`** tool. Same inbound JSON-RPC pattern as permission requests (client must reply with the same **`id`**).

```json
{
  "jsonrpc": "2.0",
  "id": 11,
  "method": "session/request_question",
  "params": {
    "sessionId": "sess_abc123def456",
    "requestId": "q_1730000000000",
    "toolCallId": "call_003",
    "questions": [
      {
        "question": "Pick a stack",
        "options": [{ "label": "Go" }, { "label": "Rust" }]
      }
    ]
  }
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": 11,
  "result": {
    "answers": [["Go"]]
  }
}
```

## Client Filesystem Methods

The agent can call these methods on the client (if client supports them):

### `fs/read_text_file`

```json
{
  "jsonrpc": "2.0",
  "id": 5,
  "method": "fs/read_text_file",
  "params": { "path": "/absolute/path/to/file.go" }
}
```

### `fs/write_text_file`

```json
{
  "jsonrpc": "2.0",
  "id": 6,
  "method": "fs/write_text_file",
  "params": {
    "path": "/absolute/path/to/file.go",
    "content": "package main\n..."
  }
}
```
