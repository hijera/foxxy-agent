# Editors

FoxxyCode reaches an editor two ways. The IntelliJ plugin and the VS Code extension of this repository run the agent next to the project and show the whole web UI in a panel; any other editor, and those two as well, can drive `foxxycode acp` over the Agent Client Protocol.

## The IntelliJ and VS Code plugins

The plugin (`editors/intellij`, every IntelliJ-based IDE from 2022.2) and the extension (`editors/vscode`) bundle the `foxxycode` binary, so an editor panel needs no separate install; both are attached to every [GitHub Release](https://github.com/hijera/foxxy-agent/releases). Each starts one `foxxycode http` for the open project on a free loopback port, with `--cwd` set to the project root and the shared `~/.foxxycode` as its home, and embeds the [web UI](web-ui.md) in a tool window (JCEF) or a webview. Everything the web UI does is there - sessions and History, modes and models, settings, skills, the scheduler - plus what only an editor can give it:

- the open files and the active editor, sent to the agent with every prompt (`<foxxycode_ide_context>`), and the output of the IDE terminals behind `@terminal`;
- files dragged from the project tree into the composer as `@` mentions;
- the agent's file edits as native inline diffs with Accept, Reject and checkpoints, driven by the `/foxxycode/ide/*` routes;
- inline code completion in the editor when `autocomplete` is on ([config reference](../reference/config.md#autocomplete)).

The panel talks to its own loopback server, so it shares sessions with the console and the browser through the home directory, not through the network. Build and packaging are in [editors/intellij/README.md](../../editors/intellij/README.md) and [editors/vscode/README.md](../../editors/vscode/README.md); how the JCEF panel hosts the SPA is in [Embedding the UI in IntelliJ](../contributing/intellij-embedding.md).

## ACP

FoxxyCode is an ACP server. `foxxycode acp` speaks the [Agent Client Protocol](https://agentclientprotocol.com/) over its own stdin and stdout, so an editor, a plugin or a script that implements the client side of ACP starts the binary and gets the same agent as the console and the web UI: the same modes and models, the same permission policy, the same skills, and the same session bundles under `$FOXXYCODE_HOME`. Nothing in FoxxyCode is editor-specific - Zed, VS Code and a Python harness connect the same way.

## What ACP is

ACP standardises how a client talks to an agent process: the client spawns the agent, writes newline-delimited JSON-RPC requests to its stdin and reads responses and `session/update` notifications from its stdout; stderr carries logs, never protocol. FoxxyCode implements the agent side - `initialize`, `session/new`, `session/load`, `session/list`, `session/prompt`, `session/cancel`, `session/set_mode` and `session/set_config_option` - and calls back into the client for `session/request_permission`, `session/request_question`, `fs/read_text_file` and `fs/write_text_file`. `initialize` advertises `loadSession` whenever a session store is configured (always, for `foxxycode acp`) and an empty `authMethods` list: there is no login step.

The wire contract - every method, notification and payload - is in the [ACP protocol reference](../reference/acp-protocol.md). This page is about connecting a client to it.

## The command line

```bash
foxxycode acp [flags]
```

| Flag | Meaning |
|---|---|
| `--home DIR` | agent state directory (`FOXXYCODE_HOME`, default `~/.foxxycode`): config, sessions, skills, trust receipts |
| `--config PATH` | `config.yaml` (`FOXXYCODE_CONFIG`, else `<home>/config.yaml`) |
| `--cwd DIR` | default session working directory when the client sends an empty `cwd` (`FOXXYCODE_CWD`, default the process cwd); editors that pass a path in `session/new` use that path instead |
| `--sessions-dir DIR` | sessions root (else `sessions.dir` in the config, else `~/.foxxycode/sessions`) |
| `--session-id ID` | the next `session/new` reopens that bundle when it exists, otherwise creates a fresh one whose folder is named `ID` |
| `--mcp-project-trust ask\|allow\|deny` | policy for the workspace's `.foxxycode/mcp.json` in this process; overrides `mcp.project_trust` |
| `--remote TARGET`, `--remote-token TOKEN` | proxy the whole protocol to a remote `foxxycode serve` (see [Driving a remote server](#driving-a-remote-server)) |
| `--log-level`, `--log-output`, `--log-file`, `--log-format` | logging; the default output is stderr, and stdout must stay free for the protocol |
| `--scheduler` | run the cron scheduler in this process (needs the `scheduler` build tag) |
| `--skills-auto-discovery=false` | switch off the model-driven `load_skill` tool for this process |
| `-t` / `--test-config`, `--dry-run` | check the config (and probe what it points at) and exit; see [Configuration](../getting-started/configuration.md) |

Configure clients with the **absolute path** to the binary rather than relying on `PATH`: some harnesses spawn the agent through `cmd /c` or `sh -c` without the user's `PATH`. The one-line installer puts the binary in `~/.local/bin/foxxycode` on Linux and macOS and in `%LOCALAPPDATA%\Programs\foxxycode\foxxycode.exe` on Windows (see [Install](../getting-started/install.md#windows)). Every extra flag, such as `--home` or `--remote`, goes into the client's argument list after `acp`.

## Zed

Zed connects to external agents through its `agent_servers` setting. Open **Agent Settings**, go to **External Agents**, click **Add Agent** and choose **Add Custom Agent**: Zed opens `settings.json` with an `agent_servers` entry to fill in. Add one entry for FoxxyCode:

```json
{
  "agent_servers": {
    "FoxxyCode": {
      "type": "custom",
      "command": "/home/you/.local/bin/foxxycode",
      "args": ["acp"]
    }
  }
}
```

An optional `env` object on the entry sets variables for the process - `FOXXYCODE_HOME` when the editor should use a home other than `~/.foxxycode`, or a provider key that is not in your login environment. Zed picks the change up without a restart; pick **FoxxyCode** under *External Agents* in the agent panel's new-thread menu.

FoxxyCode's own modes (`agent` / `plan` / `docs` / `ask` / `debug`), its configured models and its permission policy appear in Zed's composer, and its skills show up as slash commands. A permission prompt is Zed's own approval dialog: the request carries the options FoxxyCode offers, and the answer Zed sends is read in the nested form the protocol prescribes.

## VS Code

The FoxxyCode extension above does not use ACP. Any ACP client extension works for `foxxycode acp`, for example the **ACP Client** extension (`formulahendry.acp-client`). Point the extension at the binary the same way as Zed: the command is the absolute path of `foxxycode`, the arguments are `acp` plus any flags. What the extension's panel can show is what the protocol carries - the mode, model and permission-mode selectors that `session/new` returns, the tool calls and file changes of a turn, the slash commands, and the sessions that `session/list` and `session/load` give it. Where the command is configured belongs to the extension; FoxxyCode only sees the process it is spawned as.

## Obsidian and other clients

FoxxyCode's README lists Obsidian among the clients. There is no Obsidian-specific code in the repository: a plugin or an application that implements the client side of ACP connects like any other client - it spawns `foxxycode acp` and speaks JSON-RPC over the pipe. If a client can run a command and read its stdout line by line, the rest is the protocol reference.

## Scripts

`examples/acp/acp_e2e_todo.py` is the reference client: a newline-delimited JSON-RPC harness that drives a real turn and checks the todo list, the tool calls and the files the agent wrote. Use it as the template for a client of your own rather than chaining `echo` lines into a pipe. Four behaviours a hand-written client must have, from the [Stdio clients](../reference/acp-protocol.md#stdio-clients-foxxycode-specific) section of the reference:

- a success response may omit `result` when the handler returns nothing (`session/set_mode` does): treat any line with your request `id` and no `method` as the completion;
- after `session/prompt` many `session/update` notifications arrive before the final response; read line by line until your `id` comes back, and answer `session/request_permission` and `session/request_question` in between with a response carrying the same `id`;
- when stdout is not a terminal it may be block-buffered: wrap the binary in `stdbuf -oL -eL` or equivalent, as the harness does;
- outstanding requests are dispatched asynchronously; do not send the next RPC before consuming the previous response if your client assumes ordering.

The whole ACP suite runs with `./examples/build_foxxycode.sh && ./examples/test_acp.sh` against `examples/config.demo.yaml`; `FOXXYCODE_BIN`, `FOXXYCODE_CONFIG`, `SESSION_ROOT` and `SESSION_ID` override what each script uses (see `examples/README.md`). A one-line sanity check needs no script at all:

```bash
echo '{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":1,"clientCapabilities":{}}}' | foxxycode acp
```

## What the editor shows

Everything an editor renders about FoxxyCode comes out of the protocol, so every client gets the same set:

| In the client | Where it comes from |
|---|---|
| Mode selector (`agent`, `plan`, `ask`) | the `mode` config option of `session/new` and `session/load` (plus the legacy `modes` field); changed with `session/set_config_option` or `session/set_mode` |
| Model selector | the `model` config option, one row per `models[].model` in `config.yaml`, default `agent.model`; absent when the list is empty |
| Permission policy (`ask`, `accept_edits`, `bypass`) | the `permission_mode` config option; an override is session-scoped, persisted in `session.json`, and wins over `tools.permission_mode` |
| Slash commands | `available_commands_update` after `session/new` and `session/load`: the built-ins (`compact`, `export`, `plugin`) first, then every skill from `skills.dirs` |
| Permission prompts | `session/request_permission` with the options Allow, Allow always and Reject (and the program-wide `allow_always_<program>` for a plain shell command); sent only under `ask` (commands and writes) or `accept_edits` (commands); `bypass` never asks |
| Questions | `session/request_question` from the `question` tool; the client answers with one array of choices per question |
| Tool calls, plan checklist, token and account usage | `tool_call` / `tool_call_update`, `plan`, `token_usage`, `provider_usage` updates |
| Session list and resume | `session/list` (optional `cwd` filter; `sessionId`, `cwd`, `title`, `updatedAt` per row) and `session/load`, which replays the transcript and then re-sends the command catalog |

When the agent delegates to a subagent, the child's own updates never reach the editor: they go to the background task's log. The one thing a client receives on a child's behalf is a permission prompt, delivered under the parent's `sessionId` with a title prefixed `[subagent <name>]`. MCP servers the client itself declares in `session/new` are connected as declared; servers from the workspace's `.foxxycode/mcp.json` go through the trust gate first (see [Security and trust](../operate/security.md)).

## Sessions shared with the console and the web UI

`foxxycode acp` stores every session as a bundle under `$FOXXYCODE_HOME/sessions/<sessionId>/` - `session.json`, `messages.json`, `assets/`, `todos/active.md`, `plans/` - in the same store the console and `foxxycode serve` open. With the same home directory a thread started in Zed is a row in `foxxycode sessions list`, in the web UI's history and in the console's `/resume` picker, and can be continued from any of them; the editor sees the others' sessions through `session/list`. The config file, the skills, the rules, the MCP servers, the hooks and the subagent definitions are shared the same way, because they are read from that home and from the session's working directory.

The only requirement is that the processes agree on the home: an editor started from a launcher may not inherit the `FOXXYCODE_HOME` your shell exports, so either set it in the client's `env` block or pass `--home` in the arguments. `--sessions-dir` and `sessions.dir` move the store for every surface alike. `foxxycode acp --session-id <id>` reopens a bundle from the command line; `session/load` does the same from the client.

## Driving a remote server

```bash
foxxycode acp --remote nas02 --remote-token "$FOXXYCODE_REMOTE_TOKEN"
```

With `--remote` the process keeps no store, runs no scheduler and no agent loop: every session call is proxied to a remote `foxxycode serve`, its streamed answer is translated back into ACP updates, and permission and question prompts are answered through the server's REST routes. The target is a configured remote name, a `host:port` or an `http(s)://` URL; the token comes from `--remote-token` or `FOXXYCODE_REMOTE_TOKEN` and is never read from `config.yaml`. Sessions persist on the server only, `--session-id` still names the bundle to reopen there, and the model selector lists the server's catalog. An editor entry for a remote server is the same entry with the two flags appended to `args`. Details, the token and CORS: [Remote mode](../operate/remote.md).

## Troubleshooting

- **The editor cannot start the agent.** Use the absolute path of the binary in the client configuration; `PATH` is often not inherited. On Windows the installer's path is `%LOCALAPPDATA%\Programs\foxxycode\foxxycode.exe`.
- **The agent starts and nothing comes back.** Run `foxxycode acp -t` to check the config, then `foxxycode acp --dry-run` to probe the providers, the MCP commands, the paths and, with `--remote`, the server. Both print each problem with its line and exit without starting anything.
- **You need to see what the process does.** Add `--log-level debug` to the arguments; logs go to stderr by default, and `--log-output file --log-file <path>` writes them to a file instead. Never send logs to stdout, which the client reads as protocol.
- **A project MCP server does not start.** It is waiting for approval: `foxxycode mcp list` shows it, `foxxycode mcp trust <name>` approves it, or start the process with `--mcp-project-trust allow` for a checkout you trust.
- **The editor and the shell see different sessions.** The two processes use different homes; set `FOXXYCODE_HOME` in the client's `env` block or pass `--home`.
- **The client's permission answer is ignored.** FoxxyCode reads both the nested outcome editors send and the flat form; a `cancelled` outcome or the `reject` option denies the call, anything else allows it. Check which `optionId` the client returns.

More general failures - the binary missing from `PATH`, a provider rejecting the key, a busy port - are in [Troubleshooting](../getting-started/troubleshooting.md).
