# Quickstart

This page takes a fresh machine to a first answer: install the binary, give it one model, check the file, then talk to the agent from a terminal, from a browser and from an editor panel. Every other page assumes these steps were taken. Budget five minutes and one API key.

## 1. Install

Download the archive for your platform from [GitHub Releases](https://github.com/hijera/foxxy-agent/releases) (`foxxycode_X.Y.Z_linux_amd64.tar.gz`, `..._darwin_arm64.tar.gz`, `..._windows_amd64.zip` and so on), unpack it and put `foxxycode` on `PATH` (`~/.local/bin` on Unix, `%LOCALAPPDATA%\Programs\foxxycode` on Windows). The `coddy.dev` one-line installers install upstream coddy-agent, not this fork; the [install guide](install.md) has the Linux packages, Homebrew and manual placement. Check the binary from a new terminal:

```bash
foxxycode -v
```

Working inside an editor? The IntelliJ plugin and the VS Code extension bundle the binary and need none of this - see step 6.

Debian, Ubuntu, Fedora and their relatives can take the `.deb` or `.rpm` instead, and a container runs the same binary as `foxxycode serve` ([Docker](docker.md)). If the command is not found, see [Troubleshooting](troubleshooting.md#the-binary-is-not-found-after-the-install).

## 2. Give it a model

Everything FoxxyCode needs to know lives in `~/.foxxycode/config.yaml` (`%USERPROFILE%\.foxxycode\config.yaml` on Windows). Create it with three blocks, the minimum for an OpenAI-compatible provider ([config.example.yaml](../../config.example.yaml) in the repository explains every section in comments):

```yaml
providers:
  - name: openai
    type: openai
    api_key: "${OPENAI_API_KEY}"

models:
  - model: "openai/gpt-5.6-terra"
    max_tokens: 8192
    reasoning_default: medium

agent:
  model: "openai/gpt-5.6-terra"
  max_turns: 30
  max_tokens_per_turn: 200000
```

Three rules hold the blocks together. A provider's `name` becomes the prefix of every model id, so `models[].model` is `<provider name>/<model id as the API knows it>`. `agent.model` has to be one of the `models` entries. And `api_key` is either the secret itself, a `${VAR}` reference expanded when the file loads, or empty, in which case `OPENAI_API_KEY` (the provider name in upper case plus `_API_KEY`) is read at call time.

Put the key where the reference finds it, in the shell or in `~/.foxxycode/.env`, which is read before the file is parsed and never overrides a variable the shell already set:

```bash
export OPENAI_API_KEY="sk-..."
# or, once and for all:
echo 'OPENAI_API_KEY=sk-...' >> ~/.foxxycode/.env
```

Any OpenAI-compatible server works with `type: openai` and an `api_base` that includes `/v1`; a local Ollama or llama.cpp needs no real key:

```yaml
providers:
  - name: local
    type: openai
    api_base: "http://localhost:11434/v1"
    api_key: "~"
```

Anthropic, NeuralDeep (`foxxycode providers login neuraldeep` instead of a key) and Codex (ChatGPT OAuth) are covered in [Configuration](configuration.md).

## 3. Check the file

Two flags tell you whether the file is right before anything starts, and every command that loads the file takes them:

```bash
foxxycode -t          # the file against the schema and the loader's rules
foxxycode --dry-run   # the same, then the world it points at
```

`-t` prints `<path>: valid` or every problem as `file:line:column: what is wrong` with a `fix:` line under it, and exits 1 on errors. `--dry-run` goes on to ask each provider for its model list, which exercises the address and the credential in one request, and checks every `models[]` entry against that list; a clean setup answers with one line, `dry run: 0 errors, 0 warnings, N ok`. Details: [Checking the file](configuration.md#checking-the-file-from-the-command-line) and [Dry run](configuration.md#dry-run-probing-what-the-file-points-at).

## 4. The console

In a project directory, run the bare command:

```bash
cd ~/src/my-project
foxxycode
```

That opens the interactive console: type a request, press Enter, and watch the tool boxes, the thinking block and the answer stream in. Under the default permission mode the agent asks before it runs a command or writes a file. Escape interrupts a turn, `/quit` (or Ctrl-C twice) leaves, and `foxxycode -c` reopens the latest session of this folder.

The same agent runs without a terminal, for a script or a cron job:

```bash
foxxycode -p "Explain in three sentences how tasks are saved in this repository"
foxxycode --mode ask -p "Which files read config.yaml?"
```

`-p` streams the answer to stdout, sends diagnostics to stderr, exits non-zero on an error and persists the turn as a normal session; `--mode ask` keeps it read-only. Everything the console can do is in [Console (TUI)](../surfaces/console.md).

## 5. The browser

```bash
foxxycode serve
```

```text
foxxycode serve X.Y.Z
  httpserver  http://127.0.0.1:12345  (no auth)
```

Open **http://127.0.0.1:12345/**. The empty screen is a new chat: pick a mode and a model in the composer, type, send. Sessions live under the same `~/.foxxycode` whichever surface created them, so the console and the browser share one history. Settings live at `#/settings`, the OpenAI-compatible API at `/v1/*` and its Swagger UI at `/docs/`.

The server binds loopback on purpose. To reach it from another machine, bind wider and require a token: `foxxycode serve -H 0.0.0.0 --auth-token <secret>` (or `httpserver.host` and `httpserver.auth_token` in the file). `foxxycode serve --daemon` keeps it running in the background and `foxxycode serve status | stop | restart` control it; see [foxxycode serve and the daemon](../operate/serve.md). On Windows, `foxxycode desktop` opens the same UI in a native window instead of a browser tab.

## 6. An editor panel

Install the FoxxyCode plugin into an IntelliJ-based IDE (**Settings | Plugins | Install Plugin from Disk**) or the extension into VS Code (**Extensions: Install from VSIX**), both from the release assets, and open a project. The panel starts its own `foxxycode http` for that project and shows the web UI of step 5, with the open files, the IDE terminals and native inline diffs wired in; its first run walks through the same model setup as step 2 when `config.yaml` has no model yet. Details: [Editors](../surfaces/editors.md#the-intellij-and-vs-code-plugins).

## 7. An editor over ACP

`foxxycode acp` speaks the Agent Client Protocol over stdio, which is what Zed, VS Code, Obsidian and scripts use. For Zed, add one entry to `settings.json` with the absolute path of the binary (the one `which foxxycode` prints; editors do not always inherit your `PATH`):

```json
"agent_servers": {
  "FoxxyCode": {
    "type": "custom",
    "command": "/home/you/.local/bin/foxxycode",
    "args": ["acp"]
  }
}
```

Then pick **FoxxyCode** under *External Agents* in the agent panel's new-thread menu. FoxxyCode's modes (`agent`, `plan`, `docs`, `ask`, `debug`), its configured models and its permission policy appear in Zed's composer, and its skills show up as slash commands. The sessions are again the ones in `~/.foxxycode`. VS Code, other ACP clients and the reference script client are in [Editors](../surfaces/editors.md); the protocol itself is in [ACP protocol](../reference/acp-protocol.md), with runnable examples under `examples/acp/`.

## Where next

- [Configuration](configuration.md) - every provider type, the `.env` file, the environment variables and SSH remote execution;
- [Operating modes](../features/modes.md) - what agent, plan, docs, ask and debug allow and how to switch on each surface;
- [Skills](../features/skills.md), [Rules](../features/rules.md) and [MCP servers](../features/mcp.md) - how a project teaches the agent its conventions and tools;
- [Telegram gateway](../surfaces/gateway.md) - the same sessions from a messenger;
- [Update](update.md) - `foxxycode update`, and what happens when a package manager owns the binary;
- [Troubleshooting](troubleshooting.md) - when a step above did not go as written.
