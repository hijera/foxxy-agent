# Diagnostics (`debug`)

When a turn goes wrong the transcript rarely says why. The model asked for a tool that was never offered, the provider rejected the request body, a turn burned four minutes on one call — none of that is visible in the answer the user sees. The diagnostics layer is the switch that makes a turn inspectable: it records what FoxxyCode actually sent to the LLM, what came back, and every tool boundary in between.

It is **off by default** and costs nothing when off.

> **Not to be confused with Debug mode.** `debug` is also the name of a *session mode* — the diagnose-before-fixing persona selected with `model: "debug"` (see [`docs/contributing/react-agent.md`](../contributing/react-agent.md) and the mode list in [`docs/contributing/architecture.md`](../contributing/architecture.md)). The two are unrelated: the mode changes how the **model** behaves, the diagnostics layer changes what **FoxxyCode records**. Either works without the other — you can run a Debug-mode session with diagnostics off, and diagnose an Agent-mode session with diagnostics on.

## Turning it on

```yaml
debug:
  enable: true      # master switch for the whole layer
  capture_llm: true  # optional; omit to follow `enable`
```

`enable: true` does four things at once: it forces the process logger to **debug** level regardless of `logger.level`, turns on raw LLM HTTP capture, traces every LLM connection step by step (see [Connection trace](#connection-trace)), and starts writing a per-session trace.

`capture_llm` is the one knob that opts out of a part of it. Leave it unset and it follows `enable`. Set it explicitly to `false` to keep the debug-level logs, the connection trace and the session trace while suppressing the request/response bodies — the right setting when the conversation carries content you would rather not have on disk (see [Privacy](#privacy-what-lands-on-disk)).

### The `--debug` flag

| Entry point | `--debug` flag | Honours `debug.enable` from config |
|---|---|---|
| `foxxycode acp` | yes | yes |
| `foxxycode http` | yes | yes |
| `foxxycode gateway` (tag `gateway`) | yes | yes |
| `foxxycode desktop` (tag `desktop`) | no | yes |
| `foxxycode` / `foxxycode cli` (tag `cli`) | no | yes |

The flag only ever turns diagnostics **on**. Passing `--debug=false` does not disable a config-enabled layer — the flag is read only when it was explicitly provided, so a default-false flag can never silently override `config.yaml`. Same semantics as `--plan-no-self-run`.

### Toggling at runtime

`PUT /foxxycode/config` with `debug.enable` flipped takes effect **immediately, without restarting the process**. The logger is built once over a shared `slog.LevelVar` (`internal/logger`), so `ReplaceConfig` re-levels the existing handler instead of rebuilding it, and the raw-capture and connection-trace switches are atomic flags read per request (`llm.ApplyDebugConfig`, `internal/llm/debug_capture.go`). Turning it off mid-session stops new capture and new trace events; what was already written stays.

This is the intended way to catch a problem you cannot reproduce on demand: leave the server running, turn diagnostics on when the user reports the bad turn, turn it off afterwards.

## What it produces

Three separate outputs, deliberately: the bulky raw bodies go to the process log, while the lightweight structured timeline goes somewhere a UI can read it.

| Output | Where | Gated by |
|---|---|---|
| Raw LLM HTTP request/response excerpts | the process log (`logger.outputs` / `logger.file`), at `DEBUG` level | `capture_llm` |
| LLM connection trace (`llm net: ...` lines) | the process log, at `DEBUG` level | `enable` |
| One line per model call (`llm call started` / `llm call finished`) | the process log, at `DEBUG` level | `enable` |
| Structured turn timeline | `<session bundle>/debug_trace.jsonl` | `enable` |
| The same timeline, live | SSE `event: debug` on the composer stream | `enable` |
| The same timeline, on demand | `GET /foxxycode/sessions/{id}/debug` | `enable` |

## The trace timeline

`internal/agent/debug_emit.go` emits one event per boundary in the ReAct loop. Tracing is best-effort: a write failure is logged and the turn continues, because diagnostics must never break a turn.

| Phase | Emitted | `meta` fields |
|---|---|---|
| `turn_start` | once per loop iteration | `mode`, `model`, `messages`, `tools` |
| `llm_request` | before each provider call | `model`, `messages` |
| `llm_response` | after each provider call | `stop_reason`, `input_tokens`, `output_tokens`, `tool_calls` |
| `tool_start` | before each tool executes | `tool_call_id`, `kind` |
| `tool_finish` | after each tool returns | `tool_call_id`, `kind`, `status`, `ok` |

Every event also carries `turn` (the loop iteration) and `at` (RFC3339 UTC). A real six-call turn reads like this:

```
 0 turn=0 turn_start    debug        messages=2 mode=debug model=neuraldeep/qwen3.6-35b-a3b
 1 turn=0 llm_request                messages=2 model=neuraldeep/qwen3.6-35b-a3b
 2 turn=0 llm_response               input_tokens=13003 output_tokens=112 stop_reason=tool_use
 3 turn=0 tool_start    grep         kind=read tool_call_id=chatcmpl-tool-b420d311e714a53a
 4 turn=0 tool_finish   grep         kind=read ok=true status=completed
 5 turn=1 turn_start    debug        messages=5 mode=debug model=neuraldeep/qwen3.6-35b-a3b
```

That is usually enough on its own to answer the common questions: how many round trips the turn took, where the tokens went, which tool failed, and whether the model was still asking for tools when the turn ended.

### Reading it

```bash
curl -s localhost:12345/foxxycode/sessions/<id>/debug
```

```json
{ "object": "foxxycode.session_debug", "sessionId": "sess_...", "events": [ ... ] }
```

A session with no trace answers `"events": null` rather than 404 — "nothing was collected" is a normal state, not an error. The endpoint is read-only and reads the file directly, so it works on a session this process never ran.

Clients that want it live get the same records as `event: debug` frames on `GET /foxxycode/sessions/{id}/composer-stream` (and over ACP as the `debug` session update, `internal/acp/types.go`).

### Operational notes

- The file is **append-only and never trimmed**. A long diagnostics session on a chatty agent grows it without bound; it is deleted with the session bundle, and turning `enabled` off stops the growth.
- Malformed lines are skipped on read rather than failing the whole request, so a truncated write (a killed process mid-append) does not make the endpoint unusable.

## Raw LLM capture

`internal/llm/debug_transport.go` wraps every provider's HTTP client — the wrapper is installed in `HTTPClientForOptionalProxy`, so openai, anthropic, codex, and neuraldeep are covered uniformly, direct or through a proxy.

Each call logs one `llm http request` line (method, URL, request body) and one `llm http response` line (body) at `DEBUG` level.

Two details matter:

- **Bodies are capped at 16 KB** with a truncation marker appended. LLM payloads carry the whole conversation plus every tool schema, so an uncapped dump would swamp the log. The *provider* still receives the complete body — the cap applies only to what is logged.
- **Streaming stays streaming.** The response body is teed as it is read, not buffered, so an SSE stream is not held back waiting to be recorded. The response excerpt is written when the body closes, which is why the response line appears *after* the turn has already streamed to the client.

### Privacy: what lands on disk

**Request headers are never logged**, so provider API keys do not reach the log. Verified: a capture session against a live provider leaves zero occurrences of the key in the log file.

**Request bodies are logged**, and an LLM request body contains the full conversation — the system prompt, the user's messages, and the contents of every file the agent read into context. Treat a log file produced with `capture_llm` on as carrying everything the agent saw. If that is a problem for the workspace in question, set `capture_llm: false` and keep the trace, which carries only counts and identifiers.

## Connection trace

`internal/llm/net_trace.go` follows every LLM HTTP request at the network level, through the same wrapper as the raw capture. It logs no bodies and no headers, so it stays on under `capture_llm: false`. It exists for the turn that hangs: behind a proxy a request can die in the proxy's tunnel, in TLS, on a pooled connection that was already dead, or at an upstream that took the request and went quiet. All of these look the same from the agent loop.

Every line carries `req=<n>`, the number of the request in this process. A model call from a turn also carries `session`, `turn` and `call`, so the request can be matched to the turn that waited on it.

| Line | When | Fields |
|---|---|---|
| `llm net: request` | the request starts | `method`, `url` (scheme and host only), `route`: `direct`, `direct (proxy bypassed: loopback or NO_PROXY)`, `proxy http://...`, `socks socks5h://...` or `env-proxy http://...` (a proxy inherited from `HTTP(S)_PROXY`). The proxy URL keeps only its scheme and host; user info is replaced with `redacted`. Paths, queries and fragments are omitted. |
| `llm net: dns` | a name was resolved (the proxy's name, behind a proxy) | `addrs`, `took`, error |
| `llm net: dialed` | a TCP connection opened or failed (to the proxy, behind one) | `addr`, `took`, error |
| `llm net: socks dial` | the SOCKS handshake finished | `proxy`, `target`, `local`, `took`, error |
| `llm net: proxy CONNECT answered` | an HTTP proxy answered the tunnel for an `https` target | `proxy`, `target`, `status`, `since_dial` |
| `llm net: tls` | the TLS handshake finished | `server_name`, `version`, `alpn` (`h2` = HTTP/2), `resumed`, `took`, error |
| `llm net: conn` | the request got a connection | `reused`, `was_idle`, `idle_for`, `local`, `remote` |
| `llm net: request sent` | the request is written | error |
| `llm net: first response byte` | the server started answering | `elapsed` |
| `llm net: response` | headers arrived | `status`, `proto`, `content_type`, `content_length`, `request_id` |
| `llm net: no activity` | nothing happened for 15 s, then again every 15 s while the silence lasts | `phase`, `idle`, `elapsed`, `bytes` |
| `llm net: activity resumed` | data arrived after a reported silence | `phase`, `silence` |
| `llm net: request context ended` | the caller cancelled the request before it finished | `phase`, `idle`, `cause`: who cut it, for example `agent: no first token within agent.llm_first_token_timeout_ms` |
| `llm net: body read failed` | the response stream broke | `phase`, `idle`, `bytes`, error |
| `llm net: done` / `llm net: request failed` | the request ended | `outcome` (`eof`, `closed_early`, `read_error`, `request_failed`), `phase`, `elapsed`, `bytes`, `reads`, `first_body_after`, `max_gap`, error |

`phase` is the step the request was in: `get_conn`, `dns`, `dial`, `socks_dial`, `socks_handshake`, `proxy_connect`, `connected`, `tls`, `sending_request`, `awaiting_response`, `reading_headers`, `streaming`. Errors come with `error`, `error_kind` (`conn_refused`, `conn_reset`, `conn_aborted`, `timeout`, `dns`, `proxy_connect`, `socks`, `tls`, `tls_timeout`, `response_header_timeout`, `http2`, `unexpected_eof`, `eof`, `canceled`, `deadline`, `other`), and `op`, `op_addr` and `errno` when a socket operation failed.

Next to it, the agent loop logs `llm call started` for every model call, with the model, the message and tool counts and both guard timeouts. It logs `llm call finished` with `took`, `first_progress_after`, `output`, `cut_by` (`first_token_timeout`, `stall_timeout`, `loop_guard`, `user`) and the error. The retry layer inside `internal/llm` logs `LLM request failed; retrying`, `LLM request failed; retries exhausted` and, at debug level, `LLM request failed; not retrying` with the reason. All of these go to the process log, including the retry lines, which used to go to stderr, where the editor plugins and the desktop app discard them.

For the SSE frames themselves (the gaps between frames, the terminal frame, which channel carried what), also set `FOXXYCODE_LLM_TRACE` (see [environment variables](../reference/environment-variables.md)).

## Recipes

**"The turn sits on *Provider is not responding, retrying* for an hour."** That status is the stall ladder (`agent.llm_stall_retry_*`): each silent attempt waits out `llm_first_token_timeout_ms`, then the loop pauses before the next one. Turn `debug.enable` on, reproduce, and read the log from the start of the turn:

1. `llm call finished ... cut_by=first_token_timeout`, repeated, means every attempt got nothing back. The `llm net:` lines with the same `turn` and `call` show where each attempt stopped.
2. `no activity phase=proxy_connect` or `phase=socks_handshake` means the proxy never set up the tunnel. `phase=dial` or `phase=dns` means the proxy itself cannot be reached.
3. `phase=awaiting_response` means the request went through and nothing came back. Look at the `llm net: conn` line of that request. `reused=true` with the same `local` port as the requests before it, over `alpn=h2`, is a pooled HTTP/2 connection the proxy no longer forwards: every new request lands on it and waits.
4. `phase=streaming` with `bytes` growing and then stuck is the upstream stopping mid-answer (the stall guard's case), not the proxy.
5. `request_failed` and `body read failed` carry `error_kind`. `conn_reset` or `unexpected_eof` means something on the route closed the connection.


**"The model called a tool it shouldn't have."** Read the trace: `turn_start` gives the `tools` count for that iteration, and `tool_start` gives the name and `kind`. If the count looks wrong, the mode's tool set is the suspect (`internal/agent/toolsets.go`).

**"The provider rejected the request."** Turn on `capture_llm` and read the `llm http request` line — the exact body FoxxyCode sent, including whatever the provider objected to. The paired response line carries its error verbatim.

**"The turn was slow."** Compare `at` between `llm_request` and `llm_response` versus between `tool_start` and `tool_finish` to see whether the time went to the model or to a tool.

**"Tokens ran away."** `llm_response.input_tokens` per turn shows the context growing round to round; a jump usually points at a large tool result that should have been paged or evicted (see result eviction in [`docs/contributing/react-agent.md`](../contributing/react-agent.md)).

## Where the code lives

| Area | File |
|---|---|
| Config section and `--debug` semantics | `internal/config/debug.go` |
| Runtime log level (`slog.LevelVar`) | `internal/logger/format.go`, `internal/logger/builder.go` |
| Raw HTTP capture | `internal/llm/debug_transport.go`, `internal/llm/debug_capture.go` |
| Connection trace | `internal/llm/net_trace.go`, `internal/llm/proxy_http_client.go` |
| Per-call log and guard causes | `internal/agent/llm_call_log.go`, `internal/agent/react.go` |
| Trace store (JSONL) | `internal/session/debug_trace.go` |
| Event emission in the ReAct loop | `internal/agent/debug_emit.go`, `internal/agent/react.go` |
| ACP update type | `internal/acp/types.go` (`DebugUpdate`) |
| HTTP endpoint | `external/httpserver/foxxycode_foxxycode.go` |
| Runtime re-application on config save | `external/httpserver/server.go` (`ReplaceConfig`) |
