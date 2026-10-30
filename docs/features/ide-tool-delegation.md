# IDE tool delegation

A tool call can be **executed by a connected IDE client** instead of the agent: the editor plugin that declared the tool on its event-stream subscription receives the call, performs it through the editor - `write` / `edit` / `apply_patch` through the document model, for example - and returns the tool result. The model sees an ordinary tool call and result; only where the work happened changes.

The point of delegating the filesystem writes is what the editor gives: edits land in the user's **dirty buffers**, ride the **undo stack**, and can be reviewed in a native diff - while a local write by the agent silently overwrites whatever the user had open but unsaved.

## How it works

1. The plugin subscribes to **`GET /foxxycode/ide/events?handles=write,edit,apply_patch`** - the `handles` parameter names the tools it can execute (`*` means all of them; names match exactly as the model called the tool, so MCP names like `server__tool` work). A subscription without `handles` only watches events, as before.
2. When the model calls one of those tools, the agent broadcasts **`event: tool_execute`** on the stream: `{type, toolCallId, sessionId, tool, args, nonce, path, before, after}`. `tool` and `args` are the raw tool call (name and arguments exactly as the model called them); `path` / `before` / `after` carry the resolved diff for `write` / `edit` / `apply_patch`, computed with the same transforms the local write would apply. The call waits - nothing is written by the agent.
3. The plugin executes the call and answers **`POST /foxxycode/ide/tool-result`** with `{toolCallId, nonce, status, output?, error?}`:
   - **`handled`** - the client ran the call; `output` (or `error`) is the tool result the model receives;
   - **`not_handled`** - the client declines this call; the agent runs it locally.

Everything around the call is unchanged: PreToolUse/PostToolUse hooks, the permission gate (a delegated write is still asked for when the mode says so), result persistence, output caps, transcript rows.

## Fallback

Delegation is best-effort by design:

- **no client declared the tool** - the call runs in the agent, as always (the console, Telegram, scheduler runs and memory subagents never delegate);
- **the client declines or errors before taking the call** (`not_handled`) - local run;
- **the client does not answer within 60 seconds** - local run;
- **a turn cancelled while the client was deciding** - the call ends cancelled; it is not replayed locally;
- **subagent children never delegate** - their calls run in the agent, whatever the parent's surface can do.

## What a client must do

- Declare only the tools it can really execute; a `handled` answer claims the work is done - the agent will not verify it.
- For a delegated `write` / `edit` / `apply_patch`: compare the document text against `before`. Equal - apply `after` through the document model and answer `handled`. Different - answer `error` with a conflict message ("file changed in the editor since it was read"), so the model re-reads and retries; do not silently apply against a stale buffer.
- Echo the `nonce` from the event; a result without it (or with a wrong one) is rejected with `409`.
- Answer `not_handled` for anything it did not declare - it may still see `tool_execute` events for tools another subscriber declared.

## Trust

`/foxxycode/ide/*` is open to a direct loopback client without a credential (see [HTTP API](../reference/http-api.md)), which is how the editor plugins call it. The `nonce` keeps a blind local process from forging results without watching the stream - the same trust model as the permission verdicts these clients already send.

## Client support

The **IntelliJ plugin** executes `write` / `edit` / `apply_patch` through the editor since its latest version: the edit lands in the document (open buffers included), on the undo stack, and is saved immediately - unless the buffer holds unsaved changes or diverged from the file, in which case the agent gets a conflict and re-reads. The **Apply file edits through the editor** setting (Settings | Tools | FoxxyCode, on by default) turns it off, falling back to the agent's own writes. The VS Code extension does not speak the protocol yet. The full design record is [`docs/plans/ide-tool-delegation.md`](../plans/ide-tool-delegation.md).
