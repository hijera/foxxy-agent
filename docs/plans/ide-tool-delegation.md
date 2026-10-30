# Feasibility: executing agent tools through the IDE (IntelliJ / VS Code)

Status: design record, research only - written 2026-10-30 on branch
`worktree-ide-tool-delegation`. Nothing in it is implemented; the branch carries
this file alone. It answers the question: could the editor plugins connect the
**`foxxycode`** binary and **replace its tool implementations**, so a tool
execution runs through the IDE (edits through the editor's own diffs and
documents, shell through the IDE terminal) instead of the binary writing files
and spawning processes itself?

## 1. Verdict

**Yes in theory**, at two levels of ambition. Both have a concrete hook point in
the code that exists today, and both need a channel the code does not have yet:
a **request/response RPC that returns a tool result from the IDE**.

- **Route A - delegate RPC in the tool dispatch** (true substitution for the
  plugin surfaces). The dispatch block in `internal/agent/react.go:2457-2476`
  already branches on the tool name - MCP tools go one way, the registry the
  other. A third branch ("this call is delegated to the connected IDE") keeps
  the whole surrounding machinery intact: PreToolUse/PostToolUse hooks, the
  permission gate, argument and result persistence, output caps, transcript
  rows. Medium-sized feature: one new RPC plus a plugin-side executor per tool.
- **Route B - the standard ACP client-fs path** (works for any ACP client, and
  is already half-wired). ACP defines exactly this: the client declares
  `fs.readTextFile` / `fs.writeTextFile` / `terminal` capabilities and the agent
  calls back into it (`docs/reference/acp-protocol.md:742-769`). FoxxyCode has
  the typed call `acp.Server.CallClientFS` (`internal/acp/server.go:419-468`)
  and the capability fields (`internal/acp/types.go:69-78`) - but **nothing
  calls it**; every built-in tool touches the disk itself. Wiring the fs tools
  through it when the capability is present is the lowest-friction first step.
  The catch: our own IntelliJ/VS Code plugins do not speak ACP (they use the
  HTTP bridge, `docs/surfaces/editors.md:74-76`), so this route pays off first
  for Zed and other ACP clients, or after the plugins grow an ACP client mode.

What does **not** work as substitution: **MCP** (route C). An MCP server can add
tools and the dispatch already routes `server__tool` names to
`mcp.Client.CallTool` (`internal/agent/react.go:2463-2473`), but the built-in
tool set stays as it is - at best the built-ins could be hidden and same-named
MCP tools offered instead, which is prompt-level imitation, not a swap of the
executors.

## 2. What exists today

### 2.1 Tool pipeline

- A tool is a plain struct: `Definition llm.ToolDefinition` plus
  `Execute func(ctx, argsJSON string, env *tooling.Env) (string, error)`
  (`internal/tooling/tool.go:10-19`). Implementations are already **swappable
  data** - a registry entry can be replaced at composition time
  (`internal/tools/export.go:34-108`).
- One dispatch point: `executeToolCall` (`internal/agent/react.go:2251-2509`),
  which persists args, runs PreToolUse hooks, raises the permission prompt, then
  executes (`:2457-2476`) and finishes the call into the transcript. This is
  the choke point a delegate belongs at.
- The permission gate runs **before** execution and is already a blocking
  client round-trip (`acp.UpdateSender.RequestPermission`,
  `internal/acp/sender.go:7-16`; HTTP mirror: SSE `permission` event plus
  `POST /foxxycode/sessions/{id}/permission`, `external/httpserver/bridge.go:456-483`).
  The `question` tool proves a tool's `Execute` can block on a client round-trip
  today (`internal/tools/question.go:135-143` -> `RequestQuestion`).

### 2.2 The plugin ↔ binary channel

- Each plugin spawns its own `foxxycode http` on loopback and talks plain JSON
  over HTTP plus one SSE stream (`editors/intellij/.../FoxxyCodeProcessManager.kt:106-118`,
  `editors/vscode/src/process/processManager.ts:88-98`). No ACP, no WebSocket,
  no protocol versioning beyond `window.foxxycodeUi.version === 1` for the SPA
  bridge.
- **IDE → binary**: exactly six `/foxxycode/ide/*` routes
  (`external/httpserver/ide_routes.go:5-12`) - editor-state, terminal-state
  (push and pull), copy-buffer, paste-classify, and the permission verdict.
  All of it **telemetry and approval**; nothing asks the IDE to do work.
- **Binary → IDE**: one fire-and-forget SSE hub (`GET /foxxycode/ide/events`,
  `internal/idehost/events.go:7-61`) with **four** event types: `edit_proposed`
  (before the permission prompt, with full before/after),
  `edit_applied` (after the binary wrote the file), `open_file`, `reveal_file`.
  No request/response in this direction at all.
- The native-diff flow shows the current division of labour precisely: the IDE
  renders the proposed edit and answers Accept/Reject as the ordinary
  permission verdict - and **the binary performs the write itself**, then
  announces `edit_applied` (`external/httpserver/bridge.go:266-275`, fed from
  `internal/agent/react.go:558-574`). The IDE writes the disk only in **Revert**,
  a local rollback the binary never hears about.

### 2.3 The dormant ACP seam

`CallClientFS` (`internal/acp/server.go:419-468`) is typed, documented
(`docs/reference/acp-protocol.md:742-769`, `docs/surfaces/editors.md:28`) and
**has no caller** - verified 2026-10-30. The capability handshake
(`clientCapabilities.fs`) is parsed. This is the one place where "run the file
tool in the client" is already the intended design; it is simply not connected
to the fs tools.

## 3. What "through the IDE" would mean per tool

| Tool family | Delegable? | Notes |
|---|---|---|
| `read` / `KeepResult` | yes, low value | `fs/read_text_file` already defined; the IDE win is reading **unsaved buffers** the disk does not have |
| `write` / `edit` / `apply_patch` | **yes, highest value** | the diff/preview flow (`internal/tools/fs/preview.go:32-72`) already computes before/after; the IDE could apply through its document model, keep the caret and undo stack, and accept dirty buffers as the `before` side |
| `glob` / `grep` / `print_tree` | pointless | no IDE API beats direct FS access; keep in the binary |
| `run_command` | hard | "run in the IDE terminal" is possible (IntelliJ JediTerm / VS Code `Terminal.sendText` + shell integration, whose output capture the terminal-state service already uses), but `background: true`, the bgtask pool, output streaming and completion detection do not fit a single request/response; the honest first step is a **"run in my terminal" opt-in for foreground commands** |
| todo / plan / web / docs / config_* / question | no | in-process state (session bundle, todo store) or already client-facing (`question`) |
| `spawn_agent` / memory / scheduler | no | they run in the binary's background pool with no surface attached; delegation must be a **per-surface override with local fallback**, never a replacement |

## 4. Route A - the delegate RPC (what to build)

1. **Dispatch branch** in `executeToolCall` next to the MCP arm
   (`internal/agent/react.go:2463-2476`): if the call is delegated and a client
   with that capability is attached, send it out; on absence, timeout or error,
   fall back to `registry.Execute`. Everything around the block - hooks,
   permission, persistence, `ApplyOutputLimit`, `finishToolCall` - is preserved
   for free.
2. **Transport**: extend `acp.UpdateSender` (`internal/acp/sender.go:7-16`)
   with something like `RequestToolExecution(ctx, params) (string, error)`,
   modeled on `RequestQuestion` (`internal/acp/server.go:387-417`). The HTTP
   bridge gets the matching pair: an outbound request (new SSE event or a
   polled queue - the current hub is one-way) and an inbound
   `POST /foxxycode/ide/tool-result` with `{toolCallId, output, error}`.
   The blocking-permission pattern (`registerPermissionWait`,
   `external/httpserver/bridge.go:456-483`) is the proven template: block with
   timeout, fall back when nothing answers.
3. **Plugin executors**: IntelliJ and VS Code side handlers per delegated tool
   kind - apply-edit-in-document (the diff services already hold before/after
   and write the document on Revert), run-in-terminal. VS Code's terminal
   service today is poll-only; execution needs shell-integration completion
   events.
4. **Capability negotiation**: the plugin must say what it can execute (the
   current channel has no identity or capability handshake at all).

## 5. Route B - ACP client capabilities (smallest real step)

Wire the fs tools through `CallClientFS` when `clientCapabilities.fs` is
advertised (Zed already implements both methods), keeping the local
implementation as fallback. Gains: unsaved-buffer reads/writes in every ACP
editor, one RPC pair that already exists. Gains nothing for our own plugins
until they speak ACP - which is a larger re-architecture (they are built around
`foxxycode http` and the SPA panel) than route A's incremental RPC.

## 6. Risks and open questions

- **Trust**: `/foxxycode/ide/*` is deliberately credential-free for loopback
  clients (`external/httpserver/auth.go:60-75`, `:151-158`). Fine for telemetry
  and permission verdicts; for "the IDE ran a command for the agent" the result
  channel needs an integrity story, or any local process can inject results
  into the agent's transcript.
- **Disk vs buffers**: the preview pipeline reads the disk
  (`internal/tools/fs/preview.go`); an editor-applied write can race the user's
  unsaved edits (the 2026-09-15 stale-highlight fix was a milder form of this).
  Delegated edits should go document → disk, with the preview built from the
  document.
- **No surface attached**: the console, Telegram, `--remote`, scheduler runs,
  memory subagents and background subagents have no IDE behind them - hence
  fallback is mandatory, and delegated state must never be load-bearing for a
  transcript that a headless run continues.
- **Revert asymmetry**: today a revert is IDE-local and the binary never knows
  (`editors/intellij/.../FoxxyCodeIdeDiffService.kt:258-268`). If the IDE
  becomes the executor, this asymmetry inverts into an opportunity - real
  checkpoints (a ROADMAP item) fall out of it - but until then the agent's
  picture of the disk can be wrong.
- **Latency**: every delegated call is a cross-process round trip; fine for
  edits, painful if `read` is delegated wholesale.

## 7. Suggested first experiment

1. Route B for `read`/`write` in the ACP server (~small, testable with the
   existing `examples/acp` harness), capability-gated with local fallback.
2. Route A limited to `write`/`edit`/`apply_patch` in the HTTP bridge for the
   IntelliJ plugin: the diff service already has the before/after and the
   verdict plumbing; only "apply through the document and return success" is
   new. This is where the user-visible win is (undo stack, dirty buffers,
   multi-file diffs).
3. `run_command` in the IDE terminal only as an explicit, opt-in mode for
   foreground commands, with completion via shell integration; leave the
   background family local.

Nothing above needs the tool definitions the model sees to change - only where
`Execute` runs. That is what makes substitution tractable.
