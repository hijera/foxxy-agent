# Slash commands

The built-in commands on each surface and how skills become commands. A slash command is one of five things: a settings command (`/model`, `/reasoning`, `/think`, `/nothink`, `/agent`, `/plan`, `/ask`, `/debug`, `/permissions`), which the session manager takes off the start of a prompt and applies before any turn starts; a client-side command of the console, which never leaves the terminal; a deterministic built-in (`/compact`, `/export`, `/plugin`), which the agent recognises before the prompt becomes a message and runs without a turn of the model; a Telegram bot command, handled by the adapter; or a skill, whose body is prepended to the message for the model. `/docs` remains the built-in documentation reader in the console and web UI; the `docs` editing mode is selected through `/mode docs` or a Mode picker. The first four run whatever the session mode is, because they are operator input rather than tool calls.

## The commands

| Command | Surfaces | What it does | Details |
|---|---|---|---|
| `/model <id> [--once\|--count=N]` | console, web UI, ACP editors, HTTP, Telegram | Switches to a configured model for the session or the next turns. A bare command opens a selector in the console and web UI, or Telegram's keyboard. | [Session settings](../features/session-settings.md#the-commands) |
| `/reasoning <level\|off\|default>`, `/effort`, `/think`, `/nothink` | console, web UI, ACP editors, HTTP, Telegram | Changes the reasoning level for the session or the next turns. A bare `/reasoning` opens a picker in the console and web UI. | [Session settings](../features/session-settings.md#thinking-off) |
| `/agent`, `/plan`, `/ask`, `/debug` | console, web UI, ACP editors, HTTP, Telegram | Selects an operating mode for the session or the next turns. | [Session settings](../features/session-settings.md#the-commands) |
| `/permissions <ask\|accept_edits\|bypass>` | console, web UI, ACP editors, HTTP | Changes when tools ask for approval until process restart; unavailable as a Telegram bot command. | [Session settings](../features/session-settings.md#switching-from-the-permission-dialog) |
| `/mode [agent\|plan\|docs\|ask\|debug]` | console; Telegram | Console: opens the mode selector, or switches directly when a valid mode follows. Telegram: an inline keyboard. | [Operating modes](../features/modes.md#switching-on-each-surface) |
| `/resume [id or title]` | console; Telegram | Console: a picker over the sessions of the current folder; the chosen one replaces the current session. Telegram: an inline keyboard over every session the server keeps, or, with words after it, the session whose id or title they name. | [Sessions](../features/sessions.md#resuming), [Telegram gateway](../surfaces/gateway.md#commands) |
| `/new` | console | Starts a new session in the same folder. | [Console](../surfaces/console.md#commands-and-keys) |
| `/theme` | console | Selector between the dark and the light palette. | [Console](../surfaces/console.md#flags) |
| `/hotkeys` | console | Prints the key list. | [Keyboard](keyboard.md) |
| `/queue [list\|drop <n>\|clear]` | console | Lists the follow-ups waiting for the running turn, takes one back by its number, or empties the queue. Works against a remote server too. | [Message queue](../features/message-queue.md#in-the-console) |
| `/usage` | console | Forces a fresh read of the active provider's account usage and prints the breakdown; under `--remote` the server's own key is read. | [Console](../surfaces/console.md#commands-and-keys) |
| `/tasks` | console | Opens the background tasks of the session in the place of the editor: what runs, what finished, a task's output on **enter**, **s** to stop one. Under `--remote` it lists and stops the processes of the server. | [Background tasks](../features/background-tasks.md#in-the-console) |
| `/docs [words or page]` | console, web UI | Opens FoxxyCode's built-in documentation, the same screen F1 opens: the contents, a search for the words, or a page (`/docs features/mentions#completion`) opened at its section. A page is named by its address or its exact title; any other words are a search. In the web UI the composer runs it in the browser and nothing reaches the agent. `/help` is the same command in the console. | [Built-in documentation](../features/built-in-docs.md#the-console-help) |
| `/quit`, `/exit` | console | Exits. | [Console](../surfaces/console.md#commands-and-keys) |
| `/compact [--model <id>] [instructions]` | console, web UI, ACP editors, `POST /v1/responses` | Summarises the older history and keeps the recent turns verbatim; the instructions steer the summariser; `--model` selects one configured model for this call on either engine. Listed only while `compaction.enable` is true; a manual compaction is always forced. | [Context compaction](../features/compaction.md#the-compact-command) |
| `/export [md\|html\|json\|jsonl] [path] [--no-tools] [--no-thinking]` | console, web UI, ACP editors, `POST /v1/responses` | Writes the transcript into the session workspace; a directory receives `foxxycode-export-<timestamp>.<ext>`, a file name sets the format by extension. Under `--remote` the file lands on the server. | [Session export](../features/session-export.md#usage) |
| `/plugin marketplace list\|add\|remove\|sync`, `/plugin install\|remove\|enable\|disable` | console, web UI, ACP editors, `POST /v1/responses` | Manages skill plugins and marketplaces, the chat twin of `foxxycode plugin`. | [Skills](../features/skills.md#the-plugin-command-cli-and-plugin-in-chat) |
| `/start`, `/help` | Telegram | The greeting and the command list of the bot. | [Telegram gateway](../surfaces/gateway.md#commands) |
| `/context` | Telegram | The context window usage of the chat's session by category. | [Telegram gateway](../surfaces/gateway.md#commands) |
| `/clear` | Telegram | Starts a new session for the chat; the old one stays on disk, and `/resume` brings it back. | [Telegram gateway](../surfaces/gateway.md#session-lifecycle) |
| `/<skill>` | console, web UI, ACP editors, `POST /v1/responses` | Runs a skill: the full `SKILL.md` body is prepended to the message the model receives for this turn. | [Skills](../features/skills.md#how-skills-are-applied) |

Three boundaries follow from the code:

- the Telegram adapter answers its own seven commands, passes the settings commands but `/permissions` to the session (`isSettingsCommand` in `external/gateway/telegram/bot.go`), and drops any other message that starts with `/`, so `/compact`, `/export` and skills are not reachable there;
- a subagent never runs a built-in: a child prompt that starts with `/export` is an ordinary task for the child (`internal/agent/react.go`);
- the console's client-side commands exist only in the console; the settings commands are the same everywhere, because one parser and one setter in the session manager serve every surface.

## Where each surface gets its list

| Surface | Source of the list | Notes |
|---|---|---|
| Console | A fixed client-side list (`resume`, `new`, `theme`, `hotkeys`, `queue`, `usage`, `tasks`, `docs`, `quit`) merged with the rows the server advertises through the ACP `available_commands_update` notification, the settings commands among them (`slashCatalog` in `external/cli/app.go`). | Typing `/` opens the suggestion menu; Enter on a suggestion applies it and submits in one stroke. |
| Web UI | One paginated `GET /foxxycode/slash-commands` catalog, filtered by `prefix` and scoped to the session workspace through `X-FoxxyCode-Session-ID`: settings commands and deterministic actions lead, followed by skills. Settings rows carry an argument `hint`. `GET /foxxycode/commands` separately exposes richer metadata for other clients. | The list is re-read after `event: config_reloaded`, so an installed skill shows without reloading the page. Picking `/model`, `/reasoning` or `/permissions` opens its selector; `/agent`, `/plan`, `/ask` or `/debug` switches mode; `/docs` opens the reader. |
| ACP editors | `available_commands_update` after `session/new` and `session/load`: the settings commands, then the other built-ins, then the skills sorted by name; rows carry `name` (without the slash), `description` and, for a settings command, `input.hint` with its argument and flags. A skill named like a built-in or one of its aliases is left out. | The same function, `session.BuiltinCommandRows` in `internal/session/commands.go`, feeds the HTTP endpoint and the notification, so the two never disagree: `compact` only while compaction is enabled, `export` and `plugin` always. |
| Telegram | `start`, `help`, `model`, `agent`, `plan`, `ask`, `context`, `resume` and `clear`, registered with `setMyCommands` at startup. | They appear in the client's command menu; `/reasoning`, `/think`, `/nothink` and `/debug` work when typed but are not in the menu. `/mode` keeps the fork's five-mode picker. |

## How skills become commands

Every skill file that the loader accepts is a command:

- the identifier is derived from the file location by `skills.CanonicalCommandName`: a `SKILL.md` inside a folder takes the folder name (`skills/code-review/SKILL.md` becomes `/code-review`), and a Markdown file at the root of a `skills.dirs` entry takes its stem (`skills/deploy.md` becomes `/deploy`);
- the `name` field of the frontmatter sets the skill's display name, while the command identifier stays the path-derived one;
- when two files map to the same identifier, the first one in `skills.dirs` order wins (`skills.ListSkills`), and a skill disabled in the configuration is not loaded at all;
- the description shown next to the command is the frontmatter `description`, or the first plain line of the body when there is none, capped at 160 characters.

The catalog reaches the model too: the system prompt carries a `## Slash commands` block listing every `/name` with its description (`skills.BuildSlashCatalogMarkdown`), and with `skills.auto_discovery` on, the model can pull a full body itself through the `load_skill` tool ([Tools](tools.md)).

Five of those commands are there on a fresh install without anything being downloaded - `/configure-foxxycode`, `/rpa-init`, `/rpa-feat`, `/rpa-bugfix` and `/rpa-gen-rules`, the [standard delivery](../features/skills.md#the-standard-delivery) the binary writes into `${FOXXYCODE_HOME}/skills`.

## How a command in a prompt is parsed

The settings commands are taken first, by `session.ParseSettingsCommands`, before the turn lock and before the text becomes a message:

- only the start of the typed text counts; a settings command in the middle of a sentence is prose, and a mention's attachment or a skill body is never read;
- each command's value and its `--once`, `--count=N` or `--count N` flags are the words after its name, on its line; the flags may come before or after the value;
- commands chain, on one line or on consecutive ones (`/model x --once /nothink --once review this`), and the first word that belongs to no command starts the prompt, which is kept verbatim and may itself be `/compact`, `/export` or a skill;
- a prompt of commands only runs no turn and leaves nothing in the model's history; each change is reported as a notice;
- the names are matched case-insensitively, aliases included, and win over a skill of the same name.

The three built-ins are recognised on the whole prompt (`parseCompactCommand`, `parsePluginCommand` and `parseExportCommand` in `internal/agent`):

- after trimming, the text must be exactly `/compact`, `/plugin` or `/export`, or start with one of them followed by whitespace; a built-in in the middle of a sentence is not a command;
- the words after `/compact` are the summariser's instructions;
- the words after `/plugin` are split into the subcommand and its arguments;
- after `/export`, `--no-tools` and `--no-thinking` may appear anywhere, the first remaining word is the format when it names one, and the rest joined by single spaces is the target path;
- the command text is persisted as a user row, so the transcript shows it, and the outcome is stored as an assistant message.

Skills are found by `skills.ParseInvokedCommandNames` over the text the user typed (never over the attachments a mention brought), line by line:

- lines inside fenced code blocks and lines starting with `>` are skipped;
- on the remaining lines, the regular expression `invokedMidLineSlashRE`, `(?:^|[\t ])\/([a-zA-Z0-9][a-zA-Z0-9_-]*)`, takes every `/name` that stands at the start of the line or right after a space or a tab, so `/review` in the middle of a sentence counts, while `x/foo`, `path/to/file` and `https://` do not;
- the picker form of older SPA drafts, `[/name](foxxycode-skill:name)`, is accepted as well when both names agree;
- each name is kept once, in order of appearance, and a name with no matching skill is left alone as plain text, which is why a stray `/tmp` in a prompt is harmless;
- when the message is sent, the body of each matched skill is written into it once, as an attachment after the text (`<foxxycode_attachment path="skill:name" kind="skill">`, `invokedSkillBlocks` in `internal/agent/react.go`). A later turn replays the message with the body it was sent with, so the provider's cached prefix holds and the model keeps the instructions it was given; the transcript still shows the message as typed, the web UI and a reopened console drop the body from the bubble. A follow-up queued mid-turn invokes skills the same way ([Mentions](../features/mentions.md#mentions-and-the-prompt-cache)).

The console's client-side commands are dispatched before any of that: the first whitespace-separated field of the draft, with the leading `/` removed, is matched against the fixed list, and an unknown name falls through to the agent as an ordinary prompt (`dispatchSlash` in `external/cli/slash.go`).
