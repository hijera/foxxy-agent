# IDE tool delegation

A tool call can be **executed by a connected IDE client** instead of the agent: the editor plugin that declared the tool on its event-stream subscription receives the call, performs it through the editor - `write` / `edit` / `apply_patch` through the document model, for example - and returns the tool result. The model sees an ordinary tool call and result; only where the work happened changes.

The point of delegating the filesystem writes is what the editor gives: edits land in the user's **dirty buffers**, ride the **undo stack**, and can be reviewed in a native diff - while a local write by the agent silently overwrites whatever the user had open but unsaved.

## How it works

1. The plugin subscribes to **`GET /foxxycode/ide/events?handles=write,edit,apply_patch`** - the `handles` parameter names the tools it can execute (`*` means all of them; names match exactly as the model called the tool, so MCP names like `server__tool` work). A subscription without `handles` only watches events, as before.
2. When the model calls one of those tools, the agent sends **`event: tool_execute`** to one capable subscriber: `{type, toolCallId, sessionId, tool, args, nonce, cwd, deadline, path, before, after}`. `tool` and `args` are the raw tool call (name and arguments exactly as the model called them); `cwd` is the session workspace the call runs in, so relative paths resolve there; `deadline` is the epoch-ms moment the agent stops waiting and runs the call itself; `path` / `before` / `after` carry the resolved diff for `write` / `edit` / `apply_patch`, computed with the same transforms the local write would apply. The call waits - nothing is written by the agent. Only the selected client receives the request and its nonce; other clients cannot execute or decline that call. Full subscriber queues are skipped. If none can accept delivery, the agent runs the tool locally immediately. Ordinary edit notifications still reach every subscriber.
3. The plugin executes the call and answers **`POST /foxxycode/ide/tool-result`** with `{toolCallId, nonce, status, output?, error?}`:
   - **`handled`** - the client ran the call; `output` (or `error`) is the tool result the model receives;
   - **`not_handled`** - the client declines this call; the agent runs it locally.

Everything around the call is unchanged: PreToolUse/PostToolUse hooks, the permission gate (a delegated write is still asked for when the mode says so), result persistence, output caps, transcript rows.

## Fallback

Delegation is best-effort by design:

- **no client declared the tool** - the call runs in the agent, as always (the console, Telegram, scheduler runs and memory subagents never delegate);
- **the session moved to another workspace** (a worktree switch) - the agent does not offer the call to a client attached to a different checkout; the paths would land in the wrong tree;
- **the file is not UTF-8** - the agent decodes workspace files through its own textenc while an editor document decodes with the file's charset, so a legacy-encoded file stays with the agent's own tools, which read and write the right bytes;
- **the client declines or errors before taking the call** (`not_handled`) - local run;
- **the client does not answer within 60 seconds** - local run;
- **the client only starts the call after `deadline`** - the agent has already run it locally, so the client must not run it at all;
- **a turn cancelled while the client was deciding** - the call ends cancelled; it is not replayed locally;
- **subagent children never delegate** - their calls run in the agent, whatever the parent's surface can do.

## What a client must do

- Declare only the tools it can really execute; a `handled` answer claims the work is done - the agent will not verify it.
- For a delegated `write` / `edit` / `apply_patch`: compare the document text against `before`, ignoring a BOM and line-ending style (the preview carries the raw file bytes; a document does not). Equal - apply `after` through the document model and answer `handled`. Different - answer `error` with a conflict message ("file changed in the editor since it was read"), so the model re-reads and retries; do not silently apply against a stale buffer.
- Echo the `nonce` from the event; a result without one is rejected with `400`, and one with a wrong nonce (or naming no pending call) with `409`.
- Answer `not_handled` if the selected client can no longer execute the offered tool (for example, its setting changed after subscription). The agent falls back locally; it does not offer the same call to another client.

## Trust

`/foxxycode/ide/*` is open to a direct loopback client without a credential (see [HTTP API](../reference/http-api.md)), which is how the editor plugins call it. The `nonce` keeps a blind local process from forging results without watching the stream - the same trust model as the permission verdicts these clients already send.

## Client support

The **IntelliJ plugin** delegates `write` / `edit` / `apply_patch` (through the editor document: the edit lands in the document, open buffers included, on the undo stack, and is saved immediately - unless the buffer holds unsaved changes or diverged from the file, in which case the agent gets a conflict and re-reads), `read` from the editor buffer (unsaved edits included), `plan_write` through the document, `mkdir` / `rm` / `mv` / `touch` through the IDE's filesystem (the tree updates at once, Local History can roll a change back), and foreground `run_command` in an IDE terminal tab, whose captured output (plus the exit code where the shell expands it) becomes the tool result. Each tool has its own switch in Settings | Tools | FoxxyCode, and the subscription declares exactly what the switches allow. Non-UTF-8 files stay with the agent's own tools (see the fallback above). The same panel pushes a **pre-send context snapshot**: right before a message goes out it reports the open buffers' text (unsaved edits included), so `<foxxycode_ide_context>` carries what the user is actually working on from the first message; the push honours the **trackOpenFiles** switch. The **Apply file edits through the editor** master setting (on by default) turns the execution side off, falling back to the agent's own runs. The VS Code extension does not speak the protocol yet. The full design record is [`docs/plans/ide-tool-delegation.md`](../plans/ide-tool-delegation.md).

When the binary override points to **Coddy 1.4.16 or newer**, the same IntelliJ execution switches apply through Coddy's synchronous `ToolExecute` hook. The plugin also supplies the open-buffer snapshot through `FirstPrompt`, whose context stays out of the cached system prompt. It manages only its own handlers in the selected Coddy home's `hooks.json`, preserving other hooks, removes them when the project or plugin closes, and prunes the handlers of instances that died without a dispose. Hooks answer only sessions whose cwd is inside that IDE project; `plan_write`, sessions that moved elsewhere and non-UTF-8 files stay local to Coddy. Coddy has no `/foxxycode/ide/*` API, so native inline diffs, editor-state POSTs, terminal tracking, copy-buffer sync, autocomplete and the SPA host bridge used for file mentions stay specific to the FoxxyCode binary.

Delegated `run_command` honors its own `cwd` argument: relative paths resolve against the session workspace and absolute paths are used directly. A missing directory or a path to a file fails before starting a terminal command. Files whose IDE charset is not UTF-8 stay with the native file tools, including calls delivered through the Coddy hook.
