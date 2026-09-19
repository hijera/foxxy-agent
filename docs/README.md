# FoxxyCode documentation

FoxxyCode is a general-purpose agent in one static Go binary: a ReAct loop with filesystem and shell tools, MCP servers, project rules, skills, subagents, hooks, background tasks, a cron scheduler and long-term memory, driven from a terminal console, an embedded web UI with an OpenAI-compatible HTTP API, the IntelliJ and VS Code plugins, a desktop window on Windows, editors over the Agent Client Protocol, or a Telegram bot. Every surface shares the same sessions under `~/.foxxycode`. FoxxyCode is a fork of [coddy-agent](https://github.com/coddy-project/coddy-agent); what it adds is in [FoxxyCode and coddy-agent](getting-started/foxxycode-and-coddy.md).

New here? Read [Quickstart](getting-started/quickstart.md), then the page of the surface you use. This map is generated from [nav.yaml](nav.yaml) by `make docs`; the same list feeds [llms.txt](llms.txt) and [llms-full.txt](llms-full.txt) for agents that read documentation. How the pages are organised and what a change to FoxxyCode must carry into them is in [Writing documentation](contributing/documentation.md).

<!-- docsgen:nav:start -->
## Getting started

Install FoxxyCode, give it a model, run it for the first time and keep it updated.

- [Quickstart](getting-started/quickstart.md) - From a fresh install to the first answer in five minutes, on the console, in the browser and from an editor.
- [FoxxyCode and coddy-agent](getting-started/foxxycode-and-coddy.md) - What this fork adds over upstream coddy-agent - the IDE plugins, the desktop window, the browser tool and the rest - and how it follows upstream.
- [Install](getting-started/install.md) - Release archives, Linux .deb and .rpm packages, Homebrew, the IntelliJ and VS Code plugins, Windows paths, manual placement.
- [Configuration](getting-started/configuration.md) - Where config.yaml lives, how to check it with -t and --dry-run, providers and models, SSH remote execution, the .env file.
- [Update](getting-started/update.md) - foxxycode update, release assets, installations owned by a package manager, the report of what changed.
- [Docker](getting-started/docker.md) - The GHCR image, docker compose, volumes and environment, the bundled UI on port 12345.
- [Homebrew](getting-started/homebrew.md) - The cask against the formula, which Homebrew repository takes what, the homebrew/core submission.
- [Troubleshooting](getting-started/troubleshooting.md) - What to check when the binary is not on PATH, the config does not load, a provider rejects the key, a port is busy or a surface is missing from the build.

## Surfaces

The same agent and the same sessions from a terminal, a browser, an editor or a messenger.

- [Console (TUI)](surfaces/console.md) - Bare foxxycode in a terminal, the layout, keys, slash commands, the local shell, print mode and remote mode.
- [Web UI](surfaces/web-ui.md) - The embedded single-page app served by foxxycode serve and foxxycode http, with sessions, the composer, modes and models, attachments, settings, themes and languages.
- [Editors](surfaces/editors.md) - The IntelliJ and VS Code plugins that run foxxycode http, and Zed, VS Code, Obsidian and scripts as ACP clients of foxxycode acp, and what they share with the other surfaces.
- [Telegram gateway](surfaces/gateway.md) - The Telegram bot adapter, setup, access levels, session isolation, rich messages, the same chat live in the browser, writing a new adapter.

## Operate

Running FoxxyCode as a service, reaching it from elsewhere and bounding what it may do.

- [foxxycode serve and the daemon](operate/serve.md) - One process for every enabled subsystem, --daemon with status, stop and restart, how a configuration change reaches a running process.
- [Remote mode](operate/remote.md) - Driving a remote foxxycode serve from the console, ACP or the web UI with --remote, tokens, CORS and the environment chip.
- [Swarm](operate/swarm.md) - Relays and nodes, mounts, the aggregated session list, rings and routes, the reverse tunnel.
- [Scheduler](operate/scheduler.md) - Cron job files, UTC firing rules, run sessions, the scheduler tools and REST.
- [Security and trust](operate/security.md) - What the agent may execute and how to bound it, permission modes, project trust for MCP servers, hooks and subagents, tokens and CORS, what is not sandboxed.
- [Diagnostics](operate/debugging.md) - The opt-in debug layer - raw LLM capture, the per-session trace and the loop events - and how to read them when a turn goes wrong.

## Features

What the agent can do and how each capability is configured.

- [Operating modes](features/modes.md) - agent, plan, docs, ask and debug, which tools each mode allows and how to switch on every surface.
- [Sessions](features/sessions.md) - Session bundles on disk, resuming, branches from an edited message, todo lists, the sessions CLI.
- [Rules and instructions](features/rules.md) - Rules from the .foxxycode, .agents, .cursor, .claude and .codex folders, AGENTS.md and DESIGN.md of a folder the agent enters, your own pair and rules in the agent home, instruction files, dialects by extension, activation.
- [Skills](features/skills.md) - SKILL.md packs as slash commands, skills.dirs, the registries and the plugin command.
- [Subagents](features/subagents.md) - spawn_agent, definition files, the built-ins, project trust receipts, capability narrowing, child sessions.
- [Hooks](features/hooks.md) - Lifecycle hooks in Claude Code's hooks.json shape, events, matchers, the stdin payload, exit codes and JSON answers, project trust.
- [MCP servers](features/mcp.md) - Connecting MCP servers over stdio, streamable HTTP and SSE, mcp.json files, workspace trust, enable switches, the management API.
- [Browser tool](features/browser-tool.md) - The interactive browser the agent drives through Chrome, screenshots handed back to the model, the browser switch and the build tag.
- [Message queue](features/message-queue.md) - Writing a follow-up while the agent works, when the running turn reads it, taking one back, the composer and console surfaces, the queue routes.
- [Background tasks](features/background-tasks.md) - Detached commands, the task pool, timeouts, adoption of long foreground commands, program-wide permission grants.
- [Context compaction](features/compaction.md) - /compact and automatic summarisation at a threshold, the kept recent turns, result eviction with keep_result.
- [Long-term memory](features/memory.md) - The memory copilot, what it recalls and saves, the storage layout, configuration and cost.
- [Session export](features/session-export.md) - /export and foxxycode sessions export, formats, path rules, trimming options, the JSON document.

## Reference

Complete lists, generated from the code wherever the code is the source of truth.

- [CLI reference](reference/cli.md) - Every command, verb and flag of the foxxycode binary, generated from --help.
- [config.yaml reference](reference/config.md) - Every field of config.yaml with its type, default and description, generated from the schema, plus the self-configuration tools.
- [Environment variables](reference/environment-variables.md) - Every variable the binary reads, what it overrides and where it is documented.
- [Slash commands](reference/slash-commands.md) - The built-in commands on each surface and how skills become commands.
- [Keyboard](reference/keyboard.md) - The keys of the console TUI and of the web UI composer.
- [Tools](reference/tools.md) - The built-in tools the model can call, their arguments, permissions and the modes that expose them.
- [HTTP API](reference/http-api.md) - The OpenAI-compatible endpoints and the /foxxycode REST surface, authentication, sessions and headers, the OpenAPI document.
- [ACP protocol](reference/acp-protocol.md) - How foxxycode acp implements the Agent Client Protocol, methods, notifications, permission and question requests.

## Tutorials

Task-shaped guides, each a complete path from a goal to a working result, with the configs and commands to copy, the check that it worked and what tends to go wrong.

- [FoxxyCode in CI](tutorials/foxxycode-in-ci.md) - A workflow file that runs foxxycode -p on every pull request, with the key in a secret and the answer in the job log.
- [A Telegram bot for a team](tutorials/telegram-bot-for-a-team.md) - One bot for a team with foxxycode serve: the token, who may talk to it, group isolation, and the same chat live in the browser.
- [A project set up for agents](tutorials/project-set-up-for-agents.md) - A repository that teaches any agent how it works: rules, AGENTS.md, skills, an MCP server and the trust receipts.
- [A server driven from a laptop](tutorials/server-driven-from-a-laptop.md) - A foxxycode serve on a machine with the models and the workspace, driven from a laptop over the console, an editor and the browser.
- [FoxxyCode as a model in VS Code Copilot](tutorials/foxxycode-as-a-model-in-vs-code.md) - A running foxxycode serve registered in chatLanguageModels.json, so its models and its agent sit in Copilot's model picker, with sessions per request and the agent's permission gate.
- [A skill of your own](tutorials/a-skill-of-your-own.md) - A SKILL.md that becomes a slash command on every surface, from the first file to a registry install.
- [A relay and its nodes in Docker](tutorials/swarm-relay-and-nodes.md) - A Compose stand with a relay and nodes that dial out to it, the tokens, the checks, a mounted node from the console and the browser, scaling by adding nameless workers.
- [A chain of relays](tutorials/swarm-multi-hop.md) - A second relay behind the first with its own nodes, two-hop mounts, the recursive session list, rings and alternates.
- [Working with remote nodes](tutorials/swarm-remote-nodes.md) - Driving a node behind a relay from the console, an editor and the browser, what runs where, and which credential opens what.

## Contributing

How FoxxyCode is built, tested, documented and released.

- [Contributing guide](../CONTRIBUTING.md) - The development environment, the branch and pull request flow, what every change must carry.
- [Writing documentation](contributing/documentation.md) - Page types, the navigation map, screenshots, the assets index, generated references and the checks that guard them.
- [Brand assets](contributing/brand.md) - The one vector every icon and the social preview are generated from, make brand, and the lock file that fails when an export goes stale.
- [Architecture](contributing/architecture.md) - System design and component overview, package boundaries, session modes, the directory structure.
- [Build from source](contributing/build.md) - Prerequisites, make build, TAGS against go build -tags, the release binaries and the distribution packages.
- [Custom tools](contributing/custom-tools.md) - Adding a built-in tool to the registry, its schema and permission wiring, with a complete example.
- [ReAct agent](contributing/react-agent.md) - The loop design, the system prompt structure, the tool-calling contract and mode-specific behaviour.
- [Web UI design](../DESIGN.md) - Tokens, layout and component contracts of the embedded SPA.
- [Embedding the UI in IntelliJ](contributing/intellij-embedding.md) - How the JCEF panel hosts the SPA - the Chromium 104 baseline, the embed markers, the theme bridge and the window API the plugin calls.
- [The website](contributing/website.md) - The site on GitHub Pages - React pages in English and Russian, data baked from the releases and the changelog twins, the comparison, the screenshots, and the workflow that publishes it together with docs/.
- [Release setup](contributing/release-setup.md) - How releases of this fork are published on GitHub - the workflows, the tags and the artefacts, the desktop build included (in Russian).
- [Codex hooks](contributing/codex-hooks.md) - How the Cursor rules reach a Codex CLI session working on this repository.
- [OpenCode hooks](contributing/opencode-hooks.md) - Deterministic delivery of the Cursor rules to OpenCode sessions working on this repository.
- [ZCode hooks](contributing/zcode-hooks.md) - Deterministic delivery of the Cursor rules to ZCode sessions working on this repository.
- [Syntax highlighting audit](contributing/syntax-highlighting-audit.md) - The NeuralDeep audit of code highlighting across the seven themes, its fixtures and how to re-run it.
- [Agent notes](../AGENTS.md) - The repository map and contributor notes for coding agents.
<!-- docsgen:nav:end -->
