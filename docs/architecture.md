# Architecture: FoxxyCode Agent

## Overview

FoxxyCode is a **distroless-friendly ACP harness** written in Go. At its core it is protocol plumbing
(STDIO JSON-RPC server, sessions, configuration, MCP wiring) plus a **ReAct** execution loop backed
by pluggable LLM providers. Ship it as one binary suitable for scratch or distroless images,
sidecars, CI sandboxes, or local installs.

The default toolset and prompts are tuned so the harness presents as an **interactive coding agent**
(ACP clients spawn `foxxycode acp`; users get filesystem, commands, MCP, project rules from `.foxxycode`/`.cursor`/`.claude`/`.codex` rule trees plus nested `AGENTS.md` files under session cwd, and skills from `skills.dirs`).
That coding-agent surface is **a productized profile on top of the harness**, not the only way to run FoxxyCode.

## High-Level Architecture

```
┌──────────────────────────┐   ┌──────────────────────────────────┐
│   ACP client (editor)    │   │  Messenger (Telegram, …)         │
│      Zed / scripts       │   │  (build tag: gateway.telegram    │
└────────────┬─────────────┘   │             or gateway)          │
             │ JSON-RPC 2.0    └─────────────┬────────────────────┘
             │ over stdio                    │ long-polling
             ▼                               ▼
┌────────────────────────┐    ┌────────────────────────────────────┐
│   ACP Server Layer     │    │  Gateway Hub (external/gateway/)   │
│  initialize            │    │  one goroutine per adapter         │
│  session/new           │    │  auto-restart on error             │
│  session/prompt        │    └──────────────┬─────────────────────┘
│  session/cancel        │                   │
└────────────┬───────────┘                   │
             │                               │
             └──────────────┬────────────────┘
                            │
                            ▼
            ┌───────────────────────────────┐
            │        Session Manager        │
            │  per-session state, mode,     │
            │  history, rules, skills       │
            └───────────────┬───────────────┘
                            │
                            ▼
            ┌───────────────────────────────┐
            │      ReAct Agent Loop         │
            │  [THINK] → [ACT] → [OBSERVE]  │
            │  → loop or [ANSWER]           │
            └──────┬──────────┬─────────────┘
                   │          │
        ┌──────────┘    ┌─────┴──────┐   ┌─────────────┐
        ▼               ▼            ▼   ▼             │
   ┌─────────┐    ┌──────────┐ ┌──────────────┐        │
   │   LLM   │    │  Tools   │ │  MCP Clients │        │
   │Provider │    │Registry  │ │  (external)  │        │
   └─────────┘    └──────────┘ └──────────────┘        │
                                               ┌───────┘
                                               │
                                    ┌──────────▼────────────┐
                                    │  optional external/   │
                                    │  memory  scheduler    │
                                    └───────────────────────┘
```

## Component Descriptions

### ACP Server Layer (`internal/acp`)

Implements the JSON-RPC 2.0 server that speaks the ACP protocol over stdio.
Handles:
- `initialize` - version negotiation, capability exchange
- `session/new` - create session, connect MCP servers, return modes and Session Config Options (model + mode selectors)
- `session/load` - restore a persisted bundle from disk (**`$FOXXYCODE_HOME/sessions`** by default, usually **`~/.foxxycode/sessions`**), replay history via `session/update`
- `session/list` - enumerate persisted sessions (ACP `sessionCapabilities.list`)
- `session/prompt` - receive user message, start ReAct loop
- `session/cancel` - cancel in-progress turn
- `session/set_mode` - switch between `agent`, `plan`, `docs`, and `ask` modes (legacy, kept in sync with config options)
- `session/set_config_option` - change mode or model for the session (preferred ACP API)

### Session Manager (`internal/session`)

Maintains the state for each conversation session:
- Conversation history (messages, tool results)
- Current operating mode (`agent` / `plan` / `docs` / `ask`)
- Optional model override per session (when the user selects a model via ACP)
- Connected MCP server clients
- Working directory
- Active context (skills + project rules in separate prompt sections)
- In-memory plan entries for todo tools (**`session.Plan`**), mirrored to **`todos/active.md`** when persistence is enabled (**`filesystem.go`**)

### ReAct Agent Loop (`internal/agent`)

The core reasoning engine (**`react.go`**):

1. Loads tool definitions from **`internal/tooling.Registry.AllToolDefinitions`** and applies the session **`ToolSet`** from **`internal/agent/toolsets.go`** (empty set means no registry filtering). MCP tool definitions from connected servers are appended in **`agent`** and **`plan`**. Ask receives only MCP tools annotated with **`readOnlyHint: true`**, unless **`tools.ask_disable_extended_tools`** is enabled. Docs has a closed tool surface with no MCP.
2. Builds the system prompt from **`internal/prompts.Render`**. The built-in defaults are assembled from reusable section fragments under **`internal/prompts/sections/`** (one ordered manifest per mode and provider family; see **`sections.go`**), so shared blocks like the agent body, the conditional footer, and the read/search and background guidance stay in one place instead of being forked per family. Custom files under **`prompts.dir`** keep the legacy one-file-per-mode shape and bypass section assembly. Configurable names **`prompts.agent_prompt`**, **`prompts.plan_prompt`**, and **`prompts.docs_prompt`** default to **`agent.md`**, **`plan.md`**, and **`docs.md`**; Ask uses **`ask.md`**. Model-specific and family-specific built-ins resolve to a notes fragment spliced into the mode manifest (for example the **`openai`** family adds **`agent/notes_openai`**; ask ships **`openai`** and **`gpt-oss`** alternate manifests). Template data includes **`CWD`**, tools markdown, skills markdown, rules markdown (**`{{.Rules}}`** via **`internal/rules`**), mode-specific plan/todo context, optional **`Memory`**, and **`UTCNow`** (RFC3339 UTC refreshed on every render). FoxxyCode then appends an **`<environment_context>`** block containing **`<os>`**, **`<arch>`**, and the detected **`<shell>`**, even when a custom prompt template is used.
3. Prepends that system message to the session message list and appends the newest user turn.
4. **Before every LLM invocation** inside one **`session/prompt`**, refreshes the **`system` message content** so **`TodoList`** and other template fields match state after prior tool calls in the same episode.
5. Streams the LLM response, executes tool calls, appends assistant and tool messages.
6. Loops until there are no tool calls, **`max_turns`** is exceeded, the loop guard stops a runaway turn, or cancellation.
6a. Loop guard (**`agent.loop_guard`**, on by default, **`internal/agent/loopguard.go`**): a streamed channel that degenerates into repeating the same passage has its stream cancelled and the repeated run stripped from the stored message, and a tool call repeated with identical canonical arguments stops being executed. The model is nudged to change course up to **`agent.loop_nudge_max`** times, after which the turn ends with **`StopReasonRefused`** and a UI notice.
7. On **`session/cancel`** (or HTTP **`POST /foxxycode/sessions/{id}/cancel`**) while the LLM stream is active, stream providers return **`context.Canceled`** together with any **`Response`** body accumulated so far; **`react.go`** appends that assistant **`content`** to session history when non-empty, then ends the turn with **`StopReasonCancelled`**. **`GET /foxxycode/sessions/{id}/messages`** can briefly trail that append until the filesystem bundle is read again.

### Prompt templates (`internal/prompts`)

Built-in system prompts are assembled from reusable Markdown section fragments, with no monolithic per-mode prompt files. The assembler does not enumerate variants; it resolves keys supplied by the caller through file conventions. Tuning an already-classified provider/model family is therefore a drop-in file change. Classifying a genuinely new family still requires updating **`family.go`**.

**Layout and naming conventions** (paths are relative to **`internal/prompts/sections/`**):

| File | Purpose |
|------|---------|
| **`<mode>/manifest`** | Base structure: section IDs, one per line, in render order. |
| **`<mode>/manifest.<variant>`** | Optional per-variant structure override (replaces the base ID list). |
| **`<mode>/<id>.md`** | Shared fragment for section **`<id>`**. |
| **`<mode>/<id>_<variant>.md`** | Optional variant-specific fragment override. |

**`<mode>`** is one of the four fixed session modes (**`agent`**, **`plan`**, **`docs`**, **`ask`**). **`<variant>`** is any provider family or per-model slug resolved by the caller (for example **`anthropic`**, **`openai`**, **`gpt-oss`**, or a model slug). The assembler does not enumerate variant names; family detection remains in **`family.go`**.

**Resolution** for a render with variants most-specific-first (for example `[model-reference-slug, API-model-slug, family]`):

1. **Structure** — the first variant that has a **`<mode>/manifest.<variant>`** file supplies the section ID list; otherwise **`<mode>/manifest`** (the base) is used. Unknown modes fall back to the **`agent`** manifest.
2. **Content** — for each section ID, the first existing fragment wins, trying **`<mode>/<id>_<variant>.md`** across variants in order, then the shared **`<mode>/<id>.md`**. A section ID that resolves to no file is silently skipped, so the base manifest can list an optional **`notes`** slot that is only rendered when a variant supplies **`notes_<variant>`** (provider guidance spliced in right after the header).

**Tuning a classified provider family** (fragment-only change):

- *Agent* (same body, family-specific guidance): drop **`sections/agent/notes_<family>.md`**. The base manifest already lists the **`notes`** slot.
- *Plan* (override some sections): drop **`sections/plan/<id>_<family>.md`** for each section you want to override (for example **`howto_<family>`**), plus optional **`notes_<family>`**. The base plan manifest is reused.
- *Ask* (restructure the body): drop **`sections/ask/manifest.<family>`** with the reordered/new section IDs, then the **`<id>_<family>.md`** fragments for each ID.
- *Per-model* overrides work the same way. The configured model-reference slug resolves first, followed by the resolved API-model slug and then the family. This lets a provider-neutral file such as **`model_notes_gpt-oss-20b.md`** work for both **`ollama/gpt-oss-20b`** and **`neuraldeep/gpt-oss-20b`**. A **`<id>_<model-slug>.md`** or **`manifest.<model-slug>`** fragment overrides less-specific content for that model.

The built-in gpt-oss prompts use a shared Harmony-aware family fragment plus separate **`gpt-oss-20b`** and **`gpt-oss-120b`** profiles in all four modes. The 20B profiles favor short, explicit, independently verifiable steps; the 120B profiles use broader cross-file synthesis while keeping visible output concise.

The shared footer fragment (tools, skills, rules, instructions, memory, UTC) and the agent body sections are single-sourced this way, which is what prevents the per-family drift that previously dropped the read/search and background guidance from every agent family fork.

Each fragment may use Go **`text/template`** with the **`TemplateData`** fields documented in **`loader.go`** (**`{{.CWD}}`**, **`{{.Tools}}`**, **`{{.Skills}}`**, **`{{.Rules}}`**, **`{{.Memory}}`**, **`{{.TodoList}}`**, **`{{.PlanContext}}`**, **`{{.DiscardedPlans}}`**, **`{{.Instructions}}`**, **`{{.UTCNow}}`**); use **`{{if .X}}...{{end}}`** for sections that must be omitted when empty. Custom on-disk prompts configured under YAML **`prompts.dir`** keep the legacy one-file-per-mode shape (default file names **`agent.md`** / **`plan.md`** / **`docs.md`** / **`ask.md`**, overridable via **`prompts.agent_prompt`** / **`plan_prompt`** / **`docs_prompt`**) and bypass section assembly entirely.

### LLM Provider (`internal/llm`)

Abstracted interface for LLM backends. Configured via `config.yaml`.
Supported backends (see **`docs/config.md`** for shapes):
- OpenAI and OpenAI-compatible HTTP APIs (**`type: openai`**)
- Anthropic (**`type: anthropic`**)
- NeuralDeep hub (**`type: neuraldeep`**; OpenAI-compatible, fixed endpoint)
- Ollama and other local OpenAI-compatible stacks (**`api_base`**)

### Tools Registry (`internal/tools`)

The **tool types and registry mechanics** live in **`internal/tooling`** (`Tool`, `Env`,
`Registry`, JSON `ParseArgs`, **`AllToolDefinitions`**). The **`internal/tools`** package is the
composition root (`NewRegistry` wires everything) and exposes the same APIs via type aliases so
call sites such as **`internal/agent`** keep importing **`tools`** only.

- **`internal/tools/web`** - **`websearch`** (DuckDuckGo text search) and **`webfetch`** (fetch public `http(s)` pages, readability + Markdown; SSRF guards)

Built-in implementations are grouped in subfolders under **`internal/tools/`**:

- **`internal/tools/fs`** - path helpers (`paths.go` with `ResolvePath`, `CheckInsideCWD`,
  `PathEscapesCWD`, `ToolPathsEscapeCWD`) and tools (`read.go` **`read`**, **`glob.go`** **`glob`**,
  **`grep.go`** **`grep`**, **`print_tree.go`** **`print_tree`** (directory tree, read-only), **`write.go`** **`write`**, **`edit.go`** **`edit`**, **`patch.go`**
  **`apply_patch`**, **`mkdir`**, **`rmdir`**, **`touch`**, **`rm`**, **`mv`**).
  **`grep`** uses a system **`rg`** when one is available (the pattern is passed to it untouched)
  and otherwise falls back to the built-in Go walker/matcher in **`search.go`**. **`glob`** uses
  the same built-in walker when **`rg`** is unavailable, so filesystem discovery also works in
  Windows and distroless binaries without sidecar executables.
  **Fork-specific:** a **non-ASCII pattern always uses the built-in engine**, which decodes each
  file through **`decodeText`** (**`text_encoding.go`**) first, so Cyrillic searches match
  **Windows-1251** sources as well as UTF-8 ones. System **`rg`** matches raw bytes and would
  silently miss them. ASCII patterns still go to **`rg`** (faster, honors **`.gitignore`**);
  files above **`maxDecodedSearchFileBytes`** (8 MiB) stream as UTF-8 without the decode step.
  Note that **`rg`** also cannot answer an ASCII pattern against **UTF-16**, whose ASCII
  characters are not single bytes on disk; such files are reached through the built-in engine.
- **`internal/textenc`** - the one place a file's encoding is decided, shared by prompt
  attachments (**`internal/session/promptfiles.go`**) and every file tool. Tiers: byte-order
  marks (**UTF-8/16/32**), a UTF-8 fast path, a binary guard, **chardet** detection run twice
  (whole input, then only the lines carrying non-ASCII bytes, so a mostly-ASCII source with
  Russian comments is not dragged onto ISO-8859-1), and finally the system **ANSI code page**
  (**`platform.DecodeANSI`**, Windows only). **`Decode`** also reports the **`Encoding`** so
  **`write`** / **`edit`** / **`apply_patch`** put a file back byte for byte, mark included.
  **Fork-specific:** **`internal/tools/fs`** adds one last rung after all of those,
  **Windows-1251**, so a legacy file the detector cannot name still reads as text - which is
  what this layer always did unconditionally. Prompt attachments do **not** get that rung: an
  undecodable attachment is refused with **`ErrNotDecodableText`** rather than inlined as noise.
- **`internal/platform`** - shared host shell detection: **`pwsh` → `powershell` → `cmd`** on Windows and **`bash` → `sh`** elsewhere; also renders the prompt environment context.
  It also starts every child process windowless on Windows (**`HideConsoleWindow`**,
  **`hidewindow_windows.go`**). The desktop shell links with **`-H=windowsgui`** and owns no console, so
  Windows hands each console child one of its own - with a visible window and a taskbar button. A turn
  starts those in bursts (git for the workspace chips, ripgrep behind a search tool, the shell behind
  **`run_command`**, an MCP stdio server), which is why a row of windows used to blink open and shut on
  the desktop. **`CREATE_NO_WINDOW`** keeps the console and drops the window, so the code page
  **`DecodeOutput`** reads is unchanged; it is a deliberate no-op when this process does own a console,
  where the child inherits the terminal the operator is watching. Every spawn site calls it, and
  **`hidewindow_guard_test.go`** walks the tree to fail a new **`exec.Command`** that forgets to.
- **`internal/tools/shell`** - **`run_command`**, bound to the shared detected shell and documented to the model with platform-appropriate command examples.
  A **foreground** command that outlives its timeout is **not** killed: **`foreground.go`** starts it in a detached process group with its own **`cmd.Wait`**, and at the deadline **`bgtask.Pool.Adopt`** takes it over, **`switchwriter.go`** redirects its output into the task sink with everything captured so far flushed in first, and the tool answers with the task id followed by that output. Killing was wrong twice over: a dev server is doing exactly what was asked, and a grandchild holding the output pipe kept **`cmd.Wait`** from ever returning. The result is **`(string, nil)`**, not an error, because the agent loop discards the result string when a tool errors - which is what used to throw the captured output away.
- **`internal/tools/svn`** - Subversion working copy tools (**`svn_info`**, **`svn_status`**, **`svn_diff`**,
  **`svn_log`**, **`svn_list`**, **`svn_add`**, **`svn_revert`**, **`svn_resolve`**, **`svn_update`**,
  **`svn_commit`**, **`svn_switch`**, **`svn_merge`**, **`svn_checkout`**) over **`internal/svnws`**.
  Registered only when **`vcs.svn.enabled`** is on (default) **and** an svn client is installed; the
  registry is rebuilt every prompt turn, so unchecking the setting removes them without a restart.
  Mutating tools require permission; detection is independent of git, so a branch folder that also
  holds a git repository works with both.
- **`internal/tools/todo`** - todo/plan list (**`foxxycode_todo_plan_read`**, **`foxxycode_todo_plan_replace`**,
  **`foxxycode_todo_plan_archive`**, **`foxxycode_todo_item_add`**, **`foxxycode_todo_item_remove`**,
  **`foxxycode_todo_item_update`**, **`foxxycode_todo_item_move`**)

**Tool exposure** - **`internal/agent/toolsets.go`** defines a **`ToolSet`** name allowlist per mode. An **empty** `ToolSet` means no registry filtering. **Plan** and **Docs** use fixed registry allowlists; **`ModeAllowsMCPTools`** separately limits MCP exposure to Agent and Plan.

Agents see:

- **`agent`** mode - every built-in registered by **`internal/tools.NewRegistryFor`** (filesystem, shell, todo, optional scheduler tools, **`websearch`**, **`webfetch`**, **`question`**, **`plan_exit`**, etc.) plus MCP tools from connected servers.
- **`plan`** mode - **`read`**, **`glob`**, **`grep`**, **`print_tree`**, **`websearch`**, **`webfetch`**, **`run_command`**, **`question`**, **`plan_write`**, **`plan_list`**, **`plan_read`**, and the read-only **`svn_info`** / **`svn_status`** / **`svn_diff`** / **`svn_log`** / **`svn_list`**, plus MCP tools. General workspace writes, todo tools, scheduler tools, and memory tools are not advertised to the LLM.
- **`docs`** mode - **`read`**, **`glob`**, **`grep`**, **`websearch`**, **`webfetch`**, **`question`**, **`docs_write`**, and **`docs_edit`**. It receives neither **`run_command`** nor MCP tools, so its only built-in mutations are the guarded Markdown writers.

The Docs writers accept only **`.md`** paths inside the session CWD, reject paths that escape after resolving symlinks, and protect **`internal/prompts/`**. **`docs_write`** requires **`overwrite: true`** before replacing an existing file; **`docs_edit`** requires a non-empty exact **`oldString`** that is unique unless **`replaceAll`** is set. The Docs prompt also treats review-only requests as non-mutating and requires an explicit user request before changing documentation.

`run_command`, optional write paths, out-of-tree paths, and interactive **`question`** flows still coordinate with the client (**`session/request_permission`** for destructive paths; HTTP streaming uses **`event: question`** plus **`POST /foxxycode/sessions/{id}/question`**).

### Messenger Gateway (`external/gateway`)

The gateway is a separate process entry point (`foxxycode gateway`) that lets messenger bots (Telegram today, others via the same interface) drive the same session manager and ReAct loop used by `foxxycode acp` and `foxxycode http`.

Compiled only when built with **`-tags gateway.telegram`** (Telegram) or **`-tags gateway`** (all adapters). Without these tags the `foxxycode gateway` subcommand is present but returns a "not compiled" error.

**Key packages:**

| Package | Role |
|---------|------|
| `external/gateway` | `Adapter` interface, `Hub`, `Start()` entry point |
| `external/gateway/access` | Access control: `CanAccess`, `EffectiveAccess`, `EffectiveIsolation` |
| `external/gateway/sessionstore` | `Store`: maps stable chat/user keys to FoxxyCode session IDs; `Reset` on `/clear` |
| `external/gateway/telegram` | `Bot` (polling, trigger rules, ACL), `Sender` (implements `acp.UpdateSender`) |

**Data flow for one incoming message:**

1. Adapter receives raw update, normalises it to `IncomingMessage`.
2. `access.CanAccess` rejects the message if the user fails the configured access level.
3. `sessionstore.SessionKey` derives a deterministic string key from gateway name, chat ID, user ID, and isolation mode.
4. `store.Get(key)` returns the current FoxxyCode session ID for that key (creating one on first use).
5. `manager.EnsureHTTPSession` loads or creates the session bundle.
6. `manager.HandleSessionPromptWithSender` runs the ReAct loop with the adapter's `Sender`.
7. `sender.Flush()` sends accumulated text back to the chat.

**Extending with a new adapter** — implement `gateway.Adapter`, add a `Sender` that satisfies `acp.UpdateSender`, tag files with `//go:build gateway || gateway.<name>`, append to `Start()`. See [`docs/gateway.md`](gateway.md) for the full walkthrough.

### Optional `external` tool packages (scheduler, memory)

Some features live under **`external/`** and define tools that are **not** registered through **`internal/tools.NewRegistry`**, but still use the same **`internal/tooling.Tool`** shape as the core harness.

**Contract (mirror `external/scheduler/tools/job_get.go`):**

1. **One tool per file** - a package-local constructor returns **`*tooling.Tool`** with **`Definition`** (name, description, **`InputSchema`**) and **`Execute`** in one place. **`Execute`** takes **`context.Context`**, JSON args as a string, and **`*tooling.Env`** (use **`CWD`** or other fields when the tool needs session context; pass **`&tooling.Env{}`** when unused).
2. **JSON schema maps** - prefer **`map[string]interface{}`** for **`InputSchema`** and **`[]interface{}`** for **`required`** and enum lists so OpenAI and Anthropic marshaling stay consistent with existing scheduler tools.
3. **`register.go`** - collects constructors. **`external/scheduler/tools`** exposes **`RegisterTools`** for the main agent registry. **`external/memory/tools`** exposes **`PersistTools`**, **`RecallTools`**, **`ToolDefinitions`**, and **`Exec`** because the memory copilot runs a separate LLM loop in **`external/memory/copilot.go`**.
4. **Naming** - scheduler files use the **`job_*.go`** prefix; memory tool bodies use the **`mem_*.go`** prefix; **`external/memory/tools`** keeps **`env.go`**, **`names.go`**, **`register.go`** without the **`mem_`** prefix.

### MCP Client (`internal/mcp`)

Connects to external MCP servers from three config levels (`config.yaml`
`mcp_servers`, the global `~/.foxxycode/mcp.json`, the project `./.foxxycode/mcp.json`;
later levels override by name) plus servers specified in `session/new`.
Transports (dispatched by `mcp.Connect` over a shared `transport` interface):
- stdio - local subprocess, newline-delimited JSON-RPC; the process lifetime is
  transport-owned (the connect ctx only bounds the handshake)
- streamable HTTP (`type: http`) - JSON-RPC POSTs answered as JSON or SSE
  chunks, `Mcp-Session-Id` round-trip, automatic legacy-SSE fallback
  (capability: `mcpCapabilities.http`)
- legacy HTTP+SSE (`type: sse`) - GET event stream announcing the POST
  endpoint (capability: `mcpCapabilities.sse`)

`mcp.Probe` backs the `/foxxycode/mcp` management API (connect, `tools/list`,
close); `manage.go` resolves which file owns a server for enable/disable
persistence. Tools from MCP servers are appended to the LLM tool list in
**`agent`** and **`plan`** modes (see **`internal/agent/react.go`**), filtered
per turn by the disable switches. Ask receives only tools explicitly annotated
with **`readOnlyHint: true`** and only while its extended-tool setting is off.

### Skills loader (`internal/skills`)

Loads `SKILL.md` from configured `skills.dirs` (see `docs/skills.md`). Default dirs (lowest → highest priority): **`~/.agents/skills`** (global, shared with `npx skills`/`npx skillsbd`), **`~/.foxxycode/skills`** (foxxycode-specific), **`${CWD}/.foxxycode/skills`** (project-local). Later dirs override earlier ones when the same skill name appears in multiple locations. Bundled **`/generate-rules`** is always prepended.

### Rules engine (`internal/rules`)

Discovers `.mdc` / `.md` rules from `.foxxycode/rules`, `.cursor/rules`, `.claude/rules`, `.codex/rules`, plus nested `**/AGENTS.md` files ([agents.md](https://agents.md/) convention), under session CWD. Injected into **`{{.Rules}}`** separately from skills; see **`docs/rules.md`**.

Activation uses globs, **`alwaysApply`**, **`@mention`**, and sticky auto rules (see **`docs/rules.md`**).

### IntelliJ IDEA project context (`internal/session`)

When the session CWD contains `.idea/`, readable UTF-8 files below that directory are added recursively to every ReAct model request as project metadata, including module and required-plugin declarations. The files are explicitly marked as data rather than instructions, binary and unreadable files are skipped, individual files are capped at 256 KB, and the combined metadata body is capped at 512 KB.

### Config (`internal/config`)

YAML-based configuration. Resolution uses **`FOXXYCODE_HOME`** (default **`~/.foxxycode`**), **`FOXXYCODE_CWD`**, **`FOXXYCODE_CONFIG`**, optional **`config.yaml`** in the process working directory when **`$FOXXYCODE_HOME/config.yaml`** is absent, and CLI flags (see **`docs/config.md`** and **`README.md`**).

## Session Modes

### `agent` mode (default)
- Full tool access (read, write, run commands)
- Executes tasks end-to-end
- Requests permission before destructive operations
- Suitable for: code generation, refactoring, debugging

### `plan` mode
- Narrow **registry** tool surface enforced by **`internal/agent.ToolSetForMode("plan")`**
- **`read`**, **`glob`**, **`grep`**, **`print_tree`**, **`websearch`**, **`webfetch`**, **`run_command`**, **`question`**, **`plan_write`**, **`plan_list`**, **`plan_read`**, the read-only **`svn_*`** inspection tools, plus any **MCP** tools from configured servers
- No built-in workspace writes or **foxxycode** todo tools in the advertised set (switch to **agent** for those)
- Suitable for: design docs, specs, architecture planning, external research, and light shell or MCP inspection without offering full mutating builtins

### `docs` mode
- Closed documentation-maintenance surface enforced by **`internal/agent.ToolSetForMode("docs")`**
- Read/search/web/question tools plus guarded **`docs_write`** and **`docs_edit`** Markdown writers
- No shell, MCP, general filesystem mutators, plan tools, todo tools, scheduler tools, or memory tools
- Suitable for: evidence-based documentation reviews and explicit Markdown documentation updates without code changes

### `ask` mode
- Read-only question-answering surface enforced by **`internal/agent.ToolSetForMode("ask")`** and execution-time guards
- Basic tools: repository read/search/tree, interactive questions, and skills
- By default, also exposes web search/fetch, read-only scheduler inspection, MCP tools whose server declares **`readOnlyHint: true`**, and a guarded shell command allowlist
- Shell syntax that can chain commands, redirect output, perform substitution, or invoke a non-read command is refused before execution
- **`tools.ask_disable_extended_tools: true`** hides shell, MCP, web, and scheduler tools while keeping the basic read-only set
- No file/document writers, plan/todo mutators, scheduler mutations, SSH, browser automation, or memory mutations
- Suitable for: repository-grounded explanations, reviews, investigation, and user questions without changing project state

Mode switching:
- Client calls `session/set_config_option` with `configId` `mode` (preferred) or `session/set_mode` with `agent`, `plan`, `docs`, or `ask`
- Agent sends `current_mode_update` and `config_option_update` when mode changes

## Directory Structure

Top level after **`git clone`** (folder name is arbitrary; **`foxxycode-agent`** is common):

```
.
├── cmd/foxxycode/                   # CLI entry (acp, http, sessions, skills)
├── internal/                    # core harness (acp, session, agent, config, tools, …)
├── external/
│   ├── memory/                  # long-term memory copilot (`-tags memory`)
│   ├── httpserver/              # optional REST gateway (build tag http)
│   ├── ui/                      # Vite SPA sources (embedded when built with http+ui)
│   ├── scheduler/               # optional cron runner (build tag scheduler)
│   └── gateway/                 # messenger gateway (build tag gateway | gateway.telegram)
│       ├── access/              # ACL: CanAccess, EffectiveIsolation
│       ├── sessionstore/        # chat/user → session ID mapping
│       └── telegram/            # Telegram bot adapter (tgbotapi v5)
├── examples/                    # ACP and HTTP Python harnesses
├── docs/                        # guides (see docs/README.md)
├── Dockerfile
├── docker-compose.yml
├── docker-compose.dev.yml
├── config.example.yaml
├── go.mod
├── go.sum
└── README.md
```

Optional layers **`external/httpserver`**, **`external/ui`**, **`external/scheduler`**, and **`external/memory`** are omitted from the binary unless you pass the matching **Go build tags**; see **`docs/build.md`** and **`README.md`**. Long-term memory runtime behavior is toggled with **`memory.enabled`** when the binary was built with **`memory`**.
