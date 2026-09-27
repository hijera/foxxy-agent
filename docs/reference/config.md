# `config.yaml` reference

Field-by-field reference for `~/.foxxycode/config.yaml`. For narrative documentation (file discovery, `.env`, provider guides) see [Configuration](../getting-started/configuration.md).

A machine-readable [JSON Schema](../../internal/config/config.schema.json) accompanies this reference, published at **https://hijera.github.io/foxxy-agent/config.schema.json** and embedded into the binary, where `foxxycode -t` / `--test-config` checks a file against it from the command line (see [Configuration](../getting-started/configuration.md#checking-the-file-from-the-command-line)). Point your editor's YAML language server at it to get autocomplete and typo checking:

```yaml
# yaml-language-server: $schema=https://hijera.github.io/foxxy-agent/config.schema.json
```

VS Code (with the YAML extension), Zed, Neovim and Helix pick this comment up automatically, and FoxxyCode writes it into every `config.yaml` it saves (see [Configuration](../getting-started/configuration.md)); JetBrains IDEs do not read it and need the URL registered under **JSON Schema Mappings** instead. The schema is kept in sync with the Go config structs by `TestDocsConfigSchemaMatchesStructs` in `internal/config/docs_schema_test.go`. Optional tri-state fields (for example `compaction.enable`, `models[].stream`, `tools.output_limits.*`) accept `null` as well as a value: `null` means unset, and that is the form FoxxyCode writes for everything you never set, so a saved config validates clean. On/off switches are spelled `enable`, as in coddy-agent; `enabled`, the name FoxxyCode used before 0.3.x, is still read in every section that has the switch, reported by `foxxycode -t` as a warning and written back as `enable` by the next save (see [Configuration](../getting-started/configuration.md#checking-the-file-from-the-command-line)).

Every field is optional unless marked **required**; an empty `config.yaml` (or none at all) is valid and uses built-in defaults. Any string value may reference environment variables with `${VAR_NAME}` (expanded when the file is loaded). To keep a **literal `$`** in a value (e.g. a secret like `$2y$10$…`), double it as `$$` - the UI does this automatically for the `proxy` fields. `${FOXXYCODE_HOME}` is expanded by the loader; `${CWD}` stays in the loaded value and is expanded per session by whatever reads the path, except in the process-scoped `sessions.dir`, `scheduler.dir`, `memory.dir`, and `logger.file` (see [Configuration](../getting-started/configuration.md#environment-variable-references)).

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

## Field reference

Generated from `internal/config/config.schema.json` (the schema embedded into the binary, published at <https://hijera.github.io/foxxy-agent/config.schema.json>) and from the defaults the loader applies, by `make docs`; `make docs-check` fails when this block is stale. Types read as the schema declares them (`string or null` is a tri-state field), defaults are what an empty `config.yaml` resolves to, and the description is the `doc:` line `foxxycode -t` prints for the key. Notes that do not fit a table row follow in [Notes](#notes).

<!-- docsgen:config:start -->
### `providers`

API credentials and transport selection for upstream LLM vendors. When api_key is empty, the runtime reads the NAME_API_KEY environment variable (NAME is the provider name in uppercase with hyphens mapped to underscores).

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `providers` | list of objects |  | API credentials and transport selection for upstream LLM vendors. When api_key is empty, the runtime reads the NAME_API_KEY environment variable (NAME is the provider name in uppercase with hyphens mapped to underscores). |
| `providers[].name` | string |  | Logical id used as the first segment of models[].model. ASCII letters, digits, hyphen, underscore; must start with a letter. |
| `providers[].type` | string, one of `openai`, `anthropic`, `neuraldeep`, `codex` |  | Wire protocol for this provider. Use "openai" for configurable OpenAI-compatible endpoints, "anthropic" for Anthropic, "neuraldeep" for NeuralDeep's OpenAI-compatible API at one of its two official deployments (selected with api_base), or "codex" for ChatGPT OAuth against the official Codex backend. |
| `providers[].api_base` | string |  | Optional base URL override. For type "openai" include /v1; for type "anthropic" use an Anthropic-compatible gateway. For type "neuraldeep" it selects the deployment: https://api.neuraldeep.ru/v1 (Russia, the default) or https://api.neuraldeep.tech/v1 (the international mirror); any other value falls back to the default. Ignored for type "codex", which always uses a fixed official endpoint. |
| `providers[].api_key` | string |  | Provider secret. A literal key, a "${ENV}" reference expanded at load time, or empty to read NAME_API_KEY at LLM call time. Resolution order: api_key -> api_key_command stdout -> NAME_API_KEY. |
| `providers[].api_key_command` | string |  | Optional credential-helper command. When api_key is empty it runs via the detected host shell (pwsh, powershell, or cmd on Windows; bash or sh elsewhere) and the trimmed stdout is used as the key. On failure resolution falls back to NAME_API_KEY. |
| `providers[].proxy` | string |  | Optional per-provider outbound proxy URL: http://, https://, socks5://, or socks5h:// (socks5h resolves hostnames via the proxy). Overrides a proxy inherited from the environment (HTTP_PROXY/HTTPS_PROXY, e.g. forwarded by the IDE plugin); NO_PROXY is still honored and local addresses always connect directly. When empty, the environment proxy is used. |
| `providers[].timeout_ms` | integer | 0 | Optional bound on each LLM HTTP request to this provider, including the streamed body read. 0 (the default) sets no client timeout, so slow prompt processing on large contexts is never cut short. |
| `providers[].usage_limits_panel` | boolean | true | Show this provider's account usage panel (the console footer line and /usage, the usage section and banner in the web UI) and read the provider's usage endpoint for it (GET /v1/limits for type neuraldeep). Omit or true keeps the panel on; false hides it and stops those reads for this row. Only providers whose type has a usage source are affected. |

### `models`

Named model entries the agent and UI can select.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `models` | list of objects |  | Named model entries the agent and UI can select. |
| `models[].model` | string |  | "provider_name/api_model_id": the first path segment must match a providers[].name; the remainder is sent to the LLM API (may itself contain slashes). |
| `models[].max_tokens` | integer |  | Upper bound on completion tokens per assistant message. A codex model never sends it (the Codex backend rejects max_output_tokens), so there it bounds nothing, and foxxycode -t and --dry-run warn about such a value. |
| `models[].temperature` | number |  | Sampling temperature (0 = deterministic; higher = more random). |
| `models[].max_context_tokens` | integer | 0 | Context window of the model in tokens: what the web UI context ring, the console context percentage and the automatic compaction trigger measure against. 0 reads it from the provider's model listing when the provider reports one (the NeuralDeep hub, vLLM, OpenRouter, LM Studio), else 128000. |
| `models[].multimodal` | boolean | false | Model accepts image/file inputs in addition to text; the UI shows a file attachment button for this model. |
| `models[].reasoning_levels` | list of strings |  | Override the reasoning levels offered for this model. Omit to auto-detect from the model id (gpt-5* -> minimal,low,medium,high; OpenAI o-series, gpt-oss*, qwen3*, and Claude extended-thinking models -> low,medium,high). An explicit empty list hides the selector. Settings fills this field from GET /foxxycode/config/reasoning-levels behind its Fetch reasoning levels button. |
| `models[].reasoning_default` | string |  | Reasoning level pre-selected for new chats; must be one of the resolved levels, otherwise ignored. |
| `models[].stream` | boolean | true | Transport used to talk to this model. Omit (or true) to stream over SSE. false issues one blocking completion request and delivers the whole answer at once, for servers or proxies that handle event streams badly. Rejected for providers of type codex, whose backend is streaming-only. |

### `agent`

Defaults for the main agent loop (model id and safety caps).

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `agent.model` | string |  | Default models[].model id used until the client overrides the model per session. Required when models is non-empty. |
| `agent.max_turns` | integer | 30 | Hard cap on LLM calls per prompt turn. |
| `agent.max_tokens_per_turn` | integer | 200000 | Max tokens across all LLM calls in one turn. |
| `agent.llm_retry_max` | integer | 3 | Retries after retryable LLM errors such as HTTP 429. An explicit 0 disables retries. |
| `agent.llm_retry_base_ms` | integer | 1000 | Initial backoff between LLM retries, in milliseconds. A server-provided pause (Retry-After-Ms / Retry-After headers, "Limit resets at" / "retry in Ns" body phrases) overrides the exponential backoff, capped at 60s. |
| `agent.llm_min_interval_ms` | integer | 0 | Minimum gap between consecutive LLM calls in milliseconds, retry attempts included (0 disables pacing; e.g. 12000 on strict free tiers). |
| `agent.llm_first_token_timeout_ms` | integer | 90000 | How long a streamed LLM call may stay silent before the turn cancels it (the API hang guard). An explicit 0 disables the guard; blocking (stream: false) transports are never guarded. |
| `agent.llm_stall_timeout_ms` | integer | 300000 | How long a streamed LLM call that has already produced output may go without any sign of progress before the turn cuts it, keeping the partial answer. Covers the case the first-token guard cannot: it stops at the first token and never re-arms. An explicit 0 disables the guard; blocking (stream: false) transports are never guarded. |
| `agent.llm_stall_retry` | boolean | true | When an LLM call fails without producing any output - a silent provider, a dropped connection (unexpected EOF), a provider-side timeout (Client.Timeout), a 5xx - wait and re-issue the same request instead of failing the turn. A request the endpoint refused (4xx) is not retried, and neither is a call that already streamed something, so no output can be duplicated. |
| `agent.llm_stall_retry_delays_ms` | list of integers | [60000,180000,300000] | Pause before each retry, in milliseconds. The last entry repeats for every later attempt, so [60000, 180000, 300000] means one minute, then three, then five minutes for every attempt after that. |
| `agent.llm_stall_retry_max_wait_ms` | integer | 3600000 | Total time that may be spent waiting between retries of one LLM call before the turn gives up and surfaces the error. An explicit 0 retries until the model answers or the user stops the turn. |
| `agent.loop_guard` | boolean | true | Runaway-loop protection: cut a streamed response that degenerates into repeating itself, block a tool called over and over with identical arguments, and block a sequence of calls the model keeps rotating through. |
| `agent.loop_tool_repeat_limit` | integer | 3 | Consecutive identical tool calls (same name, same canonical arguments) before the loop guard steps in. 0 disables the tool-repeat check. |
| `agent.loop_stream_repeat_cycles` | integer | 5 | Identical back-to-back output cycles inside one streamed response before the stream is cut. 0 disables the stream check. |
| `agent.loop_tool_cycle_repeats` | integer | 3 | Repetitions of the same sequence of tool calls before the loop guard steps in. This is what catches a model rotating through several calls (read A, read B, read A, ...) instead of repeating one; a lap that varies slightly still counts. 0 disables the cycle check. |
| `agent.loop_stuck_action` | string, one of `quarantine`, `stop` | quarantine | What the loop guard does once a tool loop has survived every nudge. "quarantine" (default) blocks the looping calls for the rest of the turn and lets it continue to a real answer; "stop" ends the turn with a notice. A degenerate output stream always stops the turn regardless. |
| `agent.loop_nudge_max` | integer | 2 | How many times one turn may be nudged back on track before the loop guard stops it with a notice. |
| `agent.wait_for_limit_reset` | boolean | false | Wait for a hit usage limit to lift and re-issue the call, instead of ending the turn with the provider's error. Applies to a top-level turn whose provider names a pause beyond the retry budget (a 429 with Retry-After or "Limit resets at"); the turn lock and the client stream stay open while it waits, so it is off by default. |
| `agent.wait_for_limit_reset_max_ms` | integer | 14400000 | Longest time one turn spends waiting for limits in total, in milliseconds, the retry wrapper's own sleeps on a limit included; a pause that would exceed it ends the turn at once with the error. An explicit 0 never waits. |

### `autocomplete`

LLM-backed inline completion in the editor plugins: the greyed suggestion drawn ahead of the caret and accepted with Tab. Editors fetch it over POST /foxxycode/completion, one single-shot LLM call with no tools and no agent loop.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `autocomplete.enable` | boolean | false | Turn on inline suggestions. Unset defaults to false, unlike the other optional passes: a suggestion is requested as you type, so this spends tokens on every keystroke. |
| `autocomplete.model` | string | "" | Exact models[].model id used for the suggestion pass; empty falls back to agent.model. Speed matters more than cleverness here, because a suggestion is worthless once you have typed past it. |
| `autocomplete.mode` | string, one of `auto`, `chat`, `fim` | auto | How the hole in the code reaches the model. "auto" uses native fill-in-the-middle tokens through a raw completion (POST /v1/completions) when the model family (Qwen-Coder, DeepSeek-Coder, CodeLlama, StarCoder, Codestral) and the provider allow it, and a chat prompt otherwise; a raw call that fails switches that model to chat for the rest of the process. "chat" always sends a chat prompt. "fim" always sends FIM tokens and errors when that is not possible. |
| `autocomplete.temperature` | number | 0 | Sampling temperature for suggestions. Unlike models[].temperature, 0 here is the value rather than "unset": suggestions are greedy by default so the same context yields the same suggestion. |
| `autocomplete.max_tokens` | integer | 128 | Completion token cap for one suggestion. Small values keep suggestions short and quick. |
| `autocomplete.related_files` | integer | 3 | How many other open editor tabs (reported over POST /foxxycode/ide/editor-state, workspace files only) are excerpted into the prompt so the model sees symbols from neighbouring files. 0 disables it. |
| `autocomplete.timeout_ms` | integer | 4000 | How long one suggestion request may take before the editor abandons it. |
| `autocomplete.debounce_ms` | integer | 350 | How long typing must pause before an automatic request goes out. Ignored when trigger is manual. |
| `autocomplete.trigger` | string, one of `auto`, `manual` | auto | When to ask the model: "auto" suggests while you type, after the debounce pause; "manual" suggests only on the editor shortcut. |
| `autocomplete.multi_line` | boolean | true | Allow one suggestion to span several lines. When false only the first line of a suggestion is kept. Unset defaults to true. |
| `autocomplete.max_prefix_bytes` | integer | 8000 | How much of the text before the caret is sent as context. |
| `autocomplete.max_suffix_bytes` | integer | 2000 | How much of the text after the caret is sent as context. |

### `prompts`

Override the built-in system prompt templates (Go text/template).

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `prompts.dir` | string | "" | Directory containing prompt template files. Empty uses embedded defaults. Supports ~ and ${CWD} (session cwd at render time). |
| `prompts.agent_prompt` | string | agent.md | File name inside prompts.dir for agent mode. |
| `prompts.plan_prompt` | string | plan.md | File name inside prompts.dir for plan mode. |
| `prompts.docs_prompt` | string | docs.md | File name inside prompts.dir for docs mode. |
| `prompts.ask_prompt` | string | ask.md | File name inside prompts.dir for ask mode. |
| `prompts.per_provider` | object |  | Select a system prompt tuned to the active model family (agent.<family>.md), falling back to the shared prompt. |
| `prompts.per_provider.enable` | boolean | true | Use a per-family prompt file when one exists. Defaults to true. |

### `instructions`

Files read from the session working directory and appended to the system prompt (AGENTS.md convention). The agent home's own AGENTS.md and DESIGN.md are read on top of this list whenever they exist, ahead of the project's pair, and are not named here; a file the rules block already carries, such as the root AGENTS.md or DESIGN.md, adds no second copy when it is listed. See https://github.com/hijera/foxxy-agent/blob/main/docs/features/rules.md.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `instructions.files` | list of strings | ["AGENTS.md","DESIGN.md"] | Instruction files, read in the order listed. ${FOXXYCODE_HOME}, ${CWD} and a leading ~ expand; an absolute entry is read as it stands, a relative one resolves against the session CWD. |

### `skills`

Directories scanned for skills (SKILL.md and root .md/.mdc files).

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `skills.dirs` | list of strings | ["~/.agents/skills","${FOXXYCODE_HOME}/skills","${CWD}/.foxxycode/skills"] | Search paths; later entries win on name conflicts. Defaults (lowest to highest priority): ~/.agents/skills, ${FOXXYCODE_HOME}/skills, ${CWD}/.foxxycode/skills. ${FOXXYCODE_HOME} and ${CWD} expand at runtime. |
| `skills.sources` | list of strings |  | Remote skill sources to install from: GitHub owner/repo[@ref], a git URL, or an http(s) URL to an agents-standard marketplace.json. Fetched on demand via Sync (never automatically) into the managed skills dir. EvilFreelancer/rpa-skills, the marketplace the bundled rpa-* skills are published from, is always in effect as a system source and is not listed here. See https://github.com/hijera/foxxy-agent/blob/main/docs/features/skills.md. |
| `skills.auto_discovery` | boolean | true | Offer the model-driven load_skill tool so the agent can pull a catalogued skill's instructions into a turn on its own. Unset defaults to true. |

### `rules`

Discovery of rule files from ${FOXXYCODE_HOME}/rules (the operator's own, applied in every workspace) and from .foxxycode/rules, .foxxyrules, .foxyrules, .agents/rules, .cursor/rules, .claude/rules, .codex/rules under the session CWD, plus the AGENTS.md and DESIGN.md of a folder a tool enters; .mdc files are Cursor rules, .md files Claude Code rules. See https://github.com/hijera/foxxy-agent/blob/main/docs/features/rules.md.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `rules.auto_discover` | boolean | true | Scan the session CWD rule roots automatically. |
| `rules.systems` | list of strings | [] | Restrict which rule systems are loaded: user (${FOXXYCODE_HOME}/rules), foxxycode, agents-dir (.agents/rules), cursor, claude, codex, agents (nested AGENTS.md and DESIGN.md). Empty means all. |

### `mcp_servers`

Model Context Protocol servers connected for every new session.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `mcp_servers` | list of objects |  | Model Context Protocol servers connected for every new session. |
| `mcp_servers[].type` | string, one of `stdio`, `http`, `sse` |  | Transport. "stdio" (default when empty; url-only entries default to "http") runs a local command; "http" speaks streamable HTTP to url (with automatic legacy-SSE fallback); "sse" forces the legacy HTTP+SSE transport. |
| `mcp_servers[].name` | string |  | Stable id referenced by the agent; must be unique in this list. |
| `mcp_servers[].command` | string |  | Executable for stdio transport (leave empty when using an http url). ${CWD} expands to the session cwd. |
| `mcp_servers[].args` | list of strings |  | Argv passed after command for stdio servers. ${CWD} expands to the session cwd. |
| `mcp_servers[].env` | list of objects |  | Extra environment variables for the stdio child process. |
| `mcp_servers[].env[].name` | string |  | Environment variable name. |
| `mcp_servers[].env[].value` | string |  | Environment variable value. ${CWD} expands to the session cwd. |
| `mcp_servers[].url` | string |  | HTTP(S) endpoint when type is "http". ${CWD} expands to the session cwd. |
| `mcp_servers[].headers` | list of objects |  | Optional headers sent with MCP HTTP requests. ${CWD} in a value expands to the session cwd. |
| `mcp_servers[].headers[].name` | string |  | HTTP header name. |
| `mcp_servers[].headers[].value` | string |  | HTTP header value. |
| `mcp_servers[].insecure_skip_verify` | boolean |  | Accept the TLS certificate of an "http"/"sse" server without verifying it, so a self-signed or expired certificate connects. Removes the protection against a man in the middle; use only on trusted networks. |
| `mcp_servers[].disabled` | boolean |  | Skip connecting this server without removing its definition. |
| `mcp_servers[].disabled_tools` | list of strings |  | Tool names of this server hidden from the agent. |

### `mcp`

MCP settings that are not tied to a single server entry.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `mcp.project_trust` | string, one of `ask`, `allow`, `deny` | ask | Trust policy for the project-local <cwd>/.foxxycode/mcp.json, which travels with the checkout: "ask" keeps its servers cold until the operator approves that exact declaration for that workspace; "allow" starts them automatically (trusted workspaces only); "deny" never loads them. |

### `tools`

Filesystem and shell policy for built-in tools.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `tools.permission_mode` | string, one of `ask`, `accept_edits`, `bypass` | ask | When the agent asks for user approval: "ask" prompts for commands and file writes; "accept_edits" auto-approves writes but prompts for commands; "bypass" never asks (trusted environments only). |
| `tools.command_allowlist` | list of strings |  | Commands that never require permission. Exact or prefix match (prefix + space + any args). "*" allows all commands. |
| `tools.permission_timeout_seconds` | integer | 0 | How long a permission prompt may wait for the operator before the tool call is cancelled instead. 0 (default) waits forever; a positive value keeps an unresponsive client from holding the session turn lock indefinitely. |
| `tools.ssh_connect_timeout` | integer | 30 | TCP dial timeout for SSH connections (ssh_run_command tool), in seconds. |
| `tools.plan_no_self_run` | boolean | false | Forbid the model from starting to execute a plan on its own: in plan mode plan_exit is not offered and any tool outside the plan allowlist is refused instead of run. Only the user can start the implementation (Run plan). Editor plugins pass --plan-no-self-run to turn it on by default. |
| `tools.output_limits` | object |  | Maximum lines each tool result or error may return into the LLM context. Positive limits also apply a 64 KiB byte ceiling. 0 disables both limits; unset uses the built-in default. |
| `tools.output_limits.read` | integer | 1000 |  |
| `tools.output_limits.grep` | integer | 200 |  |
| `tools.output_limits.glob` | integer | 300 |  |
| `tools.output_limits.print_tree` | integer | 400 |  |
| `tools.output_limits.run_command` | integer | 500 |  |
| `tools.output_limits.ssh_run_command` | integer | 500 |  |
| `tools.output_limits.webfetch` | integer | 800 |  |
| `tools.output_limits.websearch` | integer | 200 |  |
| `tools.output_limits.default` | integer | 1000 |  |
| `tools.background` | object |  | Commands the agent runs detached in the session task pool instead of blocking a turn. The pool dies with the foxxycode process; each task's metadata and captured output stay in the session bundle, and a task interrupted by a restart is reported as orphaned. |
| `tools.background.enable` | boolean | true | Offer the background option on run_command and expose the background task tools. |
| `tools.background.max_concurrent` | integer | 5 | How many background tasks one session may run at once. 0 uses the default. |
| `tools.background.default_timeout_seconds` | integer | 900 | Hard limit for a task started without an explicit timeout and without a duration estimate. 0 uses the default. |
| `tools.background.max_timeout_seconds` | integer | 3600 | Ceiling applied to any requested or estimate-derived timeout. 0 uses the default. |
| `tools.background.output_buffer_bytes` | integer | 262144 | How much of each task's output stays in memory for the status ticker. The full log is still written to the session bundle. 0 uses the default. |
| `tools.websearch` | object |  | Which search engines the websearch tool asks, in what order their results merge, and what it may spend asking them. Each engine reports its own outcome next to the results, so a blocked backend is named rather than counted as "the web has nothing". |
| `tools.websearch.engines` | list of strings | ["brave","bing"] | Search engines to ask, in merge order. Unset asks brave then bing. "ddg" and "google" are not asked by default: measured from a server, DuckDuckGo answers every query with an anti-bot interstitial and Google renders its results in the browser. "searxng" needs searxng_url. |
| `tools.websearch.engine_timeout_seconds` | integer | 8 | Seconds one engine may take before it is reported as unavailable. 0 uses the default. |
| `tools.websearch.total_timeout_seconds` | integer | 20 | Seconds the whole search may take, however many engines it asks. 0 uses the default. |
| `tools.websearch.max_concurrent_engines` | integer | 4 | How many engines are asked at once. 0 uses the default. |
| `tools.websearch.snippet_chars` | integer | 320 | Maximum characters of one result description. 0 uses the default. |
| `tools.websearch.cache_ttl_seconds` | integer | 300 | Seconds one engine answer is reused before the engine is asked again, so a repeated search does not repeat the request. 0 uses the default; a negative value turns caching off. |
| `tools.websearch.searxng_url` | string | "" | Base address of your own SearXNG instance, asked over its JSON API (enable the json format in its settings.yml). A self-hosted aggregator is the durable answer to a scraped engine being turned away; localhost and LAN addresses are allowed on purpose. |
| `tools.websearch.brave_api_key` | string | "" | Brave Search API subscription token. With it the brave engine uses the official JSON API instead of reading the public result page, which has no parser to break when Brave redeploys. Empty reads the BRAVE_API_KEY environment variable (also from ${FOXXYCODE_HOME}/.env), so the key need not be stored here. |
| `tools.http_request` | object |  | Policy of the http_request tool, the agent's curl. Under permission_mode ask or accept_edits a request asks the operator unless its destination is allowed here or was approved in the session; bypass never asks. See https://foxxycode.dev/docs/features/http-requests. |
| `tools.http_request.allowlist` | list of strings | [] | Destinations a request reaches without asking: a host (api.github.com), a subdomain wildcard (*.example.com), either with an optional :port, an origin (http://localhost:8080) or an address prefix (https://api.example.com/v1/). "*" allows every destination. An entry also covers the files a request uploads and an unchecked certificate; a proxy needs an entry of its own, and a file the response is saved to follows the write policy. |

### `subagents`

User-defined child agents the model can delegate to with spawn_agent. Definitions are markdown files with YAML frontmatter; each run is a background task of the parent session with its own child session and transcript. See https://github.com/hijera/foxxy-agent/blob/main/docs/features/subagents.md.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `subagents.enable` | boolean | true | Register the spawn_agent tool and list the subagent catalog in the system prompt. |
| `subagents.dirs` | list of strings | ["${FOXXYCODE_HOME}/agents","${CWD}/.claude/agents","${CWD}/.foxxycode/agents"] | Definition directories, lowest priority first; later entries override earlier ones by name. ${FOXXYCODE_HOME} and ${CWD} expand. Directories inside the workspace are project scope and follow project_trust. |
| `subagents.project_trust` | string, one of `ask`, `allow`, `deny` | ask | Trust policy for definitions found inside the workspace, which travel with the checkout: "ask" loads them but refuses to spawn one until the operator approved that exact file for that workspace on the machine running foxxycode (foxxycode agents trust there, or POST /foxxycode/subagents/{name}/trust with the session workspace as cwd); "allow" treats them like the operator's own files; "deny" never reads them. |
| `subagents.max_concurrent` | integer | 4 | How many subagent runs the whole process may have in flight at once, whatever session started them. Starting past the limit is refused, not queued. 0 uses the default. |
| `subagents.max_depth` | integer | 1 | How deep spawning may nest: 1 lets a session spawn subagents that cannot spawn further; 0 forbids spawning everywhere. Omit for the default. |
| `subagents.default_timeout_seconds` | integer | 1800 | Hard limit for one run whose definition and call give no timeout. Capped by tools.background.max_timeout_seconds. 0 uses the default. |
| `subagents.max_turns` | integer | 0 | ReAct rounds a child may take. 0 follows agent.max_turns. |

### `hooks`

Operator commands run at lifecycle points of a session: before and after a tool call, when a prompt is submitted, when the agent stops, on session start and around compaction. Definitions are JSON files in the Claude Code shape (~/.foxxycode/hooks.json, .foxxycode/hooks.json, .claude/settings*.json). See https://github.com/hijera/foxxy-agent/blob/main/docs/features/hooks.md.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `hooks.enable` | boolean | true | Load and run hooks at all. |
| `hooks.files` | list of strings | ["${FOXXYCODE_HOME}/hooks.json","${CWD}/.claude/settings.json","${CWD}/.claude/settings.local.json","${CWD}/.foxxycode/hooks.json"] | Definition files, lowest priority first; every matching hook runs. ${FOXXYCODE_HOME} and ${CWD} expand; a relative entry resolves against the session cwd. Files inside the workspace are project scope and follow project_trust; only the hooks key of a Claude Code settings file is read. |
| `hooks.project_trust` | string, one of `ask`, `allow`, `deny` | ask | Trust policy for hook files found inside the workspace, which travel with the checkout: "ask" lists them but runs nothing until the operator approved that exact file for that workspace on the machine running foxxycode (foxxycode hooks trust there, or POST /foxxycode/hooks/trust with the session workspace as cwd); "allow" treats them like the operator's own file; "deny" never reads them. |
| `hooks.default_timeout_seconds` | integer | 60 | Hard limit for one hook process whose definition gives no timeout. 0 uses the default. |
| `hooks.stop_loop_limit` | integer | 5 | How many times per turn a Stop hook may send the agent back to work. 0 uses the default. |
| `hooks.max_output_chars` | integer | 10000 | Cap on the context, messages and reasons one hook may hand to the model or the user; longer values are truncated with a marker. 0 uses the default. |

### `logger`

Process log level, outputs, format, and file rotation.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `logger.level` | string, one of `debug`, `info`, `warn`, `warning`, `error` | info | Minimum severity written to the configured outputs ("warning" is accepted as an alias of "warn"). |
| `logger.levels` | list of objects |  | Per-component minimum severity. A component is the dotted name a record carries in its "component" attribute (gateway, gateway.telegram, session, agent, scheduler); a parent name covers everything nested under it and the longest configured prefix wins. Records with no component follow "level". |
| `logger.levels[].component` | string |  | Dotted component name, for example "gateway.telegram". |
| `logger.levels[].level` | string, one of `debug`, `info`, `warn`, `warning`, `error` |  | Minimum severity for this component. |
| `logger.outputs` | list of strings | ["stderr"] | Where log records go; any combination of stdout, stderr, file. Empty means stderr only. |
| `logger.file` | string |  | Path for the file sink; required when outputs includes "file". Supports ${FOXXYCODE_HOME}. |
| `logger.format` | string, one of `text`, `json` | text | Log record format. |
| `logger.rotation` | object |  | Size-based rotation for the file sink. |
| `logger.rotation.max_size_mb` | integer | 0 | Rotate after the file reaches this size in MB; 0 disables size-based rotation. |
| `logger.rotation.max_files` | integer | 0 | Rotated backups to keep when max_size_mb > 0. |

### `sessions`

Where persisted session bundles are stored.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `sessions.dir` | string | "" | Sessions root directory. Empty resolves to ${FOXXYCODE_HOME}/sessions. Supports ${FOXXYCODE_HOME} and ~. |

### `compaction`

Summarize older turns when the conversation approaches the model's context window.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `compaction.engine` | string, one of `coddy`, `opencode` | coddy | Compaction implementation: "coddy" (default) keeps a summary row and replays only the window after it; "opencode" flags older turns and filters them from the payload. Both answer /compact, the compact endpoint and the compact_context tool, fold a long history in passes and walk fallback_models. |
| `compaction.enable` | boolean | true | Turn on auto-compaction. Unset defaults to true; set false to disable. |
| `compaction.model` | string | "" | Exact models[].model id used for the summarization pass; empty falls back to agent.model. |
| `compaction.fallback_models` | list of strings |  | Summarizer models tried in order when the one before them fails (models[].model ids). The session's own model is the last resort whether or not it is listed, so one unreachable model does not leave a session that ran out of room without a compaction. Both engines walk the same chain. |
| `compaction.threshold_percent` | integer | 80 | Trigger when context usage exceeds this percent of the model context window: its max_context_tokens, else the window its provider's model listing reports, else 128000. Default 80 (coddy) / 85 (opencode); the opencode engine clamps to 50..99. |
| `compaction.keep_recent_turns` | integer | 2 | Number of most recent user turns preserved verbatim (never summarized). Default 2. |
| `compaction.max_tokens` | integer | 4096 | Completion token cap for the summary generation (opencode engine only). |
| `compaction.result_eviction` | object |  | Collapses superseded read/grep results in the LLM projection while keeping the persisted transcript complete. |
| `compaction.result_eviction.enable` | boolean | true | Master switch for result eviction. |
| `compaction.result_eviction.keep_recent` | integer | 2 | Most recent evictable results kept as a working window. |
| `compaction.result_eviction.min_result_bytes` | integer | 2000 | Results at or below this size are never evicted. |
| `compaction.result_eviction.start_percent` | integer | 50 | Evict only once the estimated context reaches this percent of the effective model's max_context_tokens. Below it the replayed history is sent untouched, so the provider's prompt cache keeps it; a placeholder appearing mid-history invalidates every cached token behind it. 0 evicts from the first result; a model without max_context_tokens always does. |

### `title`

Generate a short LLM thread title after the first exchange in a fresh, non-pinned session. Runs backend-side for all clients.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `title.enable` | boolean | true | Turn on auto-title generation. Unset defaults to true; set false to disable. |
| `title.model` | string | "" | Exact models[].model id used for the title pass; empty falls back to agent.model. A small, cheap model is a good choice. |
| `title.max_tokens` | integer | 64 | Completion token cap for the title generation. |

### `memory`

Optional memory copilot (implementation in external/memory; enable at runtime with memory.enable).

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `memory.enable` | boolean | false | Turn on the memory copilot. |
| `memory.model` | string | "" | Exact models[].model id used only for recall/persist LLM calls; empty falls back to agent.model or the session override. |
| `memory.fallback_models` | list of strings |  | Memory copilot models tried in order when the one before them fails (models[].model ids). The session's own model is the last resort whether or not it is listed, so one unreachable model does not silence the copilot. |
| `memory.dir` | string | "" | Long-term memory root. Empty resolves to ${FOXXYCODE_HOME}/memory. Supports ${FOXXYCODE_HOME} and ~. |
| `memory.recall_max_turns` | integer | 6 | Bounds recall-side LLM rounds in the memory loop. |
| `memory.persist_max_turns` | integer | 12 | Bounds persist-side LLM rounds in the memory loop. |
| `memory.copilot_max_tokens` | integer | 4096 | Completion token cap for memory copilot LLM calls. |
| `memory.max_search_hits` | integer | 8 | Maximum snippets returned by memory_search. |

### `httpserver`

OpenAI-compatible HTTP API defaults (used only by binaries built with -tags http; the embedded SPA needs -tags http,ui). See https://github.com/hijera/foxxy-agent/blob/main/docs/reference/http-api.md.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `httpserver.enable` | boolean | true | Serve the HTTP API (and the embedded SPA) in this process. Omitted means true; set false on a node that only polls a messenger or relays a swarm. |
| `httpserver.host` | string | "" | Default bind address when foxxycode http does not pass -H/--host. Empty falls back to 0.0.0.0. |
| `httpserver.port` | integer | 0 | Default listen port when foxxycode http does not pass -P/--port. 0 falls back to 12345. |
| `httpserver.auth_token` | string | "" | Optional bearer credential for the HTTP API. Empty means no authentication. ${ENV} references are expanded at load; prefer --auth-token / FOXXYCODE_HTTP_TOKEN. Redacted from GET /foxxycode/config. See https://github.com/hijera/foxxy-agent/blob/main/docs/operate/remote.md. |
| `httpserver.login` | object |  | Optional password sign-in for the browser, so a server on a network is not readable by everyone who finds the port. Off unless an account exists here or in FOXXYCODE_HTTP_USER / FOXXYCODE_HTTP_PASSWORD. API clients keep using auth_token. See https://github.com/hijera/foxxy-agent/blob/main/docs/operate/remote.md. |
| `httpserver.login.enable` | boolean |  | Turn the sign-in form on or off explicitly. Omitted follows the credentials: an account here or in the environment enables it. false keeps the form off with the variables still set. |
| `httpserver.login.mode` | string, one of `password` | password | How a browser authenticates. Only "password" is implemented; the key exists so a trusted-proxy mode can be added without moving your configuration. |
| `httpserver.login.user` | string | "" | Account name for the sign-in form. Supports "${ENV}" references, e.g. "${FOXXYCODE_HTTP_USER}". |
| `httpserver.login.password_hash` | string | "" | argon2id hash of the password in PHC form ("$argon2id$v=19$m=...$..."), written by `foxxycode serve set-password`. Never echoed back by GET /foxxycode/config, and a save from the settings screen preserves it. Put a plaintext password in FOXXYCODE_HTTP_PASSWORD instead of here. |
| `httpserver.login.session_ttl_hours` | integer | 0 | How long a browser stays signed in. 0 means the cookie is dropped when the browser closes; the server still expires the session itself after 30 days, because a record it keeps forever is not a session. |
| `httpserver.public_docs` | boolean | false | Keep /docs and /openapi.* reachable without a token even when auth is enabled. |
| `httpserver.stream_tickets_only` | boolean | false | Refuse the durable auth token in ?access_token= on the SSE routes, so an EventSource must first mint a single-use ticket via POST /foxxycode/stream-tickets. Keeps the lasting credential out of access logs, proxy logs and browser history; breaks clients that pass the token in the URL. |
| `httpserver.allow_insecure` | boolean | false | Silence the startup warning about a non-loopback bind without authentication. |
| `httpserver.cors` | object |  | Cross-origin policy so a browser UI on another origin can call this API. |
| `httpserver.cors.enable` | boolean | false | Turn on CORS handling (preflight + Access-Control-* headers). |
| `httpserver.cors.allowed_origins` | list of strings |  | Exact origins permitted to call the API. A single "*" allows any origin (bearer auth still applies). |
| `httpserver.remotes` | list of objects |  | Remote foxxycode http servers offered in the UI environment selector. Tokens are not stored here; the UI keeps them client-side per remote. |
| `httpserver.remotes[].name` | string |  | Display name in the environment selector. |
| `httpserver.remotes[].url` | string |  | Base URL of the remote foxxycode http server. |

### `swarm`

Stateless relay that nodes register into and that chains into other relays. The relay itself needs -tags swarm and swarm.enable; the join list below is honoured by any foxxycode serve process, which is what makes an ordinary agent reachable through a relay. See https://github.com/hijera/foxxy-agent/blob/main/docs/operate/swarm.md.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `swarm.enable` | boolean | false | Run the swarm relay in this process. Independent of swarm.join, which is how an agent registers into a parent relay. |
| `swarm.host` | string | "" | Bind address for the relay when the CLI does not pass --swarm-host. Empty falls back to 0.0.0.0. |
| `swarm.port` | integer | 0 | Listen port for the relay. 0 falls back to 12346. |
| `swarm.name` | string | "" | Label for this relay in topology views and in a child's node path. |
| `swarm.auth_token` | string | "" | Bearer credential clients present to this relay. A relay reaches every node with that node's own credential, so an unauthenticated relay bound off loopback is a fleet-wide open door and the server refuses to start unless allow_insecure is set. Never echoed back. |
| `swarm.pairing_tokens` | list of strings | [] | Credentials a node must present to register. Empty closes registration unless insecure_open_registration is set. Never echoed back. |
| `swarm.allow_insecure` | boolean | false | Permit binding off loopback without a client token. |
| `swarm.insecure_open_registration` | boolean | false | Let any caller register a node without a pairing token. Development only. |
| `swarm.allow_private_upstreams` | list of strings | [] | Hosts a node may advertise even though they resolve into loopback or private ranges, which are otherwise refused so a registration cannot turn the relay into a probe of its own network. |
| `swarm.cors` | object |  | Cross-origin access for the relay API. The SPA reaches a relay from another origin by construction, so this usually has to be on. Allow-Headers includes Last-Event-ID so an SSE stream can be resumed through the relay. |
| `swarm.cors.enable` | boolean | false | Handle CORS preflight and emit Access-Control-* headers for allowed origins. |
| `swarm.cors.allowed_origins` | list of strings | [] | Exact origins permitted to call the API, e.g. "http://localhost:5173". A single "*" allows any origin. |
| `swarm.tls` | object |  | Serve the relay over HTTPS. Set both files or neither. Minimum TLS 1.2; certificates are startup state, so rotating them needs a restart. |
| `swarm.tls.cert_file` | string | "" | PEM certificate chain. |
| `swarm.tls.key_file` | string | "" | PEM private key. |
| `swarm.lease_ttl_seconds` | integer | 0 | How long a registration survives without a heartbeat. 0 falls back to 90; nodes refresh at a third of it. |
| `swarm.fanout_timeout_seconds` | integer | 0 | Per-node deadline for an aggregated call. A slower node degrades into a warning rather than stalling the answer. 0 falls back to 3. |
| `swarm.upstreams` | list of objects | [] | Nodes configured by hand rather than registered, for an agent that cannot run the join loop. |
| `swarm.upstreams[].name` | string |  | Node name; becomes a URL path segment, so letters, digits, underscore and hyphen only. |
| `swarm.upstreams[].url` | string |  | Origin the relay dials to reach the node. |
| `swarm.upstreams[].kind` | string, one of `""`, `agent`, `relay` | "" | agent (default) or relay. |
| `swarm.upstreams[].token` | string | "" | Bearer credential the relay presents to this node. Never echoed back. |
| `swarm.upstreams[].dial` | object |  | How this leg reaches the other side. Relays are expected to live in different networks, so a proxy hop and a private certificate authority are ordinary here. |
| `swarm.upstreams[].dial.proxy` | string | "" | Route the connection through a proxy: http, https, socks5 or socks5h. Empty falls back to the standard environment variables. Never echoed back. |
| `swarm.upstreams[].dial.ca_file` | string | "" | Certificate authority used to verify a peer whose certificate is signed privately. |
| `swarm.upstreams[].dial.insecure_skip_verify` | boolean | false | Accept any certificate. For a lab only; every use is logged. |
| `swarm.join` | list of objects | [] | Parent relays this process registers into on startup. Honoured whether or not this process runs a relay of its own, which is what lets relays chain. |
| `swarm.join[].url` | string |  | Parent relay origin. |
| `swarm.join[].name` | string | "" | Name to claim in that relay. Empty falls back to the host name. |
| `swarm.join[].pairing_token` | string | "" | Credential authorising the registration. Never echoed back. |
| `swarm.join[].advertise_url` | string | "" | Where the relay can reach this process. Leave empty to select the tunnel transport: this process dials out and serves its API back over that connection, which is the only way in when the network accepts no inbound connections. |
| `swarm.join[].token` | string | "" | This process's own bearer credential, handed to the relay so it can authenticate when it proxies. Prefer a credential minted for the relay alone. Never echoed back. |
| `swarm.join[].labels` | map of strings |  | Free-form tags shown in topology views. |
| `swarm.join[].dial` | object |  | How this leg reaches the other side. Relays are expected to live in different networks, so a proxy hop and a private certificate authority are ordinary here. |
| `swarm.join[].dial.proxy` | string | "" | Route the connection through a proxy: http, https, socks5 or socks5h. Empty falls back to the standard environment variables. Never echoed back. |
| `swarm.join[].dial.ca_file` | string | "" | Certificate authority used to verify a peer whose certificate is signed privately. |
| `swarm.join[].dial.insecure_skip_verify` | boolean | false | Accept any certificate. For a lab only; every use is logged. |

### `ui`

Embedded SPA preferences for desktop and HTTP UI.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `ui.enable` | boolean | true | Serve the embedded web UI at GET /. Turn off to run foxxycode http as an API-only server; /v1/* and /foxxycode/* stay available. Unset defaults to true. |
| `ui.locale` | string, one of `""`, `en`, `ru` | "" | UI locale for the embedded SPA. Empty means auto-detect from the system or browser locale. |
| `ui.send_mode` | string, one of `enter`, `ctrl_enter`, `off` | enter | How the main chat composer submits a message. "enter": Enter sends (Shift/Ctrl+Enter insert a newline). "ctrl_enter": Ctrl/Cmd+Enter sends (Enter inserts a newline). "off": disable keyboard send (Send button only). |
| `ui.status_line` | boolean | true | Show a live status line next to the typing dots while the agent works: the current tool and its target, waiting for the model, and elapsed time. Set false to show only the animated dots. Unset defaults to true. |
| `ui.effects` | boolean |  | Show the web UI's visual effects: the long-lived animations and the translucent frosted-glass panels (off, the panels turn opaque in their own colour). Written by the switch in Settings > Appearance and shared by every client. Unset means each client's default: off in the IntelliJ panel, which renders off-screen and copies every frame into the IDE (that slows the IDE down, especially without a GPU), on everywhere else. |

### `scheduler`

Cron-driven scheduled jobs (used only by binaries built with -tags scheduler). Jobs are flat *.md files with YAML frontmatter under scheduler.dir; five-field crontab in UTC.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `scheduler.enable` | boolean | false | Run the scheduler daemon and expose the foxxycode_scheduler_* tools. Can also be forced per process with foxxycode acp\|http -scheduler-enabled. |
| `scheduler.dir` | string | "" | Directory with *.md job definitions. Empty resolves to ${FOXXYCODE_HOME}/scheduler. |
| `scheduler.max_queue` | integer | 10 | Concurrent scheduled runs; when saturated, extra firings are skipped until a slot frees. |
| `scheduler.timeout` | string | 30m | Wall-clock limit for one scheduled agent run, as a Go duration (e.g. "30m", "1h30m"). |
| `scheduler.retain_sessions` | integer | 5 | Completed scheduler-run session directories kept per job_id under sessions.dir; older runs are pruned. |

### `gateways`

Messenger bot adapters (used only by binaries built with -tags gateway or -tags gateway.telegram; run with foxxycode gateway). See https://github.com/hijera/foxxy-agent/blob/main/docs/surfaces/gateway.md.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `gateways.telegram` | object |  | Telegram bot adapter. |
| `gateways.telegram.enable` | boolean | false | Activate the Telegram adapter when foxxycode gateway starts. |
| `gateways.telegram.token` | string | "" | Bot token from @BotFather. Leave empty to read the TELEGRAM_BOT_TOKEN environment variable (e.g. via ~/.foxxycode/.env). |
| `gateways.telegram.proxy` | string | "" | Optional outbound proxy for Telegram API requests: http, https, socks5, or socks5h URL. |
| `gateways.telegram.rich_messages` | boolean | false | Use Bot API 10.1 Rich Messages (native Markdown, streamed thinking placeholder, collapsible tool list). Falls back to legacy formatting when unsupported. |
| `gateways.telegram.admins` | list of integers |  | Telegram user IDs with elevated rights; admins always pass access checks. |
| `gateways.telegram.default_access` | string | all | Fallback access level for chats without an override: "all", "admins", or "group:<name>". |
| `gateways.telegram.default_isolation` | string, one of `individual`, `shared`, `admin` | individual | Fallback session isolation for group chats: "individual" (session per user), "shared" (one session for all), "admin" (admins only, shared session). |
| `gateways.telegram.user_groups` | list of objects |  | Named sets of user IDs referenced from access fields as group:<name>. |
| `gateways.telegram.user_groups[].name` | string |  | Group name referenced as group:<name>. |
| `gateways.telegram.user_groups[].user_ids` | list of integers |  | Telegram numeric user IDs in this group. |
| `gateways.telegram.chats` | list of objects |  | Per-chat overrides; chat_id is negative for groups and supergroups. |
| `gateways.telegram.chats[].chat_id` | integer |  | Telegram chat id (negative for groups/supergroups). |
| `gateways.telegram.chats[].isolation` | string, one of `individual`, `shared`, `admin` |  | Per-chat session isolation override. |
| `gateways.telegram.chats[].access` | string |  | Per-chat access override: "all", "admins", or "group:<name>". |

### `browser`

Interactive browser automation tool (requires the browser build tag; drives a local Chrome/Chromium via chromedp).

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `browser.enable` | boolean | false | Turns on the interactive browser tools (navigate, click, fill, screenshot, ...) for eligible builds. |
| `browser.headless` | boolean | true | Run the browser without a visible window. Enabled by default; disable to watch the automated session. |
| `browser.executable_path` | string | "" | Optional path to a specific Chrome/Chromium binary. Empty lets chromedp auto-detect an installed browser. |
| `browser.timeout_seconds` | integer | 30 | Per-action timeout for navigation, clicks, and other browser operations. |
| `browser.screenshots` | boolean | true | Capture a screenshot after each browser action and show it to the model. Enabled by default. Turn it off to drive the browser text-only: actions still report the URL and the page log, and foxxycode_browser_read_page plus foxxycode_browser_evaluate read the page as text - which is all a model without vision can use anyway, and it saves the base64 image on every request. |

### `vcs`

Version control integration. Git works out of the box; Subversion adds the SVN chip next to the git chip and the svn_* tools when a working copy is detected.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `vcs.svn` | object |  | Subversion support for SVN working copies and branch folders. |
| `vcs.svn.enable` | boolean | true | Turns Subversion support on. Enabled by default; turning it off hides the SVN chip and removes every svn_* tool from the model. |
| `vcs.svn.binary` | string | "" | Optional path to the svn client. Empty resolves "svn" on PATH; set it when the client is installed outside PATH. |
| `vcs.svn.timeout_seconds` | integer | 120 | Per-command timeout for svn invocations such as update, commit, and merge. |
| `vcs.svn.branch_lookup` | boolean | true | Allows listing trunk and branches/ for the SVN chip menu. This contacts the server; turn it off on slow links. |

### `debug`

Master switch for verbose diagnostics: debug-level logs, raw LLM HTTP capture, and per-session debug trace. The --debug CLI flag forces enable=true at startup.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `debug.enable` | boolean | false | Turn on the whole diagnostics layer: forces the process logger to debug level, captures raw LLM request/response bodies, traces every LLM connection (its route through a proxy, each network step, silences and how the request ended), and emits per-session debug trace events. Toggled at runtime through PUT /foxxycode/config. |
| `debug.capture_llm` | boolean | true | Log raw LLM HTTP request/response bodies at debug level. Unset (default) follows enable; set to false to keep debug logs while suppressing large bodies. |
<!-- docsgen:config:end -->

## Notes

### `providers`

List of LLM backends (`[]config.ProviderConfig`, `internal/config/providers.go`).

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
  - name: neuraldeep
    type: neuraldeep
    api_key: "${NEURALDEEP_API_KEY}"
    # usage_limits_panel: false  # hide the account usage panel for this row
  - name: codex
    type: codex # use Sign In with ChatGPT in the bundled web UI; no api_key needed
```

#### llama.cpp as an OpenAI-compatible provider

`llama-server` works as a `type: openai` provider (`api_base: "http://host:8080/v1"`). Recommended launch flags:

- `--jinja` - enables the model's chat template on the server, which is required for **tool calling**. Without it llama.cpp silently ignores the `tools` parameter and the agent loop degrades to plain text answers.
- `-c <n>` - set the context window large enough for an agent prompt (system prompt plus tool schemas plus history; 16k is a practical minimum, more is better). When a request exceeds the server context, llama.cpp reports `the request exceeds the available context size` - raise `-c` or trim `max_context_tokens`.

llama.cpp builds through 2025 report mid-stream failures with a non-standard SSE `error:` field; FoxxyCode understands both that dialect and the current `data: {"error": ...}` shape and surfaces the server's message in the error.

For `type: codex`, open **Settings → LLM Providers** in the bundled web UI and select **Sign In with ChatGPT**, or run the terminal equivalent for ACP and headless setups:

```bash
foxxycode providers login codex    # device flow: prints a URL and a one-time code, then waits
foxxycode providers list           # every provider with the credential its requests actually use,
                               # plus a warning for an openai row that has neither api_base
                               # nor a credential (its requests leave for api.openai.com)
foxxycode providers logout codex   # removes the FoxxyCode-managed credential (leaves the Codex CLI login alone)
```

(Up to 1.0.11 the same flow also had a codex-only command, `foxxycode codex login | status | logout`. It was removed in favour of the commands above, which cover every backend; `foxxycode providers list` reports what `codex status` used to.)

A successful login also publishes the subscription catalog into `config.yaml`, the way `foxxycode providers login neuraldeep` publishes its tier models (the Settings button stores the credential only): it adds the `codex` provider row when it is missing, one `models[]` entry per catalog model Codex lists (the ids Codex hides from its own picker, such as `gpt-reserve` and `codex-auto-review`, are left out), and `agent.model` when nothing is set yet - Codex's own top-ranked model. Nothing already in the file is rewritten: an existing provider row, an already listed model, and a chosen `agent.model` survive untouched, so a repeated login is a no-op. `--no-config` stores only the credential. Entries carry no `max_tokens`, because the Codex backend rejects `max_output_tokens`. A running `foxxycode serve` keeps its loaded config, so restart it to pick the new models up.

Both paths use the same storage; the provider name is the argument, so `foxxycode providers login <name>` targets a specific codex row when `config.yaml` defines several (a row under a name other than `codex` has to exist in `config.yaml` first). FoxxyCode uses the official device authorization flow and stores refreshable credentials at `$FOXXYCODE_HOME/providers/<provider-name>/codex-auth.json` with restrictive file permissions; tokens never enter `config.yaml`. `api_key`, `api_key_command`, and `api_base` are ignored for Codex, while `proxy` applies to OAuth and provider requests. A Codex model has no output cap FoxxyCode can set: the backend answers `Unsupported parameter` to `max_output_tokens`, so `models[].max_tokens` on a codex model bounds nothing. It is still accepted - the settings form seeds it on every model row - but `foxxycode -t`, `--dry-run` and the startup log of `foxxycode serve` warn about it, and a `max_tokens` sent on `POST /v1/chat/completions` for a codex model is refused with `400`. `max_context_tokens` is a different thing: the window the context ring and automatic compaction measure against, not a limit the provider enforces. The model picker reads the catalog from the official Codex backend with the saved token. If no FoxxyCode-managed credential exists, FoxxyCode remains compatible with a Codex CLI login in `~/.codex/auth.json` (or `$CODEX_HOME/auth.json`). Codex requests always target the official backend; the process-level `FOXXYCODE_CODEX_BASE_URL` is the only override (tests and self-hosted gateways), so a settings document can never redirect an OAuth token on its own.

Codex is only a model backend: the agent keeps FoxxyCode's own system prompt, tool catalog, permissions, and ReAct loop, and the ChatGPT credential is used solely to authenticate the Responses calls (`features/codex_auth.feature` pins this on both the HTTP and ACP surfaces).

**Token lifetime.** The access token is refreshed transparently shortly before it expires, and the refreshed tokens are written back to the file they came from. When the credential is the Codex CLI login, that file is `~/.codex/auth.json` itself - the same file the `codex` CLI reads, so both tools keep working off one login, and a refresh performed by FoxxyCode is visible to the CLI (and vice versa). A FoxxyCode-managed credential is refreshed in place under `$FOXXYCODE_HOME/providers/<name>/` and never touches the CLI login. Signing out (`foxxycode providers logout codex`, or **Sign Out** in Settings) removes only the FoxxyCode-managed file.

**Startup report.** When at least one `type: codex` provider is configured, `foxxycode acp` and `foxxycode serve` log one `codex credential` line per provider at startup: where the credential came from and how long the access token is still valid. A missing credential, an unusable `auth_mode`, or an expired token with no refresh token left is logged as a **warning** naming `foxxycode providers login <name>`; an expired but refreshable token is only an informational line, since the next request renews it. Setups without a codex provider log nothing.

**Reasoning.** The Codex backend serves `gpt-5*` and `gpt-6*` model ids but accepts only `none`, `low`, `medium`, `high`, and `xhigh`, so codex-backed models offer **`none`** where other providers offer `minimal` (an explicit `reasoning_levels: [minimal]` is remapped as well). Reasoning turns request summaries (`summary: auto`) so thinking streams into the UI, and encrypted reasoning (`include: reasoning.encrypted_content`) so the model's own chain of thought is replayed verbatim on the next request of the same turn - the same flow the Codex CLI uses. Replayed reasoning is tagged with the model that produced it and is skipped when the session switches models. The items are stored opaquely in `messages.json` (`reasoning_signature`, ~1 KB per assistant turn) and are not exposed by `GET /foxxycode/sessions/{id}/messages`.

### `models`

List of logical models (`[]config.ModelEntry`, `internal/config/models.go`).

```yaml
models:
  - model: "openai/gpt-5.6-terra"
    max_tokens: 8192
    reasoning_default: medium
    multimodal: true
  - model: "openai/gpt-5"
    max_tokens: 8192
    reasoning_default: medium
  - model: "local/qwen3-30b"
    max_tokens: 8192
    stream: false
```

**Non-streaming models.** `stream: false` changes one thing: foxxycode sends a single blocking `POST /chat/completions` instead of asking for SSE, and hands the finished answer to the rest of the runtime in one piece. Everything downstream is unchanged, so the transcript, tool calls, and session bundle look the same; what differs is that nothing appears until the model is done, and the thinking row shows no live duration. Two consequences are worth knowing before turning it on. Pressing **Stop** during a blocking call loses the whole answer, because the server has sent nothing yet, whereas a streamed turn keeps the tokens that already arrived. And a client asking for an SSE stream still gets one - the switch governs the connection to the LLM, not the connection to the client - which is why a streaming HTTP response now carries a keepalive comment every 15 s so proxies do not drop a turn that stays silent for minutes.

The switch is rejected for `type: codex` providers. The Codex Responses backend has no blocking mode, so honoring the key there would mean streaming anyway and only pretending not to.

### `agent`

ReAct loop settings (`config.Agent`, `internal/config/agent.go`).

### `autocomplete`

LLM-backed inline code completion (`config.AutocompleteConfig`, `internal/config/autocomplete.go`; always compiled): the greyed suggestion the editor plugins draw ahead of the caret and accept with Tab. Editors fetch it over `POST /foxxycode/completion` - one single-shot LLM call with no tools, no session and no agent loop - and read `GET /foxxycode/completion/config` to learn when to ask ([HTTP API](http-api.md)).

Unlike [`compaction`](#compaction) and [`title`](#title), this section is **off unless enabled explicitly**: a suggestion is requested as you type, so leaving it on by default would spend tokens on every keystroke. Point `model` at a small, fast `models[]` entry - speed beats cleverness here, because a suggestion is worthless once you have typed past it.

Retries are deliberately disabled for this pass regardless of `agent.llm_retry_max`: a retried suggestion lands after the user has typed past it. Qwen3-family thinking is pinned off (`chat_template_kwargs.enable_thinking: false`) regardless of the serving default, because a thinking model can spend the whole small budget inside its reasoning block and return nothing. Generation stops at sequences matched to the request - the line break for a single-line suggestion, the exact next suffix line and a blank-line run for a block - and, in chat mode, the streamed reply is cut the moment it dedents past the caret's scope. `GET /foxxycode/completion/stats` reports latency, token cost and the editor-reported acceptance rate.

**Judging quality on a live hub.** `make e2e-autocomplete` with `NEURALDEEP_API_KEY` set runs `external/httpserver/e2e_neuraldeep_autocomplete_test.go`: a dozen caret positions across Go, Python, TypeScript, JavaScript and Kotlin, each with a loose acceptance rule, run against every model in `FOXXYCODE_E2E_MODELS` in every prompt mode in `FOXXYCODE_E2E_MODES` (default `auto,chat`). It prints a markdown report with the completions and latencies, writes it to `FOXXYCODE_E2E_REPORT` when set, and only fails below `FOXXYCODE_E2E_MIN_SCORE`. It is skipped without the key, so CI never talks to the hub.

### `prompts`

System prompt template overrides (`config.Prompts`, `internal/config/prompts.go`). Template fields are documented in [Configuration](../getting-started/configuration.md#full-configuration-schema).

### `instructions`

Instruction files appended to the prompt (`config.Instructions`, `internal/config/instructions.go`). The default list is the operator's own `${FOXXYCODE_HOME}/AGENTS.md`, read in every workspace, followed by the project's `AGENTS.md`; writing a list replaces it. See [rules.md](../features/rules.md#your-own-instructions-and-rules).

### `skills`

Skill discovery (`config.Skills`, `internal/config/skills.go`).

### `rules`

Rules discovery (`config.Rules`, `internal/config/rules.go`), from the workspace folders and from the operator's own `${FOXXYCODE_HOME}/rules`. See [rules.md](../features/rules.md).

### `mcp_servers`

MCP servers connected for every new session (`[]config.MCPServerConfig`, `internal/config/mcp_servers.go`).

```yaml
mcp_servers:
  - name: filesystem
    command: npx
    args: ["-y", "@modelcontextprotocol/server-filesystem", "/home/user"]
    disabled_tools: ["write_file"]
```

Servers can also be declared in Cursor-compatible mcp.json files: the user-global
`${FOXXYCODE_HOME}/mcp.json` (`~/.foxxycode/mcp.json` by default, like Cursor's
`~/.cursor/mcp.json`; together with this `mcp_servers` list it forms the "global"
scope) and the project-local `<workspace>/.foxxycode/mcp.json` ("local" scope). Each file holds a single
`mcpServers` object keyed by server name (`env` and `headers` are JSON objects;
per-tool switches use `disabledTools`). Later levels override earlier ones by
name: `mcp_servers` < `${FOXXYCODE_HOME}/mcp.json` < `./.foxxycode/mcp.json`. Entries from the
project-local file need a workspace approval before they are started - see
[`mcp`](#mcp) and `docs/features/mcp.md`.

### `mcp`

MCP settings that are not tied to a single server entry (`config.MCP`, `internal/config/mcp.go`).

Added for [issue #80](https://github.com/coddy-project/coddy-agent/issues/80).
Approvals are recorded in `~/.foxxycode/mcp-trust.json`, keyed by the canonical workspace path
and a digest of the command-bearing declaration (transport, command, args, env, url,
headers), so rewriting an approved entry asks again. Approve with `foxxycode mcp trust <name>`,
`POST /foxxycode/mcp/{name}/trust`, or the shield button in **Settings → MCP servers**. That tab
also edits this policy itself (`POST /foxxycode/mcp/project-trust`), so it is not rendered as a
separate settings section. See `docs/features/mcp.md`.

### `tools`

Permission policy (`config.Tools`, `internal/config/tools.go`).

#### `tools.output_limits`

Maximum lines a tool result or error may return into the LLM context (`config.ToolOutputLimits`). Every enabled line limit also applies a hard **64 KiB per-call byte ceiling**, preventing a minified file, base64 payload, or one-line MCP response from bypassing the guard. `0` disables both limits for that tool; an unset field falls back to the built-in default. Truncated output ends with a marker telling the model how to fetch the rest (`offset`/`limit` for `read`, a narrower pattern for `grep`, `page` for `websearch`).

#### `tools.background`

Bounds for background execution (`config.ToolBackground`). A backgrounded `run_command` returns a task id instead of output; `background_list`, `background_output`, `background_wait`, and `background_stop` collect the result later. The pool lives inside the running `foxxycode` process: each task mirrors its metadata and captured output into the session bundle under `background/<task_id>/`, and a task interrupted by a restart is reported as `orphaned` rather than as still running. `0` on any integer field means "use the default". See `docs/features/background-tasks.md`.

### `subagents`

Subagents (`config.Subagents`, `internal/config/subagents.go`): child agents the model delegates to with the `spawn_agent` tool. A definition is a markdown file with YAML frontmatter (`name`, `description`, `model`, `mode`, `tools`, `disallowed_tools`, `permission_mode`, `max_turns`, `timeout_seconds`, `background`, `hidden`) whose body is the child's role. Each run is a background task of the parent session with its own child session and transcript, so `background_list` / `background_output` / `background_wait` / `background_stop`, the Tasks panel and `GET /foxxycode/sessions/{id}/background-tasks` all see it. `0` on `max_concurrent`, `default_timeout_seconds` and `max_turns` means "use the default"; `max_depth` is the exception, omit it for the default `1`, because an explicit `0` forbids spawning everywhere. See `docs/features/subagents.md`.

Approvals for project-scope definitions are recorded in `~/.foxxycode/subagents-trust.json`, keyed by the canonical workspace path, the definition name and a digest of the file, so editing an approved file asks again. `permission_mode`, `tools` and `disallowed_tools` in a definition can only narrow what the parent could do, in every scope.

### `hooks`

Hooks (`config.Hooks`, `internal/config/hooks.go`): operator commands run at lifecycle points of a session. A hook reads one JSON document on stdin and answers with an exit code plus optional JSON on stdout; a `PreToolUse` hook can deny a tool call whatever the permission mode, approve it past the prompt, force the prompt, rewrite its arguments or add context, and a `PostToolUse` / `PostToolUseFailure` hook can add feedback. Definitions are JSON files in Claude Code's shape. `0` on every integer key means "use the default". See `docs/features/hooks.md`.

Approvals for project-scope files are recorded in `~/.foxxycode/hooks-trust.json`, keyed by the canonical workspace path, the workspace-relative file path and a digest of the file, so editing an approved file asks again.

### `logger`

Logging (`config.Logger`, `internal/config/logger.go`). ACP flags `--log-level`, `--log-output`, `--log-file`, `--log-format` override these when set.

`levels` raises or lowers verbosity one subsystem at a time, so chasing a Telegram command that does not work no longer means turning the whole process to `debug` and reading it out of everything else:

```yaml
logger:
  level: "info"
  levels:
    - component: "gateway.telegram"
      level: "debug"
```

A component is the dotted name a subsystem tags its logger with, and it stays on every record that subsystem writes, so a file can also be filtered by subsystem after the fact. A parent name covers everything nested under it and the longest configured prefix wins: `gateway` reaches `gateway.telegram` unless that name carries its own entry. A record from an untagged part of the process has no component and follows `level`. Configuring one component twice is an error rather than a silent winner.

`--log-level` accepts the same spec as a comma-separated list, which is how an operator running under systemd raises one subsystem for a single restart without editing the file:

```bash
foxxycode serve --log-level "info,gateway.telegram=debug"
```

A bare `--log-level debug` sets only the root level and leaves the configured entries alone; a spec that names components replaces them, so the flag is a complete statement of what to log.

### `sessions`

Session bundle storage (`config.Sessions`, `internal/config/sessions.go`).

### `compaction`

Context compaction (`config.Compaction`, `internal/config/compaction.go`): summarizing older conversation history so long sessions keep fitting the model context window. Applies to the manual compact command and the automatic threshold trigger.

Two engines share the section, selected by `engine`: the default **coddy** engine (the value keeps the upstream name) inserts a summary row and replays only the window from the last summary onward; the **opencode** engine flags older messages compacted and excludes them from the model payload while keeping them in the transcript. Both answer the manual `/compact` command, the HTTP compact endpoint and the model's `compact_context` tool, fold a history larger than the summarizer's window in passes and walk `fallback_models`. Either engine republishes the context estimate right after it folds history: the agent recomputes the `conversation` and `summary` categories over the window it actually sends, persists them next to the provider token counters in `stats.json` and emits `usage_update`, so the composer's context ring drops without a reload and a session reopened after a restart reports the compacted window.

#### `compaction.result_eviction`

Collapses unmarked `read`/`grep` tool results to short placeholders when building the LLM request (`config.ResultEviction`), so paging a large file or running a wide search cannot pin dead lines in every later turn. A result survives when the model marks it (the `keep_result` tool, or `keep: true` on the call) or when it is inside the most recent working window; a filesystem write to a file invalidates earlier reads/greps that covered it. The persisted session bundle keeps every result in full.

### `title`

Automatic session titles (`config.TitleConfig`, `internal/config/title.go`; always compiled). After the first exchange in a fresh session whose title is not pinned, a hidden internal "title" agent writes a short thread title. It runs on the backend, so every client - the SPA, the IntelliJ and VS Code panels, ACP editors, the console - gets the title, pushed live over the session-update stream. A title the user pinned always wins and is never overwritten; the automatic title is generated at most once per session.

### `memory`

Long-term memory copilot (`config.MemoryConfig`, `internal/config/memory.go`; implementation in `external/memory`, `memory` build tag).

### `httpserver`

OpenAI-compatible HTTP API defaults (`config.HTTPServerConfig`, `internal/config/http.go`; `http` build tag). See [http-api.md](http-api.md).

### `swarm`

Stateless relay that nodes register into and that chains into other relays (`config.SwarmConfig`, `internal/config/swarm.go`; `swarm` build tag for the server side, though `swarm.join` is honoured by every `foxxycode serve` process). See [swarm.md](../operate/swarm.md).

### `ui`

Embedded web UI (`config.UIConfig`, `internal/config/ui.go`; only meaningful with `-tags http,ui`).

### `scheduler`

Cron scheduler (`config.SchedulerConfig`, `internal/config/scheduler.go`; `scheduler` build tag). Job file format is described in [Configuration](../getting-started/configuration.md#scheduler-optional-build).

### `gateways`

Messenger gateways (`config.GatewayConfig`, `internal/config/gateway.go`; `gateway` or `gateway.telegram` build tag; run with `foxxycode serve`). See [gateway.md](../surfaces/gateway.md).

#### `gateways.telegram`

### `browser`

The interactive browser tool (`config.BrowserConfig`, `internal/config/browser.go`): a local Chrome or Chromium driven over the DevTools Protocol through chromedp. It needs the `browser` build tag and is off by default; the tools it registers are listed in [Tools](tools.md#browser) and the guide is [Browser tool](../features/browser-tool.md).

### `vcs`

Version control integration (`config.VCSConfig`, `internal/config/vcs.go`). Git needs no configuration; the section carries the Subversion settings.

#### `vcs.svn`

Subversion support: working copy detection for the composer chips plus the `svn_*` tools ([Tools](tools.md#subversion)). Detection is independent of git, so a branch folder that also holds a git repository reports both. When no svn client is installed, detection reports `available: false`, the chip stays hidden and the tools are not registered - nothing else changes.

### `debug`

The diagnostics switch (`config.Debug`, `internal/config/debug.go`), off by default and free when off; the guide is [Diagnostics](../operate/debugging.md). It is not related to the `debug` session mode: that one changes how the model works, this one changes what FoxxyCode records.

The `--debug` flag on `foxxycode acp`, `foxxycode http` and `foxxycode gateway` forces `enable: true` for that process. It only ever turns the layer on: the flag applies only when passed, so a default-false flag cannot silently override a config-enabled layer. `foxxycode desktop` and the console have no flag but honour `debug.enable` from `config.yaml`. `PUT /foxxycode/config` applies a change to `debug.enable` without a restart - the log level is re-set on a shared `slog.LevelVar` and the capture flag is atomic.

## Related environment variables

These control config discovery itself, not individual fields (see [Configuration](../getting-started/configuration.md#config-file-location-and-paths)):

| Variable | Flag equivalent | Meaning |
|---|---|---|
| `FOXXYCODE_HOME` | `--home` | Agent state directory (default `~/.foxxycode`). |
| `FOXXYCODE_CWD` | `--cwd` | Default session working directory. |
| `FOXXYCODE_CONFIG` | `--config` | Explicit path to `config.yaml`. |
| `FOXXYCODE_SWARM_TOKEN` | `--auth-token` (swarm) | Client credential for `foxxycode serve` (see [`swarm`](#swarm)). |
| `FOXXYCODE_SWARM_PAIRING_TOKEN` | `--pairing-token` | Registration credential for `foxxycode serve` (see [`swarm`](#swarm)). |
| `NAME_API_KEY` | - | Per-provider API key fallback (see [`providers`](#providers)). |
| `TELEGRAM_BOT_TOKEN` | - | Telegram bot token fallback (see [`gateways.telegram`](#gatewaystelegram)). |
