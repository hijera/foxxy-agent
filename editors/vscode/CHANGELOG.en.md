<!--
  English twin of CHANGELOG.md, for the project website. VS Code renders only the Russian
  CHANGELOG.md in the Changelog tab, and .vscodeignore keeps this file out of the VSIX. Keep the
  same `##` headings from the top down and the same `**Title.**` entries in each section; older
  releases may stay untranslated below the last section here. Rules:
  .claude/rules/release-changelog.md. Checks: test/changelog.test.ts.
-->

# FoxxyCode for VS Code changes

## 0.3.0 — 2026-09-17

**Versions now follow the 0.3.x line.**
Everything the roadmap assigned to 0.3.x has shipped: Subversion support on a par with git, the
IntelliJ plugin's own repository, fixes to the Windows build and to session rendering, the Debug
mode, background commands and subagents. So 0.2.95 is followed by 0.3.0, then 0.3.1, 0.3.2 and so
on. Updates arrive as usual; nothing needs to be reinstalled. What is planned for 0.4.x and later,
and what of it is already done, is marked in `ROADMAP.md` in the repository.

**The `.svn` service folder no longer shows up in the project.**
In a Subversion working copy the client keeps a `.svn` folder with a copy of every file. It used to
appear in the project tree the agent sees and in the file list for `@` mentions, and its contents
ended up in the turn's change snapshot. `.svn` is now skipped wherever `.git` is: after an
`svn update` or `svn commit` run by the agent, editing a message with a file rollback no longer
tries to "restore" the insides of the working copy.

**A session export names the SVN branch.**
`/export` and `foxxycode sessions export` wrote only the git branch into the document header. For a
session in a Subversion working copy the header now shows **SVN branch** (`trunk`,
`branches/feature-x`), and the JSON gets an `svn_branch` field. The branch is read from the local
working copy, without contacting the server.

## 0.2.95 — 2026-09-14

**You can send a follow-up while the agent is working.**
The composer is no longer locked during a turn. A message sent with the usual key or with the round
button (while the field has text, the button queues the message instead of stopping the turn)
appears above the composer, and the agent reads it at its next step - between tool calls, not after
the reply. Until the agent has read it, you can remove it with the cross. With an empty field the
button stops the turn as before, together with everything waiting in the queue. If this chat's turn
is running in another IDE window, the message returns to the composer with an explanation.

**Tool rows say what the agent is doing.**
A collapsed call row names the action in words ("reading a file", "running a command", "loading a
skill") and shows what it acts on next to it: the path, the command, the skill name or the page
address. A failed call is marked "(error)" right in the row, with no need to expand it. In the
expanded card the command sits in its own block after `$` with a copy button, and the header names
the shell the server runs it with. A loaded skill is shown as formatted text, a call with no
arguments shows a line with its state instead of `{}`, and the copy buttons became icons. The Copy
button and the time stay only under the reply that ends the turn.

**Branch working copies are created inside the project.**
When the agent opens a branch in a separate git worktree, the copy goes to
`.foxxycode/worktrees/<branch>` inside the project, not to the agent's home directory.
The folder excludes itself from `git status`, and therefore from the file search that honours
`.gitignore`.

**A renamed conversation moves up in History.**
A pinned title, a change of mode or model now move the conversation to the top of History, just like
a new message. Long conversations are saved faster: another message no longer makes the whole
transcript get encoded again.

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

**FoxxyCode as a model in Copilot Chat.**
The streamed `POST /v1/chat/completions` response now follows the OpenAI format strictly: the role
in the first chunk, reasoning and tool calls in their own fields, exactly one `finish_reason`. So a
FoxxyCode server connects to Copilot Chat in VS Code as a custom model, and tools work in that
chat. How to set it up is described in `docs/tutorials/foxxycode-as-a-model-in-vs-code.md`.

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
