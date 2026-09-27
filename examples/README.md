# Examples and e2e harnesses

## Naming

Paired HTTP and ACP scripts share the same stem after the prefix:

| Stem | HTTP | ACP |
|------|------|-----|
| **`smoke_gateway`** | **`httpserver/http_smoke_gateway.py`** | **`acp/acp_smoke_gateway.py`** |
| **`e2e_models`** | **`httpserver/http_e2e_models.py`** | **`acp/acp_e2e_models.py`** |
| **`e2e_web`** | **`httpserver/http_e2e_web.py`** | **`acp/acp_e2e_web.py`** |
| **`e2e_todo`** | **`httpserver/http_e2e_todo.py`** | **`acp/acp_e2e_todo.py`** |
| **`e2e_memory`** | **`httpserver/http_e2e_memory.py`** | **`acp/acp_e2e_memory.py`** |
| **`e2e_background`** | **`httpserver/http_e2e_background.py`** (task list, live output, stop, 404) | **`acp/acp_e2e_background.py`** (persisted `background/<id>/meta.json` plus `output.log`) |
| **`e2e_toolcalls_persist`** | **`httpserver/http_e2e_toolcalls_persist.py`** | **`acp/acp_e2e_toolcalls_persist.py`** |
| **`e2e_compact`** | **`httpserver/http_e2e_compact.py`** (`/compact` prompt + REST endpoint; a manual trigger folds even a one-turn session) | **`acp/acp_e2e_compact.py`** (also auto threshold via tiny-window config) |
| **`e2e_compact`** | **`httpserver/http_e2e_compact.py`** (`/compact` prompt + REST endpoint); **`httpserver/http_e2e_compact_auto.py`** (self-boots a server whose model has no `max_context_tokens`: the `GET /v1/models` window equals the provider's listing and every `usage_update` size, and the session compacts by itself); **`httpserver/http_e2e_compact_clients.py`** (the sender, a second tab on the composer stream and an idle viewer all read the context usage fall after `/compact`, REST compact and an automatic compaction) | **`acp/acp_e2e_compact.py`** (also auto threshold via tiny-window config) |
| **`e2e_skills_slash`** | **`httpserver/http_e2e_skills_slash.py`** | **`acp/acp_e2e_skills_slash.py`** |
| **`e2e_rules`** | **`httpserver/http_e2e_rules.py`** | **`acp/acp_e2e_rules.py`** |
| **`e2e_mentions`** | **`httpserver/http_e2e_mentions.py`** (`GET /foxxycode/workspace/file`, `attachments[].source.startLine`/`endLine`, the typed `@file:N-M` grammar, `400` for lines past the end) | **`acp/acp_e2e_mentions.py`** (typed `@file:N-M`, a `resource` with a `#L5-5` URI fragment, refused range past the end) |
| **`e2e_scheduler_api`** | **`httpserver/http_e2e_scheduler_api.py`** | (REST is HTTP-only) |
| **`e2e_scheduler_agent`** | **`httpserver/http_e2e_scheduler_agent.py`** | **`acp/acp_e2e_scheduler_agent.py`** |
| **`e2e_plan_files`** | **`httpserver/http_e2e_plan_files.py`** | **`acp/acp_e2e_plan_files.py`** |
| **`e2e_ask_mode`** | **`httpserver/http_e2e_ask_mode.py`** (ask profile reads, never writes; agent on the same session writes) | **`acp/acp_e2e_ask_mode.py`** (`session/set_mode` ask, then agent) |
| **`e2e_subagents`** | **`httpserver/http_e2e_subagents.py`** (trust route, `spawn_agent` run as an `agent` task, read-only child transcript, `include_subagents`, catalog) | **`acp/acp_e2e_subagents.py`** (`foxxycode agents trust`, persisted `agent` task plus `sub_*` child bundle with the parent link) |
| **`e2e_hooks`** | **`httpserver/http_e2e_hooks.py`** (catalog with the held project file, the notice row in the transcript, `POST /foxxycode/hooks/trust` then a turn that runs the approved hook, `untrust`) | **`acp/acp_e2e_hooks.py`** (user-scope `PreToolUse` / `PostToolUse` recorder hooks see a real `run_command`, the project-scope hook stays held until `foxxycode hooks trust .foxxycode/hooks.json`, then runs on the next turn) |
| **`e2e_config`** | **`httpserver/http_e2e_config.py`** (stage, confirm-commit, rollback; server config ends unchanged) | **`acp/acp_e2e_config.py`** (temp config copy; staged file, commit snapshot, rollback) |
| **`e2e_login`** | **`httpserver/http_e2e_login.py`** (self-boots servers behind the web UI sign-in: the account from `FOXXYCODE_HTTP_USER` / `FOXXYCODE_HTTP_PASSWORD` and from `foxxycode serve set-password`, the anonymous 401s, the cookie, the CSRF refusal, config redaction, sign-out, bearer parity, `login.enable: false`) | (HTTP-only) |

## Layout

| Path | Role |
|------|------|
| **`config.demo.yaml`** | Shared YAML for demos (models, scheduler, skills dirs, logger placeholder **`__E2E_LOG_PATH__`** where scripts rewrite it). |
| **`build_foxxycode.sh`** | **`make build TAGS="http scheduler memory gateway"`** then **`./build/foxxycode -v`**. |
| **`httpserver/`** | HTTP Python harnesses, **`test_httpserver.sh`**, **`docker.sh`**. |
| **`acp/`** | ACP Python harnesses and **`test_acp.sh`**. |
| **`cli/`** | Console TUI harnesses and **`test_cli.sh`** (pty-driven, Linux-only). |
| **`gateway/`** | **`tg_e2e_offline.sh`** (wrapper **`test_gateway.sh`**): the Telegram bot against the fake Bot API and scripted model of **`cmd/tgfake`**, no Telegram and no LLM involved (bash, Git Bash on Windows included). |
| **`shared/`** | **`scheduler_e2e_common.py`**, **`plan_e2e_common.py`**, **`ask_e2e_common.py`** for paired e2e harnesses. |
| **`agents_fixture/`** | Project-scope subagent definition **`.foxxycode/agents/marker-reporter.md`** (read-only, reports the `MARKER:` line of a named file); each **`e2e_subagents`** script copies it into its work dir and approves it before the spawn. |
| **`skills_fixture/`** | Bundled skill for slash-command HTTP demo (copied into **`$FOXXYCODE_HOME/skills_fixture`** by **`test_httpserver.sh`**). |

## Telegram offline stand

```bash
./examples/build_foxxycode.sh
./examples/test_gateway.sh                        # boots tgfake --llm and foxxycode serve --gateway, sends "hello", checks the reply
TG_E2E_KEEP=1 ./examples/test_gateway.sh             # leaves both running and prints the chat page URL
RICH_MESSAGES=true ./examples/test_gateway.sh
```

Knobs: **`TG_PORT`** (18790), **`LLM_DELAY`** (50ms), **`TG_VERBOSE`** (one line per Bot API call), **`FOXXYCODE_BIN`**. Guide: [Debugging against a fake Bot API](../docs/surfaces/gateway.md#debugging-against-a-fake-bot-api).

## HTTP gateway

From the repository root:

```bash
./examples/build_foxxycode.sh
./examples/test_httpserver.sh
```

Optional port: **`./examples/test_httpserver.sh 19900`**.

**`test_httpserver.sh`** order: **`http_smoke_gateway`**, **`http_e2e_scheduler_api`** (REST CRUD plus on-disk **`$FOXXYCODE_HOME/scheduler/*.md`**), **`http_e2e_models`**, **`http_e2e_web`**, **`http_e2e_todo`**, **`http_e2e_memory`**, **`http_e2e_skills_slash`**, **`http_e2e_background`**, **`http_e2e_subagents`** (trust route, `spawn_agent` run as an `agent` task, read-only child transcript), **`http_e2e_toolcalls_persist`**, **`http_e2e_compact`**, **`http_e2e_compact_auto`** (a model without `max_context_tokens` compacts at the window its provider reports, the one the web UI ring shows; SKIP without a NeuralDeep key), **`http_e2e_compact_clients`** (three clients of one session read the context usage fall after every kind of compaction; SKIP without a key), **`http_e2e_scheduler_agent`**, **`http_e2e_plan_files`** (plan mode **`plan_write`** to **`plans/e2e-plan.plan.md`**, then **`metadata.runPlanSlug`**), **`http_e2e_ask_mode`** (**`model: ask`** reads the note and refuses the write, **`model: agent`** on the same session writes it), **`http_e2e_config`** (staged uci-like config edit, confirm-commit, rollback from the snapshot; the server config ends the script unchanged), **`http_e2e_login`** (the web UI sign-in: the form from the environment and from `foxxycode serve set-password`, an anonymous browser refused, the cookie opening the API and the event stream, a cross-site write refused, sign-out, a bearer client unaffected). All steps run every time and need a working models backend where the LLM is called.

Docker-only smoke:

```bash
./examples/httpserver/docker.sh
```

## ACP stdio

```bash
./examples/build_foxxycode.sh
./examples/test_acp.sh
```

Order: **`acp_smoke_gateway`**, **`acp_e2e_models`**, **`acp_e2e_web`**, **`acp_e2e_todo`**, **`acp_e2e_skills_slash`**, **`acp_e2e_config`** (staged config edit into a temp config copy, confirm-commit, rollback), **`acp_e2e_memory`**, **`acp_e2e_background`**, **`acp_e2e_subagents`** (`foxxycode agents trust` then a `spawn_agent` run), **`acp_e2e_toolcalls_persist`**, **`acp_e2e_compact`**, **`acp_e2e_scheduler_agent`**, **`acp_e2e_plan_files`** (plan file on disk plus run via **`_meta.foxxycode.dev/runPlanSlug`**), **`acp_e2e_ask_mode`** (**`session/set_mode`** **`ask`**: read-only tool calls and no artifact, then **`agent`** writes it).

Environment overrides: **`FOXXYCODE_BIN`**, **`FOXXYCODE_CONFIG`**, **`SESSION_ROOT`**, **`SESSION_ID`**, **`BASE_URL`**, **`MODEL`**, etc. (see each script docstring).

## Single demos

```bash
export FOXXYCODE_BIN="$PWD/build/foxxycode"
export BASE_URL="http://127.0.0.1:19876/v1"
export FOXXYCODE_HOME=...   # for http_e2e_scheduler_api when not using test_httpserver.sh
export WORK_DIR=...
python3 examples/httpserver/http_smoke_gateway.py
```

**`http_e2e_scheduler_agent.py`** expects an already running **`foxxycode http`** and **`BASE_URL`**, **`FOXXYCODE_HOME`**, **`WORK_DIR`** matching that process (as set by **`test_httpserver.sh`**).
