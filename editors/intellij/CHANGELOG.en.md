<!--
  English twin of CHANGELOG.md, for the project website. The IDE renders only the Russian
  CHANGELOG.md in its Change Notes tab. Keep the same `##` headings from the top down and the
  same `**Title.**` entries in each section; older releases may stay untranslated below the last
  section here. Rules: .claude/rules/release-changelog.md. Checks: changelog_twin_test.go.
-->

# FoxxyCode plugin changes

## Unreleased — 2026-09-15

**The settings link to the FoxxyCode website.**
At the bottom of the settings, next to "API docs", there is now a "Website" link. The site has
the downloads of the plugins and the desktop app, a comparison with other agents, and the
changes of recent releases in English and Russian.

**Links from the panel open in your browser.**
"API docs" and the other links the panel opens in a new tab used to open an empty window on top
of the IDE. They now open in the default browser.

## 0.2.94 — 2026-09-14

**A Sessions table in Settings → Sessions.**
A new settings section gathers the saved conversations in a table: model, number of messages,
tokens spent, dates created and last changed. You can search it, select single rows or the whole
page, and delete the selection with one button. The conversation open right now is marked and
protected from deletion. In the IDE panel the table, like History, respects the "This project
only" checkbox: while it is on, only the conversations of the open project are shown and deleted.

**Your own `AGENTS.md`, `DESIGN.md` and rules in every project.**
`AGENTS.md` and `DESIGN.md` from the agent's home directory (`~/.foxxycode` or the directory set in
`FOXXYCODE_HOME`) and the rules from its `rules` directory now load in any project, above the
project's own files. `instructions.files` accepts `${FOXXYCODE_HOME}`, `${CWD}` and `~`, and lists
`AGENTS.md` and `DESIGN.md` by default: both project files still reach the model once each.

**Settings → MCP servers shows the real path to `mcp.json`.**
The hints on the servers named the global file `~/.foxxycode/mcp.json` even when the agent's home
directory is somewhere else. They now show the file the server was actually read from, and the
add-server form shows the real path of the global `mcp.json` when it already holds servers.

**Password sign-in for the `foxxycode serve` web UI.**
A server open to the network can be closed with a password: `foxxycode serve set-password` writes
the account to `config.yaml`, and the browser shows a sign-in form first. The IntelliJ and VS Code
panels and the `foxxycode desktop` window talk to the server on `127.0.0.1` and do not show the
form, even when a password is set.

## 0.2.93 — 2026-09-14

**A `config.yaml` saved in Notepad reads like any other.**
A settings file with Windows line endings and the byte order mark Notepad adds no longer
confuses validation: the `# yaml-language-server:` header is found, and an error points at the
line the editor shows. Saving from the settings keeps the file's existing line endings and no
longer adds a blank line under every comment. A YAML parse error names the line where the file
stopped parsing, not the start of some block further up.

**Project "History" finds its sessions however the path is spelled.**
The panel lists the sessions of the open project. When the project was opened through a
symbolic link and a session was started on the real path (or the other way round), that
session was missing from the list. Paths are now compared as folders: links are resolved, and
letter case does not matter on Windows and macOS.

**The Copy button and time under your own message are indented.**
Under agent replies and system messages this row sits away from the card edge, but under a
user message it was pressed against the edge. The spacing is now the same everywhere.

**The plan exit card uses the right word for agent mode in Russian.**
In the Russian interface, the card that switches from a plan to work now reads «Агентный режим»
and «Переход в агентный режим…», the correct wording for agent mode.

## 0.2.89 — 2026-09-14

**The plugin updates itself through a plugin repository.**
You no longer need to download a zip from GitHub and install it with "Install Plugin from Disk"
every time. Add `https://hijera.github.io/foxxy-agent/updatePlugins.xml` once under
**Settings → Plugins → ⚙ → Manage Plugin Repositories**, and the IDE finds FoxxyCode on the
Marketplace tab, installs it and offers updates like for any marketplace plugin. The address
always points at the latest release: it is updated automatically as soon as a new plugin is
built.

## 0.2.88 — 2026-09-14

**Scheduler job editor errors are readable in the light theme.**
Error messages in the job editor ("Required field" under an empty field, the hint under the
schedule when a cron expression cannot be parsed, and the reason a job could not be saved)
were pale pink: that color was picked for a dark background and almost disappeared on a light
one. In the light theme they are now dark red and easy to read. Dark themes are unchanged.

## 0.2.86 — 2026-09-14

**Delete buttons in the settings are clearly visible in the light theme.**
The trash buttons on providers, models, skills and MCP servers, and the "Sign out" button after
signing in with ChatGPT or NeuralDeep, were pale pink on white in the light theme and looked
disabled even though they worked. They are now red and easy to read, while a button that really
is disabled stays noticeably paler. Dark themes are unchanged.

## 0.2.85 — 2026-09-14

**The root `AGENTS.md` goes to the model once, not twice.**
The project's root `AGENTS.md` landed in the system prompt twice: among the project rules and
again under "Project instructions", because the same file is listed in `instructions.files` by
default. It now enters the prompt once, and every request to the model, at every step of a
turn, gets shorter by the size of that file: for a 24 KB `AGENTS.md` that is about 6 thousand
tokens by the context window estimate. The "System prompt" segment of the window shrinks by the
same amount, and the "Rules" segment stays the same. `DESIGN.md` is also included only once now
when it is listed in `instructions.files`.

## 0.2.84 — 2026-09-14

**Search in Settings → Skills explains where skills come from.**
The search box above the installed skills searches the marketplaces from the "Remote skill
sources" section. While that section is empty, it no longer silently answers "No matching
skills found"; it says that you first need to add a source, such as `owner/repo`, and save the
settings.

**Skills from a source you just saved are found right away.**
If you looked at the search before the source was saved, it kept showing the old empty list
after "Save" until the settings were reopened. The list is now re-read after sources are saved
and after a sync.

**Skill install and update messages are translated into Russian.**
Under the search, the Russian interface now shows translated messages instead of the English
"Installed …" and "Updated …".

## 0.2.83 — 2026-09-14

**The connection relay label on the swarm map is no longer cut off.**
On the left of the swarm map, each row of nodes is labeled with its level. The Russian label of
the first row did not fit its field and was cut off at the start. The field is wider now and the
label shows in full. The swarm map opens when a relay is selected as the environment.

**The disabled scheduler hint suggests `foxxycode serve --scheduler`.**
Instead of the `-scheduler-enabled` flag, the scheduler panel now suggests enabling
`scheduler.enabled` or running `foxxycode serve --scheduler`. For the agent the plugin starts,
the simplest way is to enable `scheduler.enabled` in the settings.

## 0.2.81 — 2026-09-13

**The agent no longer sees swarm pairing tokens.**
When the agent reads the settings with the `config_get` tool, values from
`swarm.pairing_tokens` now come back masked, just like provider keys. Before, the agent received
them in plain text, and a token could end up in a model reply and in the chat history.

**`config.yaml` can be checked without starting the agent.**
The program the plugin starts can now validate the settings file: `foxxycode http -t` checks it
against the schema and prints every error with its line and a hint on how to fix it, such as a
typo in a key that the agent would silently skip at startup. `foxxycode http --dry-run` also
checks what the file points to: whether the provider answers and accepts the key, whether MCP
commands are found, whether the port is free. Nothing is started or created, and the file itself
is not changed. `foxxycode`, `foxxycode acp` and `foxxycode serve` understand the same flags.

## 0.2.79 — 2026-09-13

**Rules from nested `AGENTS.md` files load when the agent enters a folder.**
Before, at the start of every chat the agent walked the whole project looking for nested
`AGENTS.md` files. Now a folder's `AGENTS.md`, together with every `AGENTS.md` on the way to it
from the project root, loads when the agent opens a file from that folder with a tool or you
attach a file from there to a message, and stays until the end of the chat. Folders the agent
never entered are not read at all, and an `AGENTS.md` created mid-chat is picked up as soon as
the agent goes there. The root `AGENTS.md` still loads every time.

**A new chat in a large project starts without a pause.**
The agent no longer needs to walk the project tree before the first call to the model. On a
workspace of about 700 thousand files this pause reached more than 20 seconds; now it takes a
fraction of a second.

## 0.2.78 — 2026-09-12

**A model error now names the provider and the address.**
When a request to the model fails, the chat message starts with the name of the `providers`
entry and the address the request actually went to: `provider "neuraldeep"
(https://api.neuraldeep.ru/v1): … 401 Unauthorized`. Before, a bare `401` arrived, and with
several providers there was no telling whose key it was. This is especially visible when a
provider with `type: openai` has no `api_base`: requests then go to `api.openai.com`, and now
that shows right in the error text.

**A model that went silent is asked again at once, without a minute of waiting.**
A hub usually runs several servers behind one model name, and a broken one answers every
attempt with silence. Before, such a call immediately showed the provider wait banner with a
one-minute pause; now the same request is first resent immediately and may reach another server
of the group. The banner appears only if the retry is silent too. It is turned off together with
the wait: `agent.llm_stall_retry: false`.

**A reply made only of reasoning is asked again first.**
When the model finishes a step with no text and no tool call, the agent first resends the same
request once and only then asks the model to continue. So the history sometimes shows two
"thinking" bubbles in a row, after which the turn goes on as usual. This is most often seen on
`gpt-oss-120b`.
