# IntelliJ startup and first-response diagnostics

## Capture a cold and a warm request

In Settings | Tools | FoxxyCode, add these extra backend arguments:

```text
--log-level debug --log-output stderr
```

Restart the IDE, open the FoxxyCode panel, and send a short message. Record the time
of opening the panel, sending the message, and seeing the first output. Send another
message in the same session for comparison. Open the IDE log directory through
Help | Show Log in Explorer and inspect `idea.log`: the plugin forwards backend
stderr with the `[foxxycode]` prefix. Keep the `starting HTTP server` line, which
includes the bundled agent version. These diagnostics require a plugin rebuilt
with the updated backend and Kotlin sources; enabling debug on an older plugin
does not add the new events. Remove the extra arguments after collecting logs.

The plugin records `startup stage=... elapsed_ms=...` at info level, including
proxy resolution, process launch, readiness polling, and locale loading. Those
times are cumulative from startup. Backend debug events use `session`, `call_id`
(one provider operation), and `http_id` (one SDK HTTP attempt) for correlation.
Each HTTP event's `elapsed_ms` is cumulative from that HTTP attempt, so subtract
adjacent event times to measure a phase. Agent stage durations are measured separately.

## Interpret the last event before the gap

| Event or interval | What it locates |
| --- | --- |
| `proxy_resolution_start` to `proxy_resolution_end` | IDE proxy selection, including PAC/WPAD when configured; happens before the readiness deadline starts. |
| `process_launch_start` to `readiness_end` | Local backend startup and readiness polling. The current readiness loop has a 30-second deadline, with bounded individual probes. |
| `session.mcp.start` to `session.mcp.end` | One MCP server startup/initialization while creating or restoring a session. |
| `session.prompt.start` to `session.prompt.lock_acquired` | Waiting for the session turn lock. |
| `session.prompt.lock_acquired` to `session.prompt.prepared` | Preparing attachments, file mentions, and persisted prompt data. |
| `agent.stage.start` to `agent.stage.end` | Memory before the turn, provider setup (including credential resolution), context construction, or compaction. These can precede the first-token timer. |
| `llm.http.dns_start` to `dns_done` | Local DNS lookup. With an HTTP proxy this may resolve the proxy; the proxy's own upstream DNS is not visible. |
| `llm.http.connect_start` to `connect_done` | TCP connection; `address` shows the actual dial target, which may be the proxy. |
| TCP completion to TLS start/connection ready | May include HTTP proxy CONNECT negotiation. Standard client tracing does not isolate CONNECT, proxy-side DNS, and upstream TCP into separate phases. |
| `llm.http.tls_start` to `tls_done` | TLS negotiation. An HTTPS proxy can introduce another TLS handshake. |
| `llm.http.connection` with `reused=true` | An existing connection was reused; compare with the cold request's DNS/TCP/TLS stages. |
| `request_written` to `first_response_byte` / `headers` | Waiting for an HTTP response from the proxy or provider; this alone cannot identify which side delayed it. |
| `headers` to `first_body_byte` | Response body/stream delay; buffering or upstream generation are possible causes. |
| `first_body_byte` to `llm.first_chunk` | Body data arrived before usable text, reasoning, or a tool call. SSE metadata/keepalives are not model output. |
| `llm.http.start` with `sdk_retry` greater than zero | An SDK retry, with preceding HTTP status or error type explaining the failed attempt. The interval between attempts includes SDK backoff. |
| `llm.retry.wait` / `llm.pacing.wait` | FoxxyCode's own retry backoff / configured spacing between calls; includes `delay_ms`. |

The current OpenAI SDK defaults to two retries, and FoxxyCode adds up to three
outer retries by default for selected HTTP errors. Multiple attempts can therefore
be hidden behind a successful final response. These diagnostics preserve that
behavior and existing timeout/proxy settings.

The main agent stream currently cancels after 90 seconds without its first output.
A roughly three-minute delay that then succeeds is not sufficient evidence of a
single blocked model connection: check preparation stages, SDK backoff, the installed
agent version, and whether earlier streamed output had already stopped that timer.
The UI reasoning duration starts with the first reasoning delta; it excludes the
earlier connection wait.

If backend events show timely output but the panel is delayed, use the plugin's
Open DevTools action to inspect the local `POST /v1/responses` request and its
stream timing. The Kotlin HTTP probes and embedded JCEF browser have their own
network paths; the Go process's `NO_PROXY` environment does not configure them.

New LLM diagnostic events log timing, host, connection address, HTTP status, and
error classification. They omit request/response bodies, authorization headers,
proxy credentials, URL paths/query strings, and raw error messages. Other existing
log events are separate from these diagnostics; inspect a captured log before sharing it.
