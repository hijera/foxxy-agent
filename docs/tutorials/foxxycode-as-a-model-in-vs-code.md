# FoxxyCode as a model in VS Code Copilot

VS Code Copilot takes any OpenAI-compatible endpoint as a chat model through `chatLanguageModels.json` (Manage Models, then a custom endpoint). Pointing it at a running `foxxycode serve` puts FoxxyCode's models, and FoxxyCode's agent, in the model picker of Copilot Chat. The stream Copilot reads is the strict one described in [Two stream dialects](../reference/http-api.md#two-stream-dialects); the model ids are the ones `GET /v1/models` lists ([HTTP API](../reference/http-api.md)).

1. **Serve on an address VS Code can reach.** The default bind is loopback on port 12345, enough for a VS Code on the same machine. For a VS Code elsewhere, bind and protect the server the way [A server driven from a laptop](server-driven-from-a-laptop.md) does, and hand the token to VS Code as `apiKey`.

   ```bash
   foxxycode serve
   curl -s http://127.0.0.1:12345/v1/models | jq -r '.data[].id'   # the ids step 2 takes
   ```

2. **Register the endpoint.** One entry per model id. A `models[].model` id is FoxxyCode standing in for the provider: Copilot's own tools are offered to the model and its calls come back to Copilot, which runs them in the editor, so VS Code's Agent mode works the way it does against the provider directly; set `toolCalling` on, and `vision` when the entry has `multimodal: true` in `config.yaml`, which is what lets pictures through. The `agent` id is FoxxyCode's ReAct agent as a model: it reads and edits the workspace `foxxycode serve` was started in with its own tools, ignores the tools Copilot offers, and Copilot only sees the answer, so leave `toolCalling` off on that entry and use it from VS Code's Ask mode.

   ```json
   {
     "name": "FoxxyCode",
     "vendor": "customendpoint",
     "apiKey": "foxxycode",
     "apiType": "chat-completions",
     "models": [
       {
         "id": "neuraldeep/qwen3.8-27b",
         "name": "FoxxyCode · Qwen 3.8 27B",
         "url": "http://127.0.0.1:12345/v1/chat/completions",
         "toolCalling": true,
         "vision": true,
         "maxInputTokens": 128000,
         "maxOutputTokens": 8192
       },
       {
         "id": "agent",
         "name": "FoxxyCode · agent",
         "url": "http://127.0.0.1:12345/v1/chat/completions",
         "toolCalling": false,
         "maxInputTokens": 128000,
         "maxOutputTokens": 8192
       }
     ]
   }
   ```

   Copilot sends its own `temperature` with every request, and a direct model uses it in place of the configured one. A reasoning model may refuse it: a `codex` model always does, and so does an `anthropic` model with extended thinking on (its `reasoning_default` included) for any value but `1` - FoxxyCode answers `400` for both before calling the provider; OpenAI's reasoning models refuse the value Copilot sends as well, while Qwen3 thinking on vLLM takes it. Give the entry of every model that reasons `"thinking": true` - a `codex` model, or one whose `GET /v1/models` row carries `reasoning_levels` - and Copilot leaves `temperature` out, as it does for any reasoning model.

   Bearer auth is off by default, and any `apiKey` string satisfies VS Code's form; with `httpserver.auth_token` set, the `apiKey` is that token. Copilot sends the whole conversation with every request, so each request is a session of its own on the server unless a `requestHeaders` entry on the model sets `X-FoxxyCode-Session-ID` to a fixed `sess_<hex>` id, which keeps the turns in one transcript.

3. **Mind the agent's permission gate.** The `agent` model runs tools under `tools.permission_mode`, and a permission prompt cannot be answered from Copilot: the turn waits for an answer that never comes. Give the serving config `bypass` when the workspace is one you trust the agent with, or keep `ask` and answer the prompt in the web UI, where the turn is live under its session.

   ```yaml
   tools:
     permission_mode: bypass
   ```

4. **Check that it worked.** Pick "FoxxyCode · Qwen 3.8 27B" in Copilot Chat and ask anything: the answer streams in, and with a reasoning model the thinking shows in Copilot's own foldout, because `reasoning_content` is a field it reads. The same request from a terminal shows the stream Copilot consumed, one choice finished with `stop` before `[DONE]`:

   ```bash
   curl -sN http://127.0.0.1:12345/v1/chat/completions -H 'Content-Type: application/json' \
     -d '{"model":"neuraldeep/qwen3.8-27b","messages":[{"role":"user","content":"hello"}],"stream":true}' | tail -4
   ```

   "Response contained no choices." in Copilot means the server did not finish a choice: check that `foxxycode serve` is a version that streams `finish_reason` (`foxxycode -v`, later than 0.2.84), and that nothing between them rewrites the stream.
