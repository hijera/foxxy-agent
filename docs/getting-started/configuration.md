# Configuration Reference

This page is the narrative guide. Two companion artifacts cover the full key list:

- **[config.yaml reference](../reference/config.md)** - field-by-field tables: type, default, env-var fallback, required/optional, examples.
- **[config.schema.json](../config.schema.json)** - JSON Schema (draft-07) for editor autocomplete and validation, published at **https://hijera.github.io/foxxy-agent/config.schema.json** so an editor resolves it without a checkout, and embedded into the binary (from `internal/config/config.schema.json`, which `make site-schema` republishes here) so `foxxycode -t` checks a file against the same document (see [Checking the file from the command line](#checking-the-file-from-the-command-line)). Any editor with a YAML language server (VS Code YAML extension, Zed, Neovim, Helix) validates keys and values as you type once the file carries this header line:

```yaml
# yaml-language-server: $schema=https://hijera.github.io/foxxy-agent/config.schema.json
```

**FoxxyCode writes that line itself.** Every save that rewrites `config.yaml` - the settings screen (`PUT /foxxycode/config`), `foxxycode mcp add`, a skill source, a provider login, the agent's own `config_set` / `config_commit` - adds the header when the file has none, and leaves a `$schema` you chose yourself (a pinned tag, a local path) alone. The same saves keep your comments, including commented-out keys, and the order the keys are already in; only the values change. JetBrains IDEs do not read the header; if `config.yaml` is not validated there, map the same URL by hand under **Settings - Languages & Frameworks - Schemas and DTDs - JSON Schema Mappings**. VS Code can be told the same thing without touching the file:

```json
"yaml.schemas": { "https://hijera.github.io/foxxy-agent/config.schema.json": ["**/.foxxycode/config.yaml"] }
```

The schema is kept in sync with the Go config structs by `TestDocsConfigSchemaMatchesStructs` (`internal/config/docs_schema_test.go`); CI fails when a config field is added or renamed without updating the schema.

That URL is GitHub Pages serving this repository's `docs/` folder from `main`, so the
file above **is** the published schema - there is no second copy to keep current. It
also means a merge to `main` publishes it immediately, and the schema sets
`"additionalProperties": false`: a renamed or removed key marks configs already on
disk as invalid the moment it goes live. Land such a change with, or after, the
release that understands the new key - never before it.

## Config File Location and Paths

Resolved locations use environment variables and flags (see README). In short:

- **`FOXXYCODE_HOME`** - agent state directory. Default **`~/.foxxycode`**. Holds `config.yaml`, `sessions/`, `skills/`, managed provider credentials under `providers/`, and **`scheduler/`** when using the optional cron scheduler.
- **`FOXXYCODE_CWD`** - default filesystem cwd when `session/new` sends an empty `cwd`. Default is the process working directory at startup. Same meaning as the **`--cwd`** flag when set.
- **`FOXXYCODE_CONFIG`** - explicit path to `config.yaml`. Same as **`--config`**.
- **`CODEX_HOME`** - Codex CLI state directory read by `type: codex` providers when no FoxxyCode-managed credential exists. Default **`~/.codex`**.
- **`FOXXYCODE_CODEX_BASE_URL`** - process-level Codex backend override. The default is the official backend; `providers[].api_base` is intentionally ignored for Codex.

If no **`--config`** is given, the loader uses **`$FOXXYCODE_HOME/config.yaml`** (default home **`~/.foxxycode`**). If that file is missing, it tries **`config.yaml`** in the process current working directory (**`$CWD`** at startup). If neither file exists, built-in defaults apply (no error).

When the primary file exists but is invalid (YAML parse or validation error), the loader automatically recovers from **`config.yaml.bak`** in the same directory (see **`internal/config/recovery.go`**). After every successful load the server writes **`config.yaml.bak`**. The HTTP **`PUT /foxxycode/config`** route (see **`docs/reference/http-api.md`**) also snapshots the current file to **`config.yaml.bak`** before overwriting, so a failed reload can be rolled back.

The `foxxycode acp` subcommand also accepts **`--home`** (override `FOXXYCODE_HOME`), **`--sessions-dir`**, and **`--session-id`**. Optional **`sessions.dir`** in the YAML overrides the sessions root when **`--sessions-dir`** is not set (default **`$FOXXYCODE_HOME/sessions`**).

## Checking the file from the command line

Every command that loads `config.yaml` also takes `-t` (long form `--test-config`): bare `foxxycode -t`, `foxxycode cli -t`, `foxxycode acp -t`, `foxxycode http -t` (the server the editor plugins start) and `foxxycode serve -t`. The flag checks the file that command would load - `--config PATH` and `--home DIR` pick it exactly as they do for a start, `~/.foxxycode/.env` is loaded and `${VAR}` references are expanded first - and exits without starting anything. It never writes: the recovery from `config.yaml.bak` that a normal load performs on a broken file (see above) does not run, so the file you are told about is the file on disk. The flag works in every build, the lean one without the `cli` tag included.

The check has two stages. First the document is validated against the JSON Schema above, the same one editors use, embedded into the binary. This is what catches the mistakes the loader accepts silently: `config.yaml` is decoded leniently, so an unknown or misspelled key (`enbaled` for `enabled`) is ignored rather than rejected, and a value of the wrong shape, a value outside an enum or a range, a missing required key or a duplicate key surfaces later as odd behaviour. Then the loader's own rules run on the parsed document: a model naming a provider that does not exist, a `file` output without `logger.file`, `agent.model` missing from `models`. Every problem is printed as `file:line:column: what is wrong`, with an indented `fix:` line saying how to correct it and, where the schema has one, a `doc:` line carrying the field's description:

```text
$ foxxycode serve -t
/home/me/.foxxycode/config.yaml:13:3: httpserver.enbaled: unknown key "enbaled" (the loader ignores it, so it has no effect)
    fix: did you mean "enabled"? keys allowed under httpserver: allow_insecure, auth_token, cors, enabled, host, port, public_docs, remotes, stream_tickets_only
    doc: Serve the HTTP API (and the embedded SPA) in this process. Omitted means true; set false on a node that only polls a messenger or relays a swarm.
/home/me/.foxxycode/config.yaml:16:10: logger.level: "verbose" is not an allowed value
    fix: use one of debug, info, warn, warning, error
    doc: Minimum severity written to the configured outputs ("warning" is accepted as an alias of "warn").
/home/me/.foxxycode/config.yaml:18:11: warning: subagents.enabled: "yes" is read as the boolean true, but the schema and editors expect true or false
    fix: write enabled: true (true or false, unquoted)
/home/me/.foxxycode/config.yaml: 2 errors, 1 warning
config test failed
```

The exit status is 1 when the file has errors and 0 otherwise, so the flag fits a deploy script right before `foxxycode serve restart`. Warnings (marked `warning:`) never fail the check: they flag spellings the loader still reads but the schema and editors reject - `yes` for a boolean, `40.0` for an integer, coddy's `enable` for `enabled` - and a file without the `# yaml-language-server:` header. A missing file is an error, since the flag exists to check the file a start would use. Values under secret-shaped keys (`api_key`, `auth_token`, `pairing_tokens`) are never echoed in a message.

A file that does not parse at all is placed differently from one whose values are merely wrong. The parser reports the line the block it was reading began on, which in a file with a header of comments is a blank line far above the mistake, so the check re-reads the file to find the line whose arrival stops it parsing and reports that one instead. A start prints the same line, so `foxxycode -t` and `foxxycode serve` send you to the same place.

What an editor leaves in the file is not part of the configuration. A file written on Windows ends its lines with a carriage return and a line feed and may carry a byte order mark in front of the first one; both are dropped on the way in, so the `# yaml-language-server:` header behind a mark is still found and a finding still names the line the editor shows, and a save puts the file's own line endings back. A file saved as UTF-16 - Notepad's "Unicode", and what a `>` redirect writes in Windows PowerShell 5.1 - is decoded on the way in too; the check names it in a warning, because a save from the settings screen writes the file back as UTF-8.

A config written for [coddy-agent](https://github.com/coddy-project/coddy-agent), or copied from its documentation, reads as well. Upstream spells every on/off switch `enable` (`httpserver.enable`, `memory.enable`, `gateways.telegram.enable`, `compaction.enable`, ...), where FoxxyCode spells it `enabled`. The loader takes `enable` as `enabled` in every section that has that switch, so such a file keeps its switches instead of silently falling back to the defaults, and `config_get` answers for the `enabled` path. The check names each one in a warning, because the schema and editors know only `enabled`; when a section sets both, `enabled` wins and the `enable` line has no effect. Loading never rewrites the file: the next write - a save from the settings screen, a `config_commit`, `foxxycode serve set-password` - renders the key as `enabled`, with the comment that stood above it. A key called `enable` that is not a switch, such as a swarm label, keeps its name.

## Dry run: probing what the file points at

`--dry-run` looks at the world the file describes, after the same check `--test-config` performs. Every command that takes `-t` takes it too: `foxxycode --dry-run`, `foxxycode cli --dry-run`, `foxxycode acp --dry-run`, `foxxycode http --dry-run`, `foxxycode serve --dry-run`, with `--config` and `--home` selecting the file as for a start. The static check runs first, and a file with errors stops there - probing what a broken file names would only bury the first mistake under its consequences. When the file is clean, the configuration is loaded without side effects (no `config.yaml.bak` written or restored) and probed:

- **paths** - `sessions.dir`, `logger.file`, `scheduler.dir` and `memory.dir` are fine when missing as long as they can be created (the process makes them at start), and an error when a regular file stands in the way; `prompts.dir` has to exist, and a template missing from it is a warning; `skills.dirs`, `subagents.dirs` and `hooks.files` entries you wrote are warnings when missing, while absent defaults stay quiet; a hook file that exists has to parse; `swarm.tls` must load and every `dial.ca_file` must hold a certificate;
- **LLM providers** - each provider is asked for its model list, which exercises the address, the proxy and the credential in one request (`foxxycode providers login` credentials included); a provider aimed at a vendor's official endpoint with nothing to present is reported without a request. Every `models[]` entry is then checked against that list: a model the server does not name is a warning, since some servers serve more than they list;
- **MCP servers** from `config.yaml` - the executable of a stdio server is resolved in `PATH` the way the spawn would, without spawning it; a remote server is asked for any HTTP answer, with its headers. Project-local `.foxxycode/mcp.json` declarations are not contacted: they sit behind the workspace trust gate;
- **Telegram** - when `gateways.telegram.enabled` is true the token is checked against the Bot API (`getMe`), through `gateways.telegram.proxy` when set; the report names the bot;
- **remotes** - each `httpserver.remotes[]` URL is asked for an answer (a warning when down, since it is used only on request), and the `--remote` target of a console or `acp` run has to accept the token;
- **listen addresses** - `foxxycode http` binds the address `-H`/`-P` select (`httpserver.host` and `httpserver.port` when the flags are left alone) once and releases it; `foxxycode serve` does the same for every subsystem the configuration and the typed flags enable, resolved as a start would resolve them (a surface this binary was not built with is an error, not a silent skip). A port another process holds is named together with the line that set it;
- **`foxxycode serve` only** - the relays in `swarm.join` and the upstreams a relay mounts are reached through their dial settings.

On its own the flag is quiet: it prints the problems - each `warning` and `error` with the place in the file and the fix - and one status line at the end, so a healthy setup answers with that line alone and a deploy script has one thing to read:

```text
$ foxxycode --dry-run
dry run: 0 errors, 0 warnings, 4 ok
```

When something is off, the problems come first and the status line still closes the report; the exit status is 1 and the last line says `dry run failed`:

```text
$ foxxycode --dry-run
warning  skills.dirs[0]: /home/me/.foxxycode/skills does not exist
         at /home/me/.foxxycode/config.yaml:22:10
         fix: create it or remove the entry; a ${CWD} entry is resolved per session, so a folder missing here may exist in another workspace
warning  skills.dirs[1]: /opt/team-skills does not exist
         at /home/me/.foxxycode/config.yaml:22:34
         fix: create it or remove the entry; a ${CWD} entry is resolved per session, so a folder missing here may exist in another workspace
error    mcp_servers[tickets]: command "ticket-mcp" not found in PATH
         at /home/me/.foxxycode/config.yaml:20:14
         fix: install it or write an absolute path in mcp_servers[tickets].command
warning  models[local/llama-4]: not in the model list of provider local (the server may still serve it)
         at /home/me/.foxxycode/config.yaml:11:5
         fix: check the model id; the provider lists gpt-oss-20b, qwen3.6-35b
error    providers[gpu]: cannot reach http://127.0.0.1:18732/v1: dial tcp 127.0.0.1:18732: connect: connection refused
         at /home/me/.foxxycode/config.yaml:6:5
         fix: check api_base and that the server is running
dry run: 2 errors, 3 warnings, 4 ok
dry run failed
```

Add `--test-config` to see the whole picture: the config check report first (the same one `-t` prints, `valid` included), then every probe, the ones that passed too, so the report shows what was actually tried and against which address:

```text
$ foxxycode --dry-run --test-config
/home/me/.foxxycode/config.yaml: valid
ok       sessions.dir: /home/me/.foxxycode/sessions will be created at first start
warning  skills.dirs[0]: /home/me/.foxxycode/skills does not exist
         at /home/me/.foxxycode/config.yaml:22:10
         fix: create it or remove the entry; a ${CWD} entry is resolved per session, so a folder missing here may exist in another workspace
warning  skills.dirs[1]: /opt/team-skills does not exist
         at /home/me/.foxxycode/config.yaml:22:34
         fix: create it or remove the entry; a ${CWD} entry is resolved per session, so a folder missing here may exist in another workspace
ok       mcp_servers[context7]: command "npx" resolves to /usr/bin/npx
         at /home/me/.foxxycode/config.yaml:17:14
error    mcp_servers[tickets]: command "ticket-mcp" not found in PATH
         at /home/me/.foxxycode/config.yaml:20:14
         fix: install it or write an absolute path in mcp_servers[tickets].command
ok       providers[local]: openai at http://127.0.0.1:18731/v1 lists 2 models
         at /home/me/.foxxycode/config.yaml:3:5
ok       models[local/qwen3.6-35b]: listed by provider local
warning  models[local/llama-4]: not in the model list of provider local (the server may still serve it)
         at /home/me/.foxxycode/config.yaml:11:5
         fix: check the model id; the provider lists gpt-oss-20b, qwen3.6-35b
error    providers[gpu]: cannot reach http://127.0.0.1:18732/v1: dial tcp 127.0.0.1:18732: connect: connection refused
         at /home/me/.foxxycode/config.yaml:6:5
         fix: check api_base and that the server is running
skipped  models[gpu/qwen3.6-35b]: provider gpu failed
dry run: 2 errors, 3 warnings, 4 ok
dry run failed
```

`ok` and `skipped` lines carry no fix; a `warning` never fails the run; an `error` does. A file that fails the static check is always shown, whichever flags were given: nothing else can be probed until it is fixed. Network probes run concurrently and each is bounded to ten seconds, so a dead server costs one wait, not one per model. Secrets are not echoed: a Telegram token is masked in any error text and a provider key is never printed. `CODDY_TELEGRAM_API_BASE` points the Telegram probe at a stand-in Bot API (tests and self-hosted gateways).

## Full Configuration Schema

Agent name, title, and build version are not configurable here. They are fixed in the binary and reported during ACP `initialize` (`internal/acp` and `internal/version`).

```yaml
# LLM backends (Go: []config.ProviderConfig, internal/config/providers.go)
# Each providers[].name must match ^[a-zA-Z][a-zA-Z0-9_-]*$ (ASCII letter first, then letters, digits, hyphen, underscore).
# api_key may be a literal, "${ENV}" expanded when the file loads, or empty to read NAME_API_KEY at LLM call time
# (NAME is the provider name in uppercase with hyphens mapped to underscores, for example rpa -> RPA_API_KEY).
# api_key_command (optional): when api_key is empty, this command is run via the detected host shell and its trimmed stdout is
# used as the key (credential helper, like git/docker helpers or AWS credential_process). It lets a provider fetch
# short-lived or login-issued keys without storing a static secret. On failure resolution falls back to NAME_API_KEY.
# Resolution order: literal api_key -> api_key_command stdout -> NAME_API_KEY env.
providers:
  - name: "openai"
    type: "openai"
    api_key: "${OPENAI_API_KEY}"
    # api_base: ""                    # optional override for OpenAI-compatible base URL
    # api_key_command: "my-cli print-token"  # host shell: pwsh/powershell/cmd on Windows; bash/sh elsewhere
    # proxy: "http://127.0.0.1:8888"   # optional per-provider HTTP(S) or SOCKS5/SOCKS5h proxy
    # timeout_ms: 300000               # optional bound on each LLM request incl. streamed read (0 = no client timeout)

  - name: "anthropic"
    type: "anthropic"
    api_key: "${ANTHROPIC_API_KEY}"

  - name: "local"
    type: "openai"
    api_base: "http://localhost:11434/v1"
    api_key: "~"

  - name: "deepseek"
    type: "openai"
    api_base: "https://api.deepseek.com/v1"
    api_key: "${DEEPSEEK_API_KEY}"

  # Use Sign In with ChatGPT in the bundled UI. Tokens are stored under
  # $FOXXYCODE_HOME/providers/codex/, not in config.yaml.
  - name: "codex"
    type: "codex"

# Logical models (Go: []config.ModelEntry, internal/config/models.go).
# Each model value is "provider_name/api_model_id". The first path segment must match providers[].name.
# The same string is the ACP model selector and agent.model default.
models:
  - model: "openai/gpt-4o"
    max_tokens: 8192
    temperature: 0.2
    multimodal: true              # accepts images/files; UI shows file attachment button

  - model: "anthropic/claude-3-5-sonnet-20241022"
    max_tokens: 8192
    temperature: 0.2
    multimodal: true

  - model: "openai/gpt-5"
    max_tokens: 8192
    reasoning_default: medium     # level pre-selected for new chats (composer reasoning selector)
    # reasoning_levels: [low, high]  # optional override of offered levels; [] hides the selector

  - model: "local/qwen2.5-coder:14b"
    max_tokens: 4096
    temperature: 0.1

  - model: "deepseek/deepseek-coder-v2"
    max_tokens: 8192
    temperature: 0.1

  - model: "codex/gpt-5.6-sol"
    max_tokens: 8192

# ReAct loop settings (Go: config.Agent, internal/config/agent.go)
agent:
  model: "openai/gpt-4o"       # required when models is non-empty; default LLM until the client overrides per session
  max_turns: 30                # max LLM calls per prompt turn
  max_tokens_per_turn: 200000  # max tokens across all calls in one turn
  llm_retry_max: 3             # retries after HTTP 429 and similar errors (default 3; an explicit 0 disables retries)
  llm_retry_base_ms: 1000      # initial backoff between LLM retries; a server-provided
                               # pause (Retry-After-Ms / Retry-After headers, "Limit resets
                               # at" / "retry in Ns" body phrases) overrides the backoff,
                               # capped at 60s
  llm_min_interval_ms: 0       # min gap between consecutive LLM calls, retries included; e.g. 12000 on strict free tiers
  llm_first_token_timeout_ms: 90000  # cancel a silent streamed LLM call after this long (0 disables the guard);
                                     # a reasoning model given a large tool result can need most of it
  llm_stall_timeout_ms: 300000 # cut a stream that has already produced output but stopped sending data
                               # (0 disables); the partial answer is kept and the model asked to continue
  llm_stall_retry: true        # wait and re-issue a call that failed without producing output
                               # (silence, unexpected EOF, Client.Timeout, 5xx); a refused 4xx is not retried
  llm_stall_retry_delays_ms: [60000, 180000, 300000]  # pause before each retry; the last entry
                               # repeats, so this is 1min, 3min, then every 5min
  llm_stall_retry_max_wait_ms: 3600000  # total time spent waiting between retries before giving up (0 = unbounded)
  loop_guard: true             # stop a response that repeats itself, and a tool called over and over with identical args
  loop_tool_repeat_limit: 3    # identical tool calls in a row before the guard steps in (0 disables)
  loop_stream_repeat_cycles: 5 # identical output cycles in one stream before it is cut (0 disables)
  loop_tool_cycle_repeats: 3   # repeats of the same sequence of calls before intervening (0 disables)
  loop_nudge_max: 2            # nudges before the guard acts on a loop
  loop_stuck_action: quarantine # then: block the looping calls and finish the turn, or "stop" it

# System prompt templates
prompts:
  # Empty dir = use embedded defaults. Otherwise a directory containing the files named below.
  #
  # Go text/template data. Fields in internal/prompts/loader.go. YAML shape is config.Prompts in internal/config/prompts.go.
  #   {{.CWD}}      - session working directory
  #   {{.Tools}}    - markdown list of tool names and short descriptions for the current mode
  #   {{.Skills}}   - markdown block for active skills (omit section when empty via {{if .Skills}})
  #   {{.TodoList}} - current session todo checklist as markdown lines (empty until foxxycode todo tools update state)
  #   {{.Memory}}   - session agent memory plus optional long-term recall when memory.enabled is true
  #   {{.UTCNow}}   - date and time in UTC (RFC3339), refreshed whenever the system prompt is rendered
  #
  # Built-in templates order: Tools, Skills, optional TodoList block, Memory (session notes plus optional recall), trailing Current UTC time.
  # The checklist section is emitted only when the session plan is non-empty.
  dir: ""
  agent_prompt: "agent.md"     # optional; default agent.md
  plan_prompt: "plan.md"       # optional; default plan.md
  # Ask and Debug have no configurable filename: they always read ask.md and debug.md.
  # Per-provider prompts: when enabled (default), each mode can pick a prompt
  # tuned to the active model, resolved most-specific first:
  #   <mode>.<model-slug>.md  per-model. The configured model-list slug resolves first
  #                           (openai/gpt-4o -> openai-gpt-4o), then the provider-neutral
  #                           API-model slug (local/gpt-oss-20b -> gpt-oss-20b)
  #   <mode>.<family>.md      per-family (anthropic, openai, gemini, gpt-oss, qwen,
  #                           gemma, neuraldeep); family defaults are built in
  #   <mode>.md               shared fallback
  # Place custom variants in dir. Set enabled: false to always use the shared prompt.
  per_provider:
    enabled: true

# Session bundle storage (Go: config.Sessions, internal/config/sessions.go)
sessions:
  # Empty = default $FOXXYCODE_HOME/sessions. Supports ${FOXXYCODE_HOME} and ~ in path.
  dir: ""

# Optional long-term memory copilot (Go: config.MemoryConfig, internal/config/memory.go; logic in external/memory).
# Implementation is always linked; enable at runtime with memory.enabled.
memory:
  enabled: false
  # Exact id from models[]. Used only for recall and persist tool-calling passes, not for the main assistant model.
  # Example: "rpa/gpt-oss:120b". Empty means fall back to agent.model / session override.
  model: ""
  dir: "" # long-term memory root; empty = $FOXXYCODE_HOME/memory. Supports ${FOXXYCODE_HOME} and ~ when set.
  recall_max_turns: 6
  persist_max_turns: 12
  copilot_max_tokens: 4096
  max_search_hits: 8

# Skills directories (Go: config.Skills, internal/config/skills.go)
skills:
  # Directories to search for SKILL.md and optional root .md/.mdc skill files.
  # Later entries have HIGHER priority: if the same skill name appears in multiple
  # directories, the version from the last matching directory wins.
  # Default dirs (lowest → highest priority):
  #   ~/.agents/skills          - global skills, shared with npx skills / npx skillsbd
  #   ${FOXXYCODE_HOME}/skills      - foxxycode-specific; may contain symlinks to ~/.agents/skills
  #   ${CWD}/.foxxycode/skills      - project-local; overrides everything above
  # ${FOXXYCODE_HOME} and ${CWD} expand at runtime (per-session cwd for ${CWD}).
  dirs:
    - "~/.agents/skills"
    - "${FOXXYCODE_HOME}/skills"
    - "${CWD}/.foxxycode/skills"

# Rules (Go: config.Rules, internal/config/rules.go)
# Discovered from .foxxycode/rules, the shared .agents/rules, .cursor/rules,
# .claude/rules, .codex/rules, and nested **/AGENTS.md under session CWD, plus
# your own ~/.foxxycode/rules, which applies in every workspace. Your own
# ~/.foxxycode/AGENTS.md is read too, ahead of the project's, and has no key here.
# .mdc files are read as Cursor rules, .md files as Claude Code rules.
# Injected into {{.Rules}} in the system prompt (separate from skills). See docs/features/rules.md.
rules:
  auto_discover: true
  systems: []   # optional: user, foxxycode, agents-dir, cursor, claude, codex, agents

# MCP servers available to all sessions (Go: []config.MCPServerConfig, internal/config/mcp_servers.go)
mcp_servers:
  - name: "filesystem"
    command: "npx"
    args: ["-y", "@modelcontextprotocol/server-filesystem", "/home/user"]
    env: []

  # HTTP MCP server example
  # - type: "http"
  #   name: "my-api"
  #   url: "https://my-mcp-server.example.com/mcp"
  #   headers:
  #     - name: "Authorization"
  #       value: "Bearer ${MY_API_TOKEN}"

# Tool configuration (Go: config.Tools, internal/config/tools.go)
tools:
  # Controls when the agent asks for user approval before running tools.
  # ask          - always prompt for commands and file writes (default)
  # accept_edits - auto-approve file writes; prompt for shell commands
  # bypass       - never ask for permission (use only in trusted environments)
  # Overridable per session via ACP session/set_config_option with configId "permission_mode".
  permission_mode: ask

  # How long a permission prompt may wait for the operator before the tool
  # call is cancelled instead (seconds; 0 by default = wait forever).
  # permission_timeout_seconds: 0

  # TCP dial timeout for SSH connections in seconds (default: 30).
  # ssh_connect_timeout: 30

# Subagents (Go: config.Subagents, internal/config/subagents.go). Child agents the model spawns with spawn_agent
# from markdown definitions; each run is a background task with its own child session. See docs/features/subagents.md.
# subagents:
#   enabled: true
#   dirs: ["${FOXXYCODE_HOME}/agents", "${CWD}/.claude/agents", "${CWD}/.foxxycode/agents"]
#   project_trust: ask            # ask (approve project files once per workspace) | allow | deny
#   max_concurrent: 4             # subagent runs in flight across the whole process
#   max_depth: 1                  # 1 = children cannot spawn further; 0 = nobody spawns
#   default_timeout_seconds: 1800 # hard limit when the definition and the call give none
#   max_turns: 0                  # 0 follows agent.max_turns

# Hooks (Go: config.Hooks, internal/config/hooks.go). Your own commands at lifecycle points of a session,
# defined in JSON files of Claude Code's shape; project files need a one-time approval. See docs/features/hooks.md.
# hooks:
#   enabled: true
#   files: ["${FOXXYCODE_HOME}/hooks.json", "${CWD}/.claude/settings.json", "${CWD}/.claude/settings.local.json", "${CWD}/.foxxycode/hooks.json"]
#   project_trust: ask            # ask (approve project files once per workspace) | allow | deny
#   default_timeout_seconds: 60   # per hook process when the definition gives no timeout
#   stop_loop_limit: 5            # Stop-hook continuations per turn
#   max_output_chars: 10000       # cap on what one hook hands to the model or the user

# HTTP OpenAI gateway (only with go build -tags=http). Embedded SPA on / needs -tags=http,ui too. See docs/reference/http-api.md
# httpserver:
#   host: "127.0.0.1"
#   port: 8080

# Cron scheduler (only with go build -tags=scheduler). UTC crontab; flat *.md jobs under scheduler.dir.
# scheduler:
#   enabled: false
#   dir: ""
#   max_queue: 10
#   timeout: "30m"
#   retain_sessions: 5  # max completed run session dirs kept per job_id (default 5)

# Logging (Go: config.Logger, internal/config/logger.go)
logger:
  level: "info"           # debug | info | warn | error
  # Where records go: any combination of stdout, stderr, file. Omitted or empty = stderr only.
  outputs: []
  # Path for the file sink; required when outputs includes file.
  file: ""
  # text (default) or json
  format: "text"
  rotation:
    max_size_mb: 0        # 0 = no size-based rotation
    max_files: 0          # rotated backups to keep when max_size_mb > 0
```

ACP flags override the same knobs when set: **`--log-level`**, **`--log-output`** (stdout, stderr, file, both), **`--log-file`**, **`--log-format`**. Empty flag values keep the YAML (or built-in) defaults.

## Diagnostics

```yaml
# Diagnostics master switch (Go: config.Debug, internal/config/debug.go)
debug:
  # Off by default and free when off. When true: forces the process logger to
  # debug level (overriding logger.level), captures raw LLM HTTP request and
  # response bodies, and writes <session>/debug_trace.jsonl.
  enabled: false
  # Gates only the raw body capture. Omit to follow `enabled`. Set to false to
  # keep debug logs and the trace while suppressing bodies, which carry the whole
  # conversation including the contents of every file the agent read.
  # capture_llm: false
```

**`--debug`** on **`foxxycode acp`**, **`foxxycode http`**, and **`foxxycode gateway`** forces **`enabled: true`** for that process; it only ever turns the layer on, never off. **`foxxycode desktop`** and the console have no flag but honour **`debug.enabled`** from the config. **`PUT /foxxycode/config`** applies the toggle without a restart.

The timeline is readable at **`GET /foxxycode/sessions/{id}/debug`** and streamed live as SSE **`event: debug`**. Full guide: **[docs/operate/debugging.md](../operate/debugging.md)**.

This is **not** the **`debug`** session mode (the diagnose-before-fixing persona selected with **`model: "debug"`**); the two are independent.

If the older two-field style had **`file`** set under **`logger`** but no **`outputs`**, the loader expands to **`stderr`** plus **`file`** so file logging takes effect.

## SSH remote execution

The built-in `ssh_run_command` tool lets the agent run commands on remote hosts over SSH — no external `ssh` binary required (pure-Go via `golang.org/x/crypto/ssh`). The only configurable knob is `tools.ssh_connect_timeout` (TCP dial timeout, default 30 s).

**Authentication order:**
1. **SSH agent** — if `SSH_AUTH_SOCK` is set and reachable, the agent is used first. This covers YubiKeys, 1Password SSH agent, gpg-agent, and standard `ssh-agent` setups.
2. **Key files** — FoxxyCode always looks in the current OS user's `~/.ssh` directory. Key names tried in order: `id_ed25519`, `id_rsa`, `id_ecdsa`, `id_dsa`. Keys protected by a passphrase are silently skipped.

Both sources are active simultaneously — if the agent is available and has keys, files still act as a fallback if the agent declines.

**Host key verification** — derived automatically from `tools.permission_mode`:
- Any mode except `bypass` **(default)** — new hosts are added to `~/.ssh/known_hosts` automatically on first connect (TOFU); if a known host's key has changed, the old entry is replaced with the new one.
- `bypass` — host key verification is disabled (suitable for ephemeral VMs or CI environments).

**Tool schema** — `ssh_run_command` accepts:
| Field | Type | Required | Description |
|---|---|---|---|
| `host` | string | yes | `user@hostname` — user is required |
| `command` | string | yes | Shell command to run on the remote host |
| `port` | integer | no | SSH port (default: 22) |
| `timeout_seconds` | integer | no | Command timeout in seconds |
| `permission_rationale` | string | no | Text shown in the permission dialog |

The tool requires user permission (same as `run_command`) and returns combined stdout + stderr.

## HTTP gateway (optional build)

The **`httpserver`** key (`config.HTTPServerConfig` in `internal/config/http.go`) is ignored unless you use a binary built with **`-tags http`**. It sets default **`host`** and **`port`** when **`foxxycode http`** is still at the built-in flag defaults (`0.0.0.0` and `12345`). See **`docs/reference/http-api.md`**.

### Web UI sign-in (`httpserver.login`)

Off by default. It closes the browser surface of a server that is on a network, so transcripts, tool output and the configuration editor are not readable by whoever finds the port. Write the account with the command rather than by hand:

```bash
foxxycode serve set-password --user pasha
```

```yaml
httpserver:
  host: 0.0.0.0
  auth_token: "${FOXXYCODE_HTTP_TOKEN}"   # unchanged: the credential API clients present
  login:
    enabled: true                      # omit to follow the credentials; false wins over everything
    user: "pasha"
    password_hash: "$$argon2id$$v=19$$..."   # argon2id, written by the command above
    session_ttl_hours: 720                   # 0 = the browser drops the cookie on close (the server still expires its record after 30 days)
```

A hash written into this file by hand needs every `$` doubled (`$$argon2id$$v=19$$...`), because a `$NAME` is expanded as an environment reference when the file loads. The command does that for you; `foxxycode -t` names the problem when it finds a hash that no longer parses. A `${VAR}` reference in `user` or `password_hash` works like every other value here, which also means a save from the settings screen writes the expanded value back into the file - keep a credential out of the document entirely with `FOXXYCODE_HTTP_USER` / `FOXXYCODE_HTTP_PASSWORD` instead.

The account can also come from the environment alone - `FOXXYCODE_HTTP_USER` and `FOXXYCODE_HTTP_PASSWORD`, see the `.env` section below - which is the route for a container or a systemd unit. The form is for browsers; `foxxycode --remote`, `foxxycode acp --remote`, a swarm relay and every script still present the bearer token. On `foxxycode http` - the command the IntelliJ and VS Code plugins and `foxxycode desktop` start on `127.0.0.1` - a direct loopback client (the peer and the `Host` it addressed are both loopback, and no `Forwarded` / `X-Forwarded-*` / `X-Real-IP` header says a proxy relayed it) counts as signed in while no bearer token is configured, so the editor panels and the desktop window never meet the form; `GET /foxxycode/auth/me` answers it `login_required: false`. A client on the network still signs in, and `foxxycode serve` makes no such exception. Full behaviour: [HTTP API](../reference/http-api.md#web-ui-sign-in-optional), [Remote mode](../operate/remote.md#the-sign-in-form).

### MCP project trust

The **`mcp.project_trust`** key decides whether the project-local **`<workspace>/.foxxycode/mcp.json`** may start
its servers: **`ask`** (default) holds them until the operator approves each declaration for that workspace,
**`allow`** starts them automatically, **`deny`** never loads them. Pass **`foxxycode acp --mcp-project-trust <value>`**
or **`foxxycode http --mcp-project-trust <value>`** to override it for one process, which is what CI jobs and
container entrypoints use instead of editing the config file. An unknown value fails the launch.
Full guide in [docs/features/mcp.md](../features/mcp.md).

## Scheduler (optional build)

The **`scheduler`** key (`config.SchedulerConfig` in `internal/config/scheduler.go`) is used only when you build with **`-tags scheduler`**. Set **`scheduler.enabled: true`** in YAML or pass **`foxxycode acp -scheduler-enabled`** / **`foxxycode http -scheduler-enabled`** to set **`scheduler.enabled`** for that process without editing the config file.

Jobs are flat **`*.md`** files under **`scheduler.dir`** (default **`${FOXXYCODE_HOME}/scheduler`** when **`dir`** is empty). Each file has YAML frontmatter with **`description`**, **`schedule`** (five cron fields, **UTC**), optional **`cwd`** (defaults to the directory where **`foxxycode`** was started), **`model`**, **`mode`** (`agent`, `plan`, `docs`, `ask`, or `debug`), optional **`paused`** (when true, cron and manual run are skipped until resume). The markdown body is the one-shot instruction for the sub-agent. Sidecars **`basename.state`** (last fired slot) and **`basename.lock`** (run in progress) sit next to **`basename.md`**.

**`retain_sessions`** (default **5**) caps how many **completed** scheduler-run session directories are kept per **`job_id`** under **`sessions.dir`**; older runs are pruned.

When the scheduler is effectively enabled, **`foxxycode_scheduler_*`** tools cover list or get, create or replace or patch, delete, pause or resume, manual run, cancel, and listing run metadata (**`foxxycode_scheduler_jobs_list`**, **`foxxycode_scheduler_job_get`**, **`foxxycode_scheduler_job_create`**, **`foxxycode_scheduler_job_replace`**, **`foxxycode_scheduler_job_patch`**, **`foxxycode_scheduler_job_delete`**, **`foxxycode_scheduler_job_pause`**, **`foxxycode_scheduler_job_resume`**, **`foxxycode_scheduler_job_run`**, **`foxxycode_scheduler_job_cancel`**, **`foxxycode_scheduler_job_runs`**). With **`-tags=http,scheduler`**, the same operations exist as REST under **`/foxxycode/scheduler`** (see **`docs/reference/http-api.md`**).

## Messenger Gateway (`gateways`)

Requires a binary built with **`-tags gateway.telegram`** (Telegram only) or **`-tags gateway`** (all adapters). The `foxxycode gateway` subcommand reads this block.

```yaml
# Messenger gateways (external/gateway/; build with -tags gateway.telegram or -tags gateway).
# Full guide: docs/surfaces/gateway.md
gateways:
  telegram:
    # Set to true to activate the Telegram adapter when foxxycode gateway starts.
    enabled: false

    # Bot token from @BotFather. Never hard-code; always use an env reference.
    token: "${TELEGRAM_BOT_TOKEN}"

    # Optional outbound proxy for Telegram API requests (http, https, socks5, socks5h).
    # proxy: "socks5h://127.0.0.1:1080"

    # Telegram user IDs with admin privileges.
    # Admins bypass every access check and can always interact with the bot.
    admins: []
    # Example:
    # admins: [98874093]

    # Default access level for chats without a per-chat override.
    #   "all"          - anyone who can write to the chat
    #   "admins"       - only user IDs listed in admins
    #   "group:<name>" - only users in the named user_groups entry (admins always pass)
    default_access: "all"

    # Default session isolation mode for group chats without a per-chat override.
    #   "individual"   - each group member gets their own session
    #   "shared"       - all members share one session
    #   "admin"        - only admins can interact; all admins share one session
    default_isolation: "individual"

    # Named sets of user IDs for group-based access control.
    user_groups: []
    # Example:
    # user_groups:
    #   - name: "devs"
    #     user_ids: [111222333, 444555666]

    # Per-chat overrides. chat_id is negative for groups and supergroups.
    chats: []
    # Example:
    # chats:
    #   - chat_id: -1001234567890
    #     isolation: "individual"
    #     access: "all"
    #   - chat_id: -1009876543210
    #     isolation: "admin"
    #     access: "admins"
```

`token` is validated at startup when `enabled: true`. `proxy` is optional (empty = direct connection). The other fields apply defaults if omitted: `default_access: "all"`, `default_isolation: "individual"`.

See **[docs/surfaces/gateway.md](../surfaces/gateway.md)** for the full configuration guide, running instructions, and how to add adapters for other messengers.

## `.env` file

If **`$FOXXYCODE_HOME/.env`** exists, it is read at startup **before** `config.yaml` is parsed. This lets you keep all secrets in one place without touching shell profiles or Docker compose environment blocks.

```sh
# ~/.foxxycode/.env
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...
TELEGRAM_BOT_TOKEN=8992982910:AAF...
FOXXYCODE_HTTP_TOKEN=a-long-random-token       # bearer credential for API clients
FOXXYCODE_HTTP_USER=pasha                      # web UI sign-in account...
FOXXYCODE_HTTP_PASSWORD=correct-horse-battery-staple   # ...enables the form on its own
```

`FOXXYCODE_HTTP_USER` and `FOXXYCODE_HTTP_PASSWORD` are the one pair that is not referenced from `config.yaml` at all: the password is hashed as the server starts, the file never sees either value, and a save from the settings screen cannot write them into it. They win over an account in the file, and `httpserver.login.enabled: false` switches the form off with them still set.

Then in `config.yaml` reference them as usual:

```yaml
providers:
  - name: openai
    type: openai
    api_key: "${OPENAI_API_KEY}"
```

**Rules:**

- Variables that are **already set** in the process environment are **never overridden** — the process environment always wins. `.env` is a fallback only.
- A missing `.env` is silently ignored (not an error).
- The file is resolved relative to the effective `FOXXYCODE_HOME` (`~/.foxxycode` by default, or the path from `--home` / `FOXXYCODE_HOME` env var).

**Supported syntax:**

| Line form | Example |
|-----------|---------|
| `KEY=value` | `OPENAI_API_KEY=sk-abc` |
| `export KEY=value` | `export DEBUG=true` |
| Double-quoted value | `MSG="hello world"` |
| Single-quoted value | `PATH='no escape \n here'` |
| Escape sequences in `"…"` | `NOTE="line1\nline2"` → real newline |
| Inline comment (unquoted) | `KEY=val # this is ignored` |
| Comment line | `# full-line comment` |

Values already in the process environment (e.g. set by the shell, Docker, systemd) take priority and are never changed by `.env`.

## Environment Variable References

Any config value can reference environment variables using `${VAR_NAME}` syntax.
The agent resolves these at startup.

**Literal `$` in a value:** because expansion runs over the raw file, a value that must contain a
literal dollar sign (e.g. a proxy or API-key secret like `$2y$10$…`) has to double it as `$$` — `$$`
expands back to a single `$`, exactly like docker-compose / envsubst. Without this, fragments such as
`$2y` or `$10` are treated as environment-variable references and resolve to empty strings, silently
corrupting the secret. The Settings UI does this automatically for the `proxy` fields
(`providers[].proxy` and `gateways.telegram.proxy`), which are always treated as literal URLs and do
**not** support `${VAR}` references; for a literal `$` in `api_key` (which does support `${VAR}`),
write `$$` by hand.

Two placeholders are not environment variables:

- **`${FOXXYCODE_HOME}`** - the resolved `FOXXYCODE_HOME` directory, substituted when the file is read.
- **`${CWD}`** - the **session** working directory. It is **not** substituted when the file is read: it stays in the loaded value and whatever uses the path expands it against the session that asks - skill loading, subagent and hook discovery, prompt templates (**`prompts.dir`**), MCP server arguments and URLs. One **`foxxycode http`** process therefore serves many workspaces, and a session rooted in a project sees that project's **`${CWD}/.foxxycode/skills`** (or any entry you write, such as **`${CWD}/.agents/skills`**) regardless of the directory the server was started from. Only the process-scoped locations (**`sessions.dir`**, **`scheduler.dir`**, **`memory.dir`**, **`logger.file`**) expand **`${CWD}`** against the default working directory (**`FOXXYCODE_CWD`**) at load time, since no session owns them.

An environment variable named **`CWD`** does not replace the placeholder (a bare **`$CWD`** without braces is still an ordinary environment reference, as before), and **`GET /foxxycode/config`**, the Settings UI, and **`config_get`** report the entry exactly as written. The placeholder is honoured only in the fields listed above; in any other string value it stays as written (prompt templates use **`{{.CWD}}`** instead).

## Model Provider Reference

Provider **`type`** values match **`internal/llm.NewProvider`**: **`openai`**, **`anthropic`**, **`neuraldeep`**, **`codex`**.

YAML split:

- **`providers`**: **`name`** (unique), **`type`**, **`api_key`**, optional **`api_base`** (for `neuraldeep` it selects one of the two official deployments; ignored for the fixed-endpoint `codex` provider), optional **`proxy`**. Codex credentials are managed out of band through the UI or `foxxycode codex`.
- **`models`**: **`model`** (string **`provider_name/api_model_id`**, session selector and **`agent.model`** value), **`max_tokens`**, **`temperature`**, optional **`max_context_tokens`**, optional **`multimodal`**, optional **`reasoning_levels`** (omitted: auto-detected from the API model id — **`gpt-5*`** → **`minimal,low,medium,high`**; OpenAI **`o`**-series, **`gpt-oss*`**, **`qwen3*`** (qwen3, qwen3.5, qwen3.6, ...) and Claude extended-thinking models → **`low,medium,high`**), and optional **`reasoning_default`**. For **`qwen3*`** models on OpenAI-compatible providers a selected level also carries **`chat_template_kwargs`** **`{"enable_thinking": true}`**, because Qwen thinking is a chat-template switch rather than an effort tier. Codex does not receive `max_tokens`; it maps `minimal` to `none` and requests reasoning summaries plus encrypted reasoning replay across tool calls.

### `openai`
Standard OpenAI API. Supports: `gpt-4o`, `gpt-4o-mini`, `gpt-4-turbo`, `o1`, `o3-mini`, etc.

Provider needs **`api_key`**. Optional **`proxy`** applies only to this provider row (HTTP, HTTPS, SOCKS5, or SOCKS5h). The **`models[].model`** string must start with this provider **`name`** and a slash, then the OpenAI API model id, for example **`openai/gpt-4o`**. Also set **`max_tokens`**, **`temperature`**.

### `anthropic`
Anthropic API. Supports: `claude-3-5-sonnet-*`, `claude-3-5-haiku-*`, `claude-3-opus-*`

Provider needs **`api_key`**. Optional **`api_base`** overrides the Anthropic API base URL (default **`https://api.anthropic.com`**), for example an Anthropic-compatible gateway or relay. Optional **`proxy`** applies only to this provider row. Use **`models[].model`** like **`anthropic/claude-3-5-sonnet-20241022`**, plus **`max_tokens`**, **`temperature`**.

### `neuraldeep`
NeuralDeep hub (**`https://hub.neuraldeep.ru`**). It speaks the OpenAI wire protocol, so requests are handled by the OpenAI client. The same API is served from two deployments: **`https://api.neuraldeep.ru/v1`** for Russia and **`https://api.neuraldeep.tech/v1`** for everywhere else. **`api_base`** selects one - leave it empty for the first, and any value that is not one of the two falls back to it (a startup warning says so). The choice travels with the credential: sign-in goes to **`hub.neuraldeep.ru`** or **`hub.neuraldeep.tech`** to match, so pick the endpoint before signing in (**`foxxycode providers login neuraldeep --api-base https://api.neuraldeep.tech/v1`**, or the endpoint dropdown in Settings). A login with **`--api-base`** also moves an existing provider row to that endpoint (unless **`--no-config`**), so the row and the key agree; in Settings the sign-in follows the dropdown as picked in the form, before Save. A key minted by one hub is not honored by the other; FoxxyCode warns at startup when the stored login and the selected endpoint disagree, and the Settings row shows the same warning live. **`FOXXYCODE_NEURALDEEP_BASE_URL`** and **`FOXXYCODE_NEURALDEEP_HUB_URL`** still redirect the whole process for stands and tests, and they win over the config. Provider needs only **`api_key`** — a literal key, a **`"${NEURALDEEP_API_KEY}"`** reference, or empty to read **`NEURALDEEP_API_KEY`** at call time when the provider is named **`neuraldeep`**. Optional **`proxy`** applies only to this provider row. Use **`models[].model`** like **`neuraldeep/gpt-oss-120b`**, plus **`max_tokens`**, **`temperature`**.

### Local OpenAI-compatible servers (Ollama, llama.cpp, LM Studio)
Use **`type: openai`** and set **`api_base`** to an OpenAI-compatible base URL that already includes **`/v1`**, for example **`http://localhost:11434/v1`** for Ollama.
