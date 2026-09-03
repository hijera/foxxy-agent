# `config.yaml` Field Reference

Field-by-field reference for `~/.foxxycode/config.yaml`. For narrative documentation (file discovery, `.env`, provider guides) see [config.md](config.md).

A machine-readable [JSON Schema](config.schema.json) accompanies this reference. Point your editor's YAML language server at it to get autocomplete and typo checking:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/hijera/foxxy-agent/main/docs/config.schema.json
```

VS Code (with the YAML extension), IntelliJ, and Zed pick this comment up automatically. The schema is kept in sync with the Go config structs by `TestDocsConfigSchemaMatchesStructs` in `internal/config/docs_schema_test.go`.

Every field is optional unless marked **required**; an empty `config.yaml` (or none at all) is valid and uses built-in defaults. Any string value may reference environment variables with `${VAR_NAME}` (expanded when the file is loaded). To keep a **literal `$`** in a value (e.g. a secret like `$2y$10$…`), double it as `$$` — the UI does this automatically for the `proxy` fields. `${FOXXYCODE_HOME}` and `${CWD}` are expanded by the loader (see [config.md](config.md#environment-variable-references)).

## Agent self-configuration

Agent sessions expose a typed configuration tool family with staged, uci-like semantics:

- `config_get` reads a dotted path from the active YAML file. Secret-shaped fields (including `api_key_command`), MCP environment values, and HTTP header values are returned as `<redacted>`.
- `config_set` **stages** UCI-style commands (`set`, `add_list`, `del_list`, `delete`) without touching the file. Unknown schema paths and commands that would make the config invalid are rejected at staging time. Echoed command lists mask secret-shaped values as `<redacted>`; the staged store keeps the original values.
- `config_changes` lists the staged commands that a commit would apply (secrets redacted).
- `config_commit` applies the staged batch: validates, snapshots the previous file to `config.yaml.prev` (an empty document when the config file did not exist yet, so the first commit stays reversible), writes atomically, and hot-reloads skills, rules, built-in tools, and configured MCP servers. Because a commit can start MCP processes and change the permission policy itself, it prompts for tool permission in both `ask` and `accept_edits` modes - only `tools.permission_mode: bypass` skips the dialog - and the prompt lists the staged commands with secrets redacted. The agent is additionally instructed to ask the user to confirm saving first. If runtime reload fails, the file is restored and the staged commands are kept; if even that restore fails, the staged list stays consumed so a blind retry cannot replay it.
- `config_revert` discards staged commands (all of them, or those under one path).
- `config_rollback` restores the pre-commit snapshot over the active file (swapping the two, so a second rollback undoes the first) and hot-reloads. It carries the same permission policy as `config_commit`, and the agent warns the user before calling it.

Commands and paths are dotted like OpenWrt's `uci` CLI, with a selector for named sequence entries:

| Command | Meaning |
|---|---|
| `set agent.max_turns=40` | Set a mapping field |
| `set mcp_servers[name=context7]={"command":"npx"}` | Select a sequence object by scalar field; append it when setting if absent |
| `add_list skills.dirs=/opt/skills` | Append a sequence entry |
| `del_list skills.dirs=/opt/skills` | Remove a matching sequence entry |
| `delete mcp_servers[name=context7]` | Delete a field or entry |
| `skills.dirs.0` (path form) | Sequence index |

The root path (`.` or `/`) is read-only. Values are JSON for objects and arrays; string-typed fields take the literal text. Staged commands persist in the session bundle, so they survive restarts and HTTP permission resumes.

The bundled `/configure-foxxycode` skill teaches the agent this syntax, the confirm-then-commit workflow, and the safe discovery/install workflow for MCP servers and skills; it also carries the agent-facing catalog of configuration areas and must be updated together with this reference on any schema change. Process-level listener changes may still require restarting the relevant command; the hot reload is specifically guaranteed for the current session's agent configuration, skills, rules, built-in tools, and global MCP clients.

## Top-level keys

| Key | Type | Purpose | Build tag |
|---|---|---|---|
| [`providers`](#providers) | list | LLM API credentials and endpoints | — |
| [`models`](#models) | list | Logical model entries selectable per session | — |
| [`agent`](#agent) | object | ReAct loop model and safety caps | — |
| [`autocomplete`](#autocomplete) | object | Inline code completion for the editor plugins | — |
| [`prompts`](#prompts) | object | System prompt template overrides | — |
| [`instructions`](#instructions) | object | Project instruction files (AGENTS.md) | — |
| [`skills`](#skills) | object | Skill discovery directories | — |
| [`rules`](#rules) | object | Project rules discovery | — |
| [`mcp_servers`](#mcp_servers) | list | MCP servers connected per session | — |
| [`mcp`](#mcp) | object | Trust policy for project-local MCP discovery | — |
| [`tools`](#tools) | object | Permission policy for built-in tools | — |
| [`logger`](#logger) | object | Log level, outputs, rotation | — |
| [`sessions`](#sessions) | object | Session bundle storage | — |
| [`memory`](#memory) | object | Long-term memory copilot | `memory` |
| [`httpserver`](#httpserver) | object | OpenAI-compatible HTTP API defaults | `http` |
| [`scheduler`](#scheduler) | object | Cron scheduler | `scheduler` |
| [`gateways`](#gateways) | object | Messenger bot adapters | `gateway` / `gateway.telegram` |
| [`ui`](#ui) | object | Embedded SPA preferences | `http`, `ui` |

"Build tag" means the block only takes effect in binaries built with that `-tags` value; it is parsed and ignored otherwise.

## `providers`

List of LLM backends (`[]config.ProviderConfig`, `internal/config/providers.go`).

| Field | Type | Required | Default | Env fallback | Description |
|---|---|---|---|---|---|
| `name` | string | **yes** | — | — | Logical id used as the first segment of `models[].model`. Must match `^[a-zA-Z][a-zA-Z0-9_-]*$`. |
| `type` | string | **yes** | — | — | Wire protocol: `openai`, `anthropic`, `neuraldeep`, or `codex`. Use `openai` for configurable OpenAI-compatible endpoints; `neuraldeep` uses its fixed endpoint; `codex` uses ChatGPT OAuth against the official Codex backend (Responses API). |
| `api_base` | string | no | provider SDK default | — | Base URL override. For `type: openai` include `/v1` (e.g. `http://localhost:11434/v1`); for `type: anthropic` an Anthropic-compatible gateway. Ignored for `type: neuraldeep` and `type: codex`, which use fixed official endpoints. |
| `api_key` | string | no | `""` | `NAME_API_KEY` | Literal secret or `"${ENV}"` reference. Empty reads `NAME_API_KEY` at LLM call time (NAME = provider name uppercased, hyphens → underscores; e.g. `deepseek` → `DEEPSEEK_API_KEY`). For `type: neuraldeep`, when the key is empty from all three sources the key stored by `foxxycode providers login <name>` (`$FOXXYCODE_HOME/providers/<name>/neuraldeep-auth.json`) is used - an explicit key always wins over the stored login. |
| `api_key_command` | string | no | `""` | — | Credential-helper command run via the detected host shell when `api_key` is empty (`pwsh` → `powershell` → `cmd` on Windows; `bash` → `sh` elsewhere); trimmed stdout becomes the key. Falls back to `NAME_API_KEY` on failure. |
| `proxy` | string | no | environment proxy | — | Per-provider outbound proxy: `http://`, `https://`, `socks5://`, or `socks5h://` URL. Overrides a proxy inherited from the environment (`HTTP_PROXY`/`HTTPS_PROXY` — the IDE plugin forwards the editor's proxy this way); `NO_PROXY` is still honored and local addresses always connect directly. When empty, the environment proxy is used, or a direct connection when there is none. Treated as a literal URL (no `${VAR}` references); a `$` in the userinfo is auto-escaped to `$$` when saved via the UI. |

Key resolution order: `api_key` → `api_key_command` stdout → `NAME_API_KEY` env var.

```yaml
providers:
  - name: openai
    type: openai
    api_key: "${OPENAI_API_KEY}"
  - name: local
    type: openai
    api_base: "http://localhost:11434/v1"
    api_key: "~"
  - name: codex
    type: codex # use Sign In with ChatGPT in the bundled web UI; no api_key needed
```

### llama.cpp as an OpenAI-compatible provider

`llama-server` works as a `type: openai` provider (`api_base: "http://host:8080/v1"`). Recommended launch flags:

- `--jinja` — enables the model's chat template on the server, which is required for **tool calling**. Without it llama.cpp silently ignores the `tools` parameter and the agent loop degrades to plain text answers.
- `-c <n>` — set the context window large enough for an agent prompt (system prompt plus tool schemas plus history; 16k is a practical minimum, more is better). When a request exceeds the server context, llama.cpp reports `the request exceeds the available context size` — raise `-c` or trim `max_context_tokens`.

llama.cpp builds through 2025 report mid-stream failures with a non-standard SSE `error:` field; FoxxyCode understands both that dialect and the current `data: {"error": ...}` shape and surfaces the server's message in the error.

For `type: codex`, open **Settings → LLM Providers** in the bundled web UI and select **Sign In with ChatGPT**, or use the terminal flow:

```bash
foxxycode codex login    # prints a URL and one-time code, then waits
foxxycode codex status   # reports credential availability and source
foxxycode codex logout   # removes only the FoxxyCode-managed credential
```

`--provider NAME` targets a particular Codex provider. Refreshable credentials are stored at `$FOXXYCODE_HOME/providers/<provider-name>/codex-auth.json` and never enter `config.yaml`; a Codex CLI login at `~/.codex/auth.json` (or `$CODEX_HOME/auth.json`) is used as a fallback. `api_key`, `api_key_command`, and `api_base` are ignored, while `proxy` applies to OAuth and provider requests. `FOXXYCODE_CODEX_BASE_URL` is the process-level backend override intended for tests and self-hosted gateways.

Codex is only a model backend: FoxxyCode keeps its own system prompt, tools, permissions, and ReAct loop. Access tokens are refreshed shortly before expiry and written back to their source file. When a Codex provider is configured, `foxxycode acp` and `foxxycode http` log a non-secret credential status line at startup.

Codex reasoning supports `none`, `low`, `medium`, `high`, and `xhigh`; `minimal` is mapped to `none`. FoxxyCode requests reasoning summaries and encrypted reasoning content so the model can replay its own opaque reasoning item across tool calls. The opaque value is persisted as `reasoning_signature` but is not exposed by the session messages HTTP endpoint.

## `models`

List of logical models (`[]config.ModelEntry`, `internal/config/models.go`).

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `model` | string | **yes** | — | `"provider_name/api_model_id"`. First segment must match a `providers[].name`; the remainder is sent to the API verbatim (may contain slashes). |
| `max_tokens` | int | no | `0` | Completion-token cap per assistant message. Ignored by `codex`, whose backend rejects `max_output_tokens`. |
| `temperature` | float | no | `0` | Sampling temperature. |
| `max_context_tokens` | int | no | `0` | UI hint for the context bar; `0` derives from provider metadata. |
| `multimodal` | bool | no | `false` | Model accepts image/file inputs; UI shows an attachment button. |
| `stream` | bool | no | `true` | Transport. Omitted or `true` streams the answer over SSE. `false` sends one blocking completion request and delivers the whole answer at once. Rejected for `type: codex` providers, whose backend is streaming-only. |
| `reasoning_levels` | string list | no | auto-detected | Override the offered reasoning levels. Omitted: auto-detect from the model id (`gpt-5*` → `minimal,low,medium,high`; o-series, `gpt-oss*`, `qwen3*` (qwen3, qwen3.5, qwen3.6, ...) and Claude thinking models → `low,medium,high`). Explicit `[]` hides the selector. For `qwen3*` on OpenAI-compatible providers a selected level also sends `chat_template_kwargs` `{"enable_thinking": true}`. |
| `reasoning_default` | string | no | — | Level pre-selected for new chats; must be one of the resolved levels. |

```yaml
models:
  - model: "openai/gpt-4o"
    max_tokens: 8192
    temperature: 0.2
    multimodal: true
  - model: "openai/gpt-5"
    max_tokens: 8192
    reasoning_default: medium
  - model: "local/qwen3-30b"
    max_tokens: 8192
    stream: false
```

**Non-streaming models.** `stream: false` changes one thing: FoxxyCode sends a single blocking `POST /chat/completions` instead of asking for SSE, and hands the finished answer to the rest of the runtime in one piece. Everything downstream is unchanged, so the transcript, tool calls, and session bundle look the same; what differs is that nothing appears until the model is done, and the thinking row shows no live duration. Two consequences are worth knowing before turning it on. Pressing **Stop** during a blocking call loses the whole answer, because the server has sent nothing yet, whereas a streamed turn keeps the tokens that already arrived. And a client asking for an SSE stream still gets one - the switch governs the connection to the LLM, not the connection to the client - which is why a streaming HTTP response now carries a keepalive comment every 15 s so proxies do not drop a turn that stays silent for minutes.

The switch is rejected for `type: codex` providers. The Codex Responses backend has no blocking mode, so honoring the key there would mean streaming anyway and only pretending not to.

## `agent`

ReAct loop settings (`config.Agent`, `internal/config/agent.go`).

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `model` | string | required when `models` is non-empty | — | Default `models[].model` id until the client overrides per session. |
| `max_turns` | int | no | `30` | Max LLM calls per prompt turn. |
| `max_tokens_per_turn` | int | no | `200000` | Max tokens across all calls in one turn. |
| `llm_retry_max` | int | no | `3` | Retries after retryable LLM errors (e.g. HTTP 429). An explicit `0` disables retries. |
| `llm_retry_base_ms` | int | no | `1000` | Initial backoff between retries, ms. A server-provided pause (`Retry-After-Ms` / `Retry-After` headers, `Limit resets at` / `retry in Ns` body phrases) overrides the exponential backoff, capped at 60s. |
| `llm_min_interval_ms` | int | no | `0` | Minimum gap between consecutive LLM calls, ms, retry attempts included (e.g. `12000` on strict free tiers). |
| `llm_first_token_timeout_ms` | int | no | `90000` | How long a streamed LLM call may stay silent before the turn cancels it (the API hang guard). An explicit `0` disables the guard; blocking (`stream: false`) transports are never guarded. |
| `loop_guard` | bool | no | `true` | Runaway-loop protection: cut a response that degenerates into repeating itself, block a tool called over and over with identical arguments. |
| `loop_tool_repeat_limit` | int | no | `3` | Consecutive identical tool calls before the guard steps in; `0` disables the check. |
| `loop_stream_repeat_cycles` | int | no | `5` | Identical back-to-back output cycles in one streamed response before it is cut; `0` disables the check. |
| `loop_nudge_max` | int | no | `2` | Nudges the guard sends before it stops the turn with a notice. |

## `prompts`

System prompt template overrides (`config.Prompts`, `internal/config/prompts.go`). Template fields are documented in [config.md](config.md#full-configuration-schema).

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `dir` | string | no | `""` (embedded templates) | Directory with Go text/template files. Supports `~` and `${CWD}` (session cwd at render time). Ask uses `ask.md` and Debug uses `debug.md` in this directory. |
| `agent_prompt` | string | no | `agent.md` | Template file name for agent mode, inside `dir`. |
| `plan_prompt` | string | no | `plan.md` | Template file name for plan mode, inside `dir`. |
| `docs_prompt` | string | no | `docs.md` | Template file name for docs mode, inside `dir`. |
| `per_provider.enabled` | bool | no | `true` | Select a system prompt tuned to the active model for the current mode. Custom files resolve most-specific first: configured model-reference slug (e.g. `openai/gpt-4o` -> `ask.openai-gpt-4o.md`), provider-neutral API-model slug (e.g. `local/gpt-oss-20b` -> `ask.gpt-oss-20b.md`), per-family `<mode>.<family>.md`, then shared `<mode>.md`. Families: `anthropic`, `openai`, `gemini`, `gpt-oss`, `qwen`, `gemma`, `neuraldeep`. Built-in prompts use the same key order at fragment level; gpt-oss-20b and gpt-oss-120b have distinct profiles in Agent, Plan, Ask, and Docs modes. |

## `instructions`

Project instruction files (`config.Instructions`, `internal/config/instructions.go`).

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `files` | string list | no | `["AGENTS.md"]` | Filenames relative to the session CWD, read and appended to the system prompt. |

## `skills`

Skill discovery (`config.Skills`, `internal/config/skills.go`).

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `dirs` | string list | no | `["~/.agents/skills", "${FOXXYCODE_HOME}/skills", "${CWD}/.foxxycode/skills"]` | Directories scanned for skills. Later entries have **higher** priority on name conflicts. `${FOXXYCODE_HOME}` and `${CWD}` expand at runtime (per-session cwd for `${CWD}`). |
| `sources` | string list | no | `[]` | Remote skill sources to install from: `owner/repo[@ref]`, a git URL, or an `http(s)` URL to an agents-standard `marketplace.json`. Fetched on demand via `foxxycode skills sync` / the `/plugin` command / Settings → Skills (never automatically) into the managed skills dir. |
| `auto_discovery` | bool | no | `true` | Offer the model-driven `load_skill` tool so the agent can pull a catalogued skill's instructions into a turn on its own (instead of requiring an explicit `/name`). |

## `rules`

Project rules discovery (`config.Rules`, `internal/config/rules.go`). See [rules.md](rules.md).

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `auto_discover` | bool | no | `true` | Scan `.foxxycode/rules`, `.cursor/rules`, `.claude/rules`, `.codex/rules`, and nested `**/AGENTS.md` under the session CWD. |
| `systems` | string list | no | `[]` (all) | Restrict which rule systems are loaded: `foxxycode`, `cursor`, `claude`, `codex`, `agents`. |

## `mcp_servers`

MCP servers connected for every new session (`[]config.MCPServerConfig`, `internal/config/mcp_servers.go`).

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `type` | string | no | `stdio` | Transport: `stdio` (local command), `http` (streamable HTTP to `url`, with automatic legacy-SSE fallback), or `sse` (legacy HTTP+SSE). Url-only entries default to `http`. |
| `name` | string | **yes** | — | Stable unique id. |
| `command` | string | stdio only | — | Executable for stdio transport. |
| `args` | string list | no | `[]` | Argv after `command`. `${CWD}` expands to the session cwd. |
| `env` | list of `{name, value}` | no | `[]` | Extra environment variables for the stdio child process. |
| `url` | string | http/sse only | — | HTTP(S) endpoint for `type: http` or `type: sse`. `${CWD}` expands to the session cwd. |
| `headers` | list of `{name, value}` | no | `[]` | Headers sent with MCP HTTP requests (e.g. `Authorization`). |
| `insecure_skip_verify` | bool | no | `false` | Accept this http/sse server's TLS certificate without verifying it, so a self-signed or expired certificate connects. Removes the protection against a man in the middle; use only on trusted networks. Setting it changes the declaration digest, so a project-local entry needs approving again. |
| `disabled` | bool | no | `false` | Skip connecting this server without removing its definition. |
| `disabled_tools` | string list | no | `[]` | Tool names of this server hidden from the agent. |

```yaml
mcp_servers:
  - name: filesystem
    command: npx
    args: ["-y", "@modelcontextprotocol/server-filesystem", "/home/user"]
    disabled_tools: ["write_file"]
```

Servers can also be declared in Cursor-compatible mcp.json files: the user-global
`~/.foxxycode/mcp.json` (like Cursor's `~/.cursor/mcp.json`; together with this
`mcp_servers` list it forms the "global" scope) and the project-local
`<workspace>/.foxxycode/mcp.json` ("local" scope). Each file holds a single
`mcpServers` object keyed by server name (`env` and `headers` are JSON objects;
per-tool switches use `disabledTools`). Later levels override earlier ones by
name: `mcp_servers` < `~/.foxxycode/mcp.json` < `./.foxxycode/mcp.json`. See
`docs/mcp-integration.md`.

## `mcp`

MCP settings that are not tied to a single server entry (`config.MCP`, `internal/config/mcp.go`).

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `project_trust` | string | no | `ask` | Trust policy for the project-local `<workspace>/.foxxycode/mcp.json`, which travels with the checkout: `ask` keeps its servers cold until the operator approves that exact declaration for that workspace (`foxxycode mcp trust <name>`, or the shield in Settings → MCP servers); `allow` starts them automatically; `deny` never loads them. The `-mcp-project-trust` flag on `foxxycode acp` / `foxxycode http` overrides this value for one process; an unknown value fails the launch. |

Approvals live in `<home>/mcp-trust.json`, keyed by canonical workspace and bound to a digest of the
command-bearing declaration, so editing an approved entry asks again. Entries from `config.yaml` and
`~/.foxxycode/mcp.json` are operator-authored and are never gated.

## `tools`

Permission policy (`config.Tools`, `internal/config/tools.go`).

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `permission_mode` | string | no | `ask` | `ask` — prompt for commands and file writes; `accept_edits` — auto-approve writes, prompt for commands; `bypass` — never ask (trusted environments only). Overridable per session via ACP `session/set_config_option`. |
| `command_allowlist` | string list | no | `[]` | Commands that never require permission. Exact or prefix match (prefix + space + args). `"*"` allows everything. |
| `permission_timeout_seconds` | int | no | `0` | How long a permission prompt may wait for the operator before the tool call is cancelled instead. `0` waits forever; a positive value keeps an unresponsive client from holding the session turn lock indefinitely. |
| `ssh_connect_timeout` | int | no | `30` | TCP dial timeout in seconds for the `ssh_run_command` tool. |
| `plan_no_self_run` | bool | no | `false` | Forbid the model from starting to execute a plan itself. In plan mode `plan_exit` is not offered and any tool outside the plan allowlist is refused instead of run, so only **Run plan** starts the implementation. The `-plan-no-self-run` flag on `foxxycode acp` / `foxxycode http` overrides this value; the IntelliJ and VS Code plugins pass it, so their panels are guarded by default. |
| `ask_disable_extended_tools` | bool | no | `false` | Hide Ask mode's read-only shell, web, annotated MCP, and scheduler inspection tools. Basic repository read/search/tree, question, and skill tools remain available. |
| `output_limits` | object | no | — | Per-tool line and byte ceilings for results and errors. |
| `background` | object | no | — | Bounds for commands the agent runs detached in the session background task pool. See below. |

### `tools.output_limits`

Every positive line limit also applies a hard **64 KiB per-call byte ceiling**. `0` disables both limits for that tool; an unset field uses the default. Truncated output ends with a recovery hint.

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `read` | int | no | `1000` | File-page or directory-listing lines. |
| `grep` | int | no | `200` | `path:line:content` records. |
| `glob` | int | no | `300` | Returned paths. |
| `print_tree` | int | no | `400` | Directory-tree lines. |
| `run_command` | int | no | `500` | Combined stdout and stderr lines. |
| `ssh_run_command` | int | no | `500` | Remote command output lines. |
| `webfetch` | int | no | `800` | Fetched page lines. |
| `websearch` | int | no | `200` | Search-result lines. |
| `default` | int | no | `1000` | Unlisted and MCP tools; `0` is unlimited. |

### `tools.background`

Bounds for background execution (`config.ToolBackground`). A backgrounded `run_command` returns a task id instead of output; `background_list`, `background_output`, `background_wait`, and `background_stop` collect the result later. The pool lives inside the running `foxxycode` process: each task mirrors its metadata and captured output into the session bundle under `background/<task_id>/`, and a task interrupted by a restart is reported as `orphaned` rather than as still running. `0` on any integer field means "use the default". See `docs/background-tasks.md`.

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `enabled` | bool | no | `true` | Offer the `background` option on `run_command` and expose the background task tools. |
| `max_concurrent` | int | no | `5` | Background tasks one session may run at once. Starting past the limit is refused, not queued. |
| `default_timeout_seconds` | int | no | `900` | Hard limit for a task started without an explicit `timeout_seconds` and without `expected_seconds`. |
| `max_timeout_seconds` | int | no | `3600` | Ceiling applied to any requested or estimate-derived timeout. |
| `output_buffer_bytes` | int | no | `262144` | In-memory output window per task, used by the status ticker and `background_output`. The full log still goes to the session bundle. |

## `logger`

Logging (`config.Logger`, `internal/config/logger.go`). ACP flags `--log-level`, `--log-output`, `--log-file`, `--log-format` override these when set.

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `level` | string | no | `info` | `debug`, `info`, `warn`, `error` (`warning` accepted as alias of `warn`). |
| `outputs` | string list | no | `["stderr"]` | Any combination of `stdout`, `stderr`, `file`. |
| `file` | string | required when `outputs` includes `file` | `""` | Path for the file sink. Supports `${FOXXYCODE_HOME}`. |
| `format` | string | no | `text` | `text` or `json`. |
| `rotation.max_size_mb` | int | no | `0` | Rotate after this size in MB; `0` disables size-based rotation. |
| `rotation.max_files` | int | no | `0` | Rotated backups to keep when `max_size_mb > 0`. |

## `debug`

Diagnostics master switch (`config.Debug`, `internal/config/debug.go`). Off by default and free when off. Full guide: **`docs/debugging.md`**.

Not related to the `debug` **session mode** — that one changes how the model behaves, this one changes what FoxxyCode records.

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `enabled` | bool | no | `false` | Turns on the whole layer: forces the process logger to `debug` level (overriding `logger.level`), enables raw LLM HTTP capture, and writes a per-session `debug_trace.jsonl`. |
| `capture_llm` | bool | no | unset → follows `enabled` | Gates only the raw request/response body logging. Unset means "follow `enabled`". Set explicitly to `false` to keep debug logs and the trace while suppressing bodies (which carry the whole conversation). |

The `--debug` flag on `foxxycode acp`, `foxxycode http`, and `foxxycode gateway` forces `enabled: true` for that process. It only ever turns the layer **on**: the flag is applied only when explicitly passed, so a default-false flag cannot silently override a config-enabled layer. `foxxycode desktop` and the console have no flag but honour `debug.enabled` from `config.yaml`.

`PUT /foxxycode/config` applies a change to `debug.enabled` **without a restart** — the log level is re-set on a shared `slog.LevelVar` and the capture flag is atomic.

## `sessions`

Session bundle storage (`config.Sessions`, `internal/config/sessions.go`).

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `dir` | string | no | `""` → `${FOXXYCODE_HOME}/sessions` | Sessions root. Supports `${FOXXYCODE_HOME}` and `~`. Overridden by the `--sessions-dir` flag. |

## `memory`

Long-term memory copilot (`config.MemoryConfig`, `internal/config/memory.go`; implementation in `external/memory`, `memory` build tag).

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `enabled` | bool | no | `false` | Turn on the memory copilot. |
| `model` | string | no | `""` (agent model) | Exact `models[].model` id used only for recall/persist LLM calls. |
| `dir` | string | no | `""` → `${FOXXYCODE_HOME}/memory` | Long-term memory root. Supports `${FOXXYCODE_HOME}` and `~`. |
| `recall_max_turns` | int | no | `6` | Bounds recall-side LLM rounds. |
| `persist_max_turns` | int | no | `12` | Bounds persist-side LLM rounds. |
| `copilot_max_tokens` | int | no | `4096` | Completion cap for memory LLM calls. |
| `max_search_hits` | int | no | `8` | Max snippets returned by `memory_search`. |

## `compaction`

Automatic context compaction (`config.CompactionConfig`, `internal/config/compaction.go`; always compiled). When the running prompt approaches the model's context window (`models[].max_context_tokens`), older turns are summarized into one message so the session can continue. Two engines share this section, selected by `engine`: the default **coddy** engine inserts a summary row and replays only the window from the last summary onward (and enables the manual `/compact` command plus the HTTP compact endpoint); the **opencode** engine flags older messages compacted and excludes them from the model payload while keeping them in the transcript.

Either engine republishes the context estimate right after it folds history: the agent recomputes the
`conversation` / `summary` categories over the window that engine actually sends, persists the result
next to the provider token counters in `stats.json`, and emits the `usage_update` ACP/SSE event. The
composer context ring therefore drops without a reload, and a session reopened after a restart
reports the compacted window rather than its pre-compaction size.

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `engine` | string | no | `coddy` | Compaction implementation: `coddy` or `opencode`. |
| `enabled` | bool | no | `true` | Turn on auto-compaction. Unset defaults to `true`; set `false` to disable. |
| `model` | string | no | `""` (agent model) | Exact `models[].model` id used for the summarization pass. |
| `threshold_percent` | int | no | `80` (coddy) / `85` (opencode) | Trigger when context usage exceeds this percent of the model context window. The opencode engine clamps to 50..99. |
| `keep_recent_turns` | int | no | `2` | Most recent user turns preserved verbatim (never summarized). |
| `max_tokens` | int | no | `4096` | Completion token cap for the summary generation (opencode engine only). |
| `result_eviction` | object | no | — | Prunes superseded `read`/`grep` results from the LLM projection while keeping persisted messages complete. |

### `compaction.result_eviction`

Unmarked large `read` and `grep` results collapse to short placeholders in later LLM requests. A result survives when the model calls `keep_result`, sets `keep: true`, or it remains in the recent working window. Successful filesystem and Subversion mutations invalidate earlier observations of workspace content.

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `enabled` | bool | no | `true` | Master switch. |
| `keep_recent` | int | no | `2` | Recent evictable results kept intact; `0` keeps none. |
| `min_result_bytes` | int | no | `2000` | Results at or below this size are never evicted; `0` makes all results candidates. |

## `autocomplete`

LLM-backed inline code completion (`config.AutocompleteConfig`, `internal/config/autocomplete.go`; always compiled). This is the greyed suggestion the editor plugins draw ahead of the caret and accept with Tab. Editors fetch it over `POST /foxxycode/completion` — one single-shot LLM call with no tools, no session and no agent loop — and read `GET /foxxycode/completion/config` to learn when to ask. See [http-api.md](http-api.md).

Unlike [`compaction`](#compaction) and [`title`](#title), this section is **off unless enabled explicitly**: a suggestion is requested as you type, so leaving it on by default would spend tokens on every keystroke. Point `model` at a small, fast `models[]` entry — speed beats cleverness here, because a suggestion is worthless once you have typed past it.

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `enabled` | bool | no | `false` | Turn on inline suggestions in the editor plugins. |
| `model` | string | no | `""` (agent model) | Exact `models[].model` id used for the suggestion pass. |
| `mode` | string | no | `auto` | How the hole reaches the model. `auto`: native fill-in-the-middle tokens through a raw completion (`POST /v1/completions`) when the model family is known (Qwen-Coder, DeepSeek-Coder, CodeLlama, StarCoder, Codestral) and the provider is OpenAI-compatible, a chat prompt otherwise; a raw call that fails switches that model to chat for the rest of the process. `chat`: always a chat prompt. `fim`: always FIM tokens, and an error when that is not possible. |
| `temperature` | number | no | `0` | Sampling temperature. Unlike `models[].temperature`, `0` here is the value rather than "unset": suggestions are greedy by default, so the same context yields the same suggestion and it survives the next keystroke. |
| `max_tokens` | int | no | `128` | Completion token cap for one suggestion. Caps the model entry's own `max_tokens`, so sharing an entry with the agent cannot buy an 8k-token suggestion. |
| `timeout_ms` | int | no | `4000` | How long one suggestion request may take before it is abandoned. |
| `debounce_ms` | int | no | `350` | Typing pause before an automatic request goes out. Ignored when `trigger` is `manual`. |
| `trigger` | string | no | `auto` | `auto` suggests while you type; `manual` suggests only on the editor shortcut. |
| `multi_line` | bool | no | `true` | Allow a suggestion to span several lines. When `false`, only its first line is kept. Even when allowed, a block is only produced where the caret invites one: at the end of a line that opened a block, or on an empty line; with code to the right of the caret the suggestion never grows past the line. |
| `related_files` | int | no | `3` | How many other open editor tabs (reported over `POST /foxxycode/ide/editor-state`, workspace files only) are excerpted — first 40 lines, up to 1500 bytes each — into the prompt so the model sees imports and signatures from neighbouring files. `0` disables it. |
| `max_prefix_bytes` | int | no | `8000` | How much of the text before the caret is sent as context. |
| `max_suffix_bytes` | int | no | `2000` | How much of the text after the caret is sent as context. |

Retries are deliberately disabled for this pass regardless of `agent.llm_retry_max`: a retried suggestion lands after the user has typed past it. Qwen3-family thinking is pinned off (`chat_template_kwargs.enable_thinking: false`) regardless of the serving default, because a thinking model can spend the whole small budget inside its reasoning block and return nothing. Generation stops at sequences matched to the request — the line break for a single-line suggestion, the exact next suffix line and a blank-line run for a block — and, in chat mode, the streamed reply is cut the moment it dedents past the caret's scope. `GET /foxxycode/completion/stats` reports latency, token cost and the editor-reported acceptance rate.

**Judging quality on a live hub.** `make e2e-autocomplete` with `NEURALDEEP_API_KEY` set runs `external/httpserver/e2e_neuraldeep_autocomplete_test.go`: a dozen caret positions across Go, Python, TypeScript, JavaScript and Kotlin, each with a loose acceptance rule (the idea the answer must contain, no fence, no re-typed suffix, single-line where the caret demands it), run against every model in `FOXXYCODE_E2E_MODELS` in every prompt mode in `FOXXYCODE_E2E_MODES` (default `auto,chat`, so native FIM and chat prompting can be compared per model). It prints a markdown report with the actual completions and latencies, writes it to `FOXXYCODE_E2E_REPORT` when set, and only fails below `FOXXYCODE_E2E_MIN_SCORE`. It is skipped without the key, so CI never talks to the hub.

## `title`

Automatic session title generation (`config.TitleConfig`, `internal/config/title.go`; always compiled). After the first exchange in a fresh, non-pinned session, a hidden internal "title" agent generates a short thread title. It runs backend-side so every client (SPA, IntelliJ, VS Code, ACP, CLI) gets the title, pushed live over the session-update stream. A user-pinned title always wins and is never overwritten; the auto-title is generated at most once per session.

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `enabled` | bool | no | `true` | Turn on auto-title generation. Unset defaults to `true`; set `false` to disable. |
| `model` | string | no | `""` (agent model) | Exact `models[].model` id used for the title pass. A small, cheap model is a good choice. |
| `max_tokens` | int | no | `64` | Completion token cap for the title generation. |

## `httpserver`

OpenAI-compatible HTTP API defaults (`config.HTTPServerConfig`, `internal/config/http.go`; `http` build tag). See [http-api.md](http-api.md).

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `host` | string | no | `""` → `0.0.0.0` | Default bind address when `foxxycode http` does not pass `-H/--host`. |
| `port` | int | no | `0` → `12345` | Default listen port when `foxxycode http` does not pass `-P/--port`. Range 0–65535. |
| `auth_token` | string | no | `""` | Optional bearer credential for the HTTP API. Empty = no auth. `${ENV}` expanded at load; prefer `--auth-token` / `FOXXYCODE_HTTP_TOKEN`. Redacted from `GET /foxxycode/config`. See [remote-control.md](remote-control.md). |
| `public_docs` | bool | no | `false` | Keep `/docs` and `/openapi.*` reachable without a token when auth is enabled. |
| `allow_insecure` | bool | no | `false` | Silence the startup warning about a non-loopback bind without authentication. |
| `cors.enabled` | bool | no | `false` | Turn on CORS handling (preflight + `Access-Control-*` headers). |
| `cors.allowed_origins` | []string | no | `[]` | Exact origins permitted to call the API. A single `"*"` allows any origin (bearer auth still applies). |
| `remotes[].name` | string | no | — | Display name of a remote server in the UI environment selector. |
| `remotes[].url` | string | no | — | Base URL of the remote `foxxycode http` server. Tokens are not stored here; the UI keeps them client-side. |

## `scheduler`

Cron scheduler (`config.SchedulerConfig`, `internal/config/scheduler.go`; `scheduler` build tag). Job file format is described in [config.md](config.md#scheduler-optional-build).

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `enabled` | bool | no | `false` | Run the scheduler daemon and expose `foxxycode_scheduler_*` tools. `foxxycode acp\|http -scheduler-enabled` forces it per process. |
| `dir` | string | no | `""` → `${FOXXYCODE_HOME}/scheduler` | Directory with flat `*.md` job definitions. |
| `max_queue` | int | no | `10` | Concurrent scheduled runs; extra firings are skipped when saturated. |
| `timeout` | string | no | `"30m"` | Per-run wall-clock limit (Go duration, e.g. `1h30m`). |
| `retain_sessions` | int | no | `5` | Completed run session dirs kept per `job_id` under `sessions.dir`. |

## `gateways`

Messenger gateways (`config.GatewayConfig`, `internal/config/gateway.go`; `gateway` or `gateway.telegram` build tag; run with `foxxycode gateway`). See [gateway.md](gateway.md).

### `gateways.telegram`

| Field | Type | Required | Default | Env fallback | Description |
|---|---|---|---|---|---|
| `enabled` | bool | no | `false` | — | Activate the Telegram adapter. |
| `token` | string | no | `""` | `TELEGRAM_BOT_TOKEN` | Bot token from @BotFather. Empty reads the env var (e.g. via `~/.foxxycode/.env`). |
| `proxy` | string | no | direct | — | Outbound proxy: `http`, `https`, `socks5`, `socks5h`. Treated as a literal URL (no `${VAR}` references); a `$` in the userinfo is auto-escaped to `$$` when saved via the UI. |
| `rich_messages` | bool | no | `false` | — | Bot API 10.1 Rich Messages; falls back to legacy formatting when unsupported. |
| `admins` | int list | no | `[]` | — | Telegram user IDs with elevated rights; always pass access checks. |
| `default_access` | string | no | `all` | — | `all`, `admins`, or `group:<name>`. |
| `default_isolation` | string | no | `individual` | — | `individual`, `shared`, or `admin`. |
| `user_groups` | list | no | `[]` | — | Named sets: `{name, user_ids}`. Referenced as `group:<name>`. |
| `chats` | list | no | `[]` | — | Per-chat overrides: `{chat_id, isolation, access}`. `chat_id` is negative for groups/supergroups. |

## `ui`

Embedded SPA preferences (`config.UIConfig`, `internal/config/ui.go`). Used by the desktop launcher and Settings UI when built with `-tags http,ui`.

| Field | Type | Required | Default | Env fallback | Description |
|---|---|---|---|---|---|
| `enabled` | bool | no | `true` | — | Serve the embedded web UI at `GET /`. Set `false` to run `foxxycode http` as an API-only server; `/v1/*` and `/foxxycode/*` stay available. |
| `locale` | string | no | `""` (auto) | — | UI language: empty (auto-detect system/browser locale), `en`, or `ru`. |
| `send_mode` | string | no | `enter` | — | How the main chat composer submits: `enter` (Enter sends, Shift/Ctrl+Enter insert a newline), `ctrl_enter` (Ctrl/Cmd+Enter sends, Enter inserts a newline), or `off` (keyboard send disabled, Send button only). |
| `status_line` | bool | no | `true` | — | Show the live status line next to the typing dots while the agent works: current tool and its target, waiting for the model, and elapsed time. Set `false` to show only the animated dots. |

## `browser`

Interactive browser automation tool (`config.BrowserConfig`, `internal/config/browser.go`). Drives a local Chrome/Chromium over the DevTools Protocol via chromedp. Requires the `browser` build tag; disabled by default.

| Field | Type | Required | Default | Env fallback | Description |
|---|---|---|---|---|---|
| `enabled` | bool | no | `false` | — | Turns on the interactive browser tools (navigate, click, fill, screenshot, ...) for builds compiled with the `browser` tag. |
| `headless` | bool | no | `true` | — | Run the browser without a visible window. Set to `false` to watch the automated session. |
| `screenshots` | bool | no | `true` | — | Capture a screenshot after each action and show it to the model. Set to `false` to drive the browser text-only: actions still report the URL and the page log, and `foxxycode_browser_read_page` plus `foxxycode_browser_evaluate` read the page as text. Useful for a model without vision, and it drops the base64 image from every request. |
| `executable_path` | string | no | `""` (auto) | — | Path to a specific Chrome/Chromium binary; empty lets chromedp auto-detect an installed browser. |
| `timeout_seconds` | int | no | `30` | — | Per-action timeout for navigation, clicks, and other browser operations. |

## `vcs`

Version control integration (`config.VCSConfig`, `internal/config/vcs.go`). Git needs no configuration; this section carries the Subversion settings.

### `vcs.svn`

Subversion support: working copy detection for the composer chips plus the `svn_*` tools (`svn_info`, `svn_status`, `svn_diff`, `svn_log`, `svn_list`, `svn_add`, `svn_revert`, `svn_resolve`, `svn_update`, `svn_commit`, `svn_switch`, `svn_merge`, `svn_checkout`). Detection is independent of git, so a branch folder that also holds a git repository reports both.

| Field | Type | Required | Default | Env fallback | Description |
|---|---|---|---|---|---|
| `enabled` | bool | no | `true` | — | Turns Subversion support on. `false` hides the SVN chip and removes every `svn_*` tool from the definitions sent to the model. |
| `binary` | string | no | `""` (PATH) | — | Path to the svn client; empty resolves `svn` on `PATH`. Set it when the client is installed outside `PATH`. |
| `timeout_seconds` | int | no | `120` | — | Per-command timeout for svn invocations such as update, commit, and merge. |
| `branch_lookup` | bool | no | `true` | — | Allows listing `trunk` and `branches/` for the SVN chip menu. This contacts the server; turn it off on slow links. |

When no svn client is installed, detection reports `available: false`, the chip stays hidden and the tools are not registered — nothing else changes.

## Related environment variables

These control config discovery itself, not individual fields (see [config.md](config.md#config-file-location-and-paths)):

| Variable | Flag equivalent | Meaning |
|---|---|---|
| `FOXXYCODE_HOME` | `--home` | Agent state directory (default `~/.foxxycode`). |
| `FOXXYCODE_CWD` | `--cwd` | Default session working directory. |
| `FOXXYCODE_CONFIG` | `--config` | Explicit path to `config.yaml`. |
| `NAME_API_KEY` | — | Per-provider API key fallback (see [`providers`](#providers)). |
| `TELEGRAM_BOT_TOKEN` | — | Telegram bot token fallback (see [`gateways.telegram`](#gatewaystelegram)). |
