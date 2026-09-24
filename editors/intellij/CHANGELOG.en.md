<!--
  English twin of CHANGELOG.md, for the project website. The IDE renders only the Russian
  CHANGELOG.md in its Change Notes tab. Keep the same `##` headings from the top down and the
  same `**Title.**` entries in each section; older releases may stay untranslated below the last
  section here. Rules: .claude/rules/release-changelog.md. Checks: changelog_twin_test.go.
-->

# FoxxyCode plugin changes

## Unreleased — 2026-09-24

**A detailed network log in debug mode.**
With `debug.enable` on, the FoxxyCode log now spells out every request to the model step by
step: whether it goes through a proxy or direct, how the connection to the proxy went, the
`CONNECT` or SOCKS tunnel and TLS, whether an already open connection was reused, when the
first byte arrived and how the request ended. While a request is silent the log says every
15 seconds which step it is stuck in, and when the agent cut it itself (no first token, or an
answer that went quiet halfway) the log says which guard did. That is what a turn hanging for
hours on "Provider is not responding, retrying" can be read from. Messages about retried model
requests now reach the log file: they used to go to stderr and were lost.

## 0.3.19 — 2026-09-24

**The IDE no longer slows down while a turn runs in the panel.**
In 2023.x IDEs the panel renders off-screen, and every frame of the page is copied into the
IDE window. The bouncing "model is working" dots repainted the panel for the whole turn, and
the shimmering start-screen title in the dark theme did so for as long as the panel was open.
Without a GPU (in a virtual machine, for example) that slowed the whole IDE down and bloated
its memory. In the IntelliJ panel the turn dots no longer bounce: they take turns glowing in
the accent colour, slowly and in a few steps. The title, and the dots for an active session, a
pending question and a background task, stand still; the seconds counter in the status line
still shows that the turn is running. Measured in PyCharm 2023.3 with the GPU off, the
load on the IDE's UI thread dropped 2.5 to 5 times during a turn, and about twentyfold with the
panel open and idle.

**Panels without the blur, and an "Animations and translucency" switch.**
The chat header, the plan card, the top bar and the other "glass" panels in the IDE panel no
longer blur what lies under them: that blur was recomputed on every repaint of whatever scrolls
beneath, by the CPU when there is no GPU. The panels are now opaque in the same colour they
had, so the transcript no longer shows through them, and the page keeps its gradient. One
switch brings the animations and the translucency back: **Settings →
Appearance → Animations and translucency**. It is one setting for the whole interface: the
choice applies at once, is saved to `config.yaml` as `ui.effects` and holds wherever the same
FoxxyCode is open - other IDE windows, the browser and VS Code. In the IntelliJ panel the
switch starts off.

## 0.3.18 — 2026-09-23

**A Codex answer cut short by a limit is no longer passed off as complete.**
ChatGPT-subscription models (the `codex` provider) now count as finished only on the
server's terminal event. When an answer hits the length limit or the content filter, the
text written so far stays and the stop is reported as a limit, not as a normal end of the
turn; a stream cut midway is an error and is retried only while nothing has reached the
screen yet, so no text is shown twice.

**A network failure before the model answers is retried instead of ending the turn.**
When the connection to the provider never came up — a TLS handshake that hung, an address
that did not answer for a moment, a link cut by a reconnecting VPN or a large download on
the same machine — the request never reached the server, so FoxxyCode repeats it with the
usual backoff instead of failing the turn with `TLS handshake timeout`. What to do when the
error still arrives is in the Troubleshooting guide.

## 0.3.16 — 2026-09-21

**A steadier transcript: a jump-to-bottom button, paths relative to the project, honest question cards.**
Scroll the transcript up and a round button appears above the composer; it glides back
to the newest message. Tool rows spell paths relative to the session's folder, so the file
name shows instead of the start of a long path, and a row appears as soon as the model
names the call. Scrolling up a long transcript no longer jumps: rows no longer skip their
own layout, which made the height move under the reader (the IntelliJ panel's workaround
for it is gone too). A question card sits under its own row, an answered question shows
the option letters and marks the chosen ones, and Enter answers the question. The model
and the reasoning level are no longer reset when the settings are reloaded, and the
Russian reasoning rows speak in the first person.

**Reloading a tab mid-turn no longer repeats steps, and the live line stays.**
A tab opened again while the agent works asks the server only for what its loaded
transcript lacks, so the turn's finished steps no longer appear twice, and nothing is lost
when it re-attaches. Replayed events keep the time they happened, so a reasoning block and a
call's duration read correctly after a reload. The status line under the transcript stands
for the whole turn instead of vanishing once the model starts writing. Scheduler calls
render as cards - a job as its fields, a run or a resume as its outcome - and a web search
shows its parameters and how each engine answered. The activity dot in History lights for
every running chat, and a message taken back from the queue returns to the composer.

**Many open tabs no longer block each other, and Stop takes effect at once.**
A browser keeps only six connections to one server for all of its tabs, and each tab used
to hold one for its events stream - past six tabs, no request went out at all. The tabs of
one environment now share a single connection. Stop releases the turn's stream right after
sending the cancel instead of waiting for the answer, so the cancel never waits for a free
connection. The IntelliJ and VS Code panels, which are a single tab, keep a stream of their
own.

## 0.3.15 — 2026-09-21

**Ready-made workflows in the box: `/rpa-init`, `/rpa-feat`, `/rpa-bugfix`, `/rpa-gen-rules`.**
The binary now carries a whole delivery of skills rather than one, and writes it into
`${FOXXYCODE_HOME}/skills` the first time it sees them missing: getting to know an
unfamiliar repository, a feature by BDD, a bug fixed from a reproduction, and rules
generated for the agents. They are ordinary skills - editable, disable-able, deletable -
a deleted one is not written again, and a release with a newer version replaces an
unedited copy on disk. The old built-in `/generate-rules` is replaced by
`/rpa-gen-rules`. The marketplace they are published from
(`EvilFreelancer/rpa-skills`) shows up in Settings → Skills as a system source: it can be
synced, but not edited away or removed.

**Web search finds things again: Brave and Bing by default, your own SearXNG if you want one.**
The `websearch` tool now asks several engines at once and merges what they return, with
each engine's own outcome beside the results: "ok", "empty", or "blocked" with a reason.
An anti-bot page used to read as "the web has nothing". DuckDuckGo and Google are still
on the list but are not asked by default - from a server they answer with a decoy. The
engine set, the timeouts, the snippet length, the cache, the address of your own SearXNG
and a Brave API key are all in Settings → Tools → Web search (`tools.websearch`).

**The agent has `http_request`, a curl of its own, and saving settings no longer eats `${VAR}`.**
The new tool sends any HTTP request - methods, headers, a body, uploaded files, a proxy,
the response saved to a file - and returns the status line, the headers and the body. It is
offered in agent and debug mode only, and asks before it goes out unless the address is in
`tools.http_request.allowlist` or was approved in this session; the card shows the whole
request - address, headers, files, proxy - and offers to remember either that exact address
or the whole origin. Saving settings is fixed along the way: a `${BRAVE_API_KEY}` reference
in `config.yaml` is no longer replaced by its resolved value when you save from the UI, and
`ssh_connect_timeout` and the web-search section are no longer dropped.

## 0.3.13 — 2026-09-21

**A chat names and files itself.**
The pass that used to name a new chat now proposes one to three labels in the same
answer - one model call for both, and labels a session already carries are left
alone. During a long conversation the model can also rename the session and fix
its labels itself: the `session_describe` tool reads the current title and tags
and changes the parts a call names. It is offered in every mode, docs and ask
included, because it writes the session's own card and nothing in the working
copy.

**History: tags, an archive, pinned chats, grouping and a filter.**
A chat now carries short labels, an archived flag and a place at the top of the
list. History groups by folder (or by date, by tag, or not at all), searches the
title, the first message, the folder and the labels, and a filter menu gathers
four questions in one place: what to show (active, archived, everything), which
environment, how to group and how to sort. Pinned conversations lead the list as
one group and are dragged into the order you want; every row has a menu with
pinning, renaming, tags, archiving and deletion. An archived chat opens read-only
and says so, and neither archiving nor labelling moves a chat up the list as if
something had just happened in it.

## 0.3.12 — 2026-09-20

**Stop and the queue follow the session's turn, not this tab.**
When someone else started the turn in a chat - a second tab, the console, the
scheduler - or the tab lost its stream and has not re-attached yet, the composer
still shows Stop, and what you type is queued for that turn. Such a tab used to
offer a plain send and get a "chat is busy" refusal.

**A Stop that failed is visible and can be retried.**
Stop now waits for the server's answer and only then drops the local stream. If the
request did not get through, the transcript says "Could not stop generation. Try
again.", the turn keeps rendering and the button stays available. The tab used to
detach silently while the turn went on.

**A draft never lands in another chat or over what you typed.**
Text the server refused to queue or send returns to the composer of the chat it was
written in only, and only if nothing new has been typed there.

**Long turns are faster and cheaper: the provider's prompt cache no longer resets on every step.**
The system message is now built once per turn and sent back without a single byte
changed. It used to carry a clock with seconds and the todo checklist, so the provider
reprocessed the whole conversation on every step. The time, the checklist and the rules
a tool activated now travel in a separate block after the history. Eviction of old
`read`/`grep` results starts only once the context is half full (the
`compaction.result_eviction.start_percent` setting): until then the history is not
rewritten and the cache holds. The share of a request served from the cache shows in
the debug log, on the `llm call usage` line.

**A plan handed over for execution survives a permission prompt.**
When a turn started from the plan card stopped on a permission prompt, the agent went
on without the plan text after the answer. The plan now stays with the turn until it
is really over.

**The context ring and automatic compaction measure against the same window.**
When a model has no window size in the settings, it is now read from the provider's
model listing instead of borrowing the default model's window. The ring in the
composer could show 40% while the session was already at its limit, and compaction
never fired. A window that arrives late is picked up too - the ring is redrawn
without a reload.

**Compaction copes with a history that does not fit one request.**
A history that is too long is folded in several passes, each carrying the summary of
the ones before; while it runs, the transcript shows a row with the pass number. When
the summarizer model is unavailable the fallbacks are tried
(`compaction.fallback_models`, and `memory.fallback_models` for memory), with the
session's own model last.

**The model can compact the context itself.**
There is a new `compact_context` tool: the agent folds the history when it sees the
window filling up, without waiting for the threshold. Compaction from the button or
`/compact` now runs as a turn of the session, so every open tab and the console see it.

**The `/compact` command works on both compaction engines.**
On the `opencode` engine the command used to answer with a refusal, and compaction
over the API did not shrink the context. Both engines can now do the same things;
also, a second compaction on `opencode` no longer loses the first one's summary.

## 0.3.10 — 2026-09-19

**Agent edit highlights are readable in a light theme.**
Lines the agent changed were painted with one dark green background, which left black
code unreadable in a light theme. The colors now come from the editor's color scheme,
the same ones the diff viewer uses: inserted, modified and removed lines each get their
own color. Switching the theme recolors the highlights at once, without another edit.

**Highlights sit on the lines the agent changed.**
When the file was open in the editor during the edit, the highlight was placed on line
numbers of the new version while the editor still held the old text, and after the file
reloaded it drifted below the insertion, onto lines the agent never touched. The plugin
now reloads the file from disk first and matches the changed lines against the text in
the editor. A line the editor does not contain (because of unsaved changes, say) is not
highlighted at all, rather than painting its neighbour.

## 0.3.9 — 2026-09-19

**Config switches are now called `enable`, the way coddy-agent spells them.**
FoxxyCode used to call them `enabled`, so a `config.yaml` did not travel between the
two agents: a file carried over loaded without complaint, yet every switch stayed at
its default. `enable` is now the name everywhere - in the file, in the schema your
editor validates against, in the settings form and in the JSON API. The old `enabled`
keeps working: it is read as `enable`, `foxxycode -t` only warns about it, and the
next save from the settings screen rewrites the key together with the comment above
it. When a section sets both, `enable` wins.

## 0.3.8 — 2026-09-19

**A permission answer is no longer lost.**
Rarely, but it happened: Allow or Reject could come to nothing when the answer
arrived in the fraction of a second while the prompt was already visible but not
yet ready to take it. The chat reported the answer as delivered while the turn
went on waiting for its timeout. That gap is gone. A session also stops carrying
the "waiting for permission" mark when the prompt itself could not be shown
because the connection had dropped.

## 0.3.7 — 2026-09-18

**The fox mark replaces a faceless icon.**
The tool window stripe showed a pair of `</>` chevrons, and the card in
`Settings | Plugins` and on the JetBrains Marketplace was a grey placeholder
with no logo at all. Both now carry the orange FoxxyCode fox, the same one the
application icon uses. The tool window icon gained a separate dark variant, so
it reads equally well in Light and in Darcula.

## 0.3.6 — 2026-09-18

**The reasoning level in the composer follows the interface language.**
The reasoning level button next to the model picker showed "Minimal", "Low", "Medium" and
"High" even in a Russian interface. The button and its menu now read «Минимальный»,
«Низкий», «Средний» and «Высокий». A level the interface has no name for is still shown the
way it is written in the model settings.

## 0.3.4 — 2026-09-17

**The settings link to the FoxxyCode website.**
At the bottom of the settings, next to "API docs", there is now a "Website" link. The site has
the downloads of the plugins and the desktop app, a comparison with other agents, and the
changes of recent releases in English and Russian.

**Links from the panel open in your browser.**
"API docs" and the other links the panel opens in a new tab used to open an empty window on top
of the IDE. They now open in the default browser.

## 0.3.1 — 2026-09-17

**The FoxxyCode panel opens again in IDE 2026.2.**
Starting with 2026.2 (IntelliJ IDEA, PhpStorm, PyCharm, OpenIDE and other IDEs on build 262), the
JCEF embedded browser moved out of the platform core into a separate bundled plugin, "Web Browser
(JCEF)", and its classes are visible only to plugins that declare a dependency on it. FoxxyCode did
not declare one, so its tool window stayed empty in these IDEs and the IDE log showed
`NoClassDefFoundError: com/intellij/ui/jcef/JBCefBrowser`. The dependency is now declared and the
panel works; nothing changes in IDEs older than 2025.3. If the "Web Browser (JCEF)" plugin is
disabled, FoxxyCode shows a message asking to enable it instead of an empty window.

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
The folder excludes itself from `git status`, and the IDE does not index it, so search and
navigation no longer show every class of the project a second time.

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
`scheduler.enable` or running `foxxycode serve --scheduler`. For the agent the plugin starts,
the simplest way is to enable `scheduler.enable` in the settings.

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
