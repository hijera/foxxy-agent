# FoxxyCode embedded UI specification

This page captures the original UI requirements and the intended end state. It is a functional spec and a design contract.

## Constraints

- UI ships as static assets embedded into the `foxxycode` binary (build tag `http`).
- Runtime has no auth and no API key checks for the UI.
- UI must work over the same origin as `foxxycode http`.
- UI copy is English.
- Favicon matches [foxxycode.dev](https://foxxycode.dev/) (**`/foxxycode-favicon.svg`**, same mark as **`docs/assets/foxxycode-logo-mark-flat.svg`**, plus PNG/ICO fallbacks embedded with the SPA).
- **Browser baseline: Chromium 104** (JCEF in PhpStorm/IntelliJ 2022.3.3 — see [`docs/intellij-embedding.md`](intellij-embedding.md)). Shipped CSS/JS must not use features newer than Chromium 104: no **`:has()`**, **`oklch()`**/**`oklab()`**, **`@container`**, or native CSS nesting; **`dvh`**/**`svh`** only with a preceding **`vh`** fallback for the same property; no **`Array.prototype.toSorted`**, **`Promise.withResolvers`**, **`URL.canParse`** and similar post-104 JS APIs.
- **`color-mix()`** is allowed in `styles.css` **sources** only in the build-resolvable form (**`in srgb`**, arguments statically resolvable per theme — hex/rgb()/`transparent`/`var()` chains). `external/ui/postcss-resolve-color-mix.mjs` compiles every occurrence to Chromium-104-safe literals or per-theme **`--cmix-*`** variables; the build fails on unresolvable expressions.
- **`npm --prefix external/ui run check:compat`** (part of **`build:go`**) scans the built bundle and fails on baseline regressions.

## Composer context row

- **Wrapping**: the workspace chips share one **`flex-wrap`** row (**`.composer-context-row`**) with the environment chip and the improve-prompt control; **`.composer-context-chips`** is **`display: contents`** so each chip wraps on its own. On a narrow viewport only the overflow moves down (e.g. environment+folder, then branch+worktree), and the worktree checkbox stays beside the branch until the branch name is long enough to push it.
- **Improve prompt**: the compact **24×24px** wand button (**`data-testid="composer-enhance-btn"`**) lives at the **right edge** of that row, next to the environment / folder / branch / worktree controls — not in the textarea or the lower composer bar. At **≤520px** it is pinned to the row's **top-right corner** above the wrapped chips. Its label is translated (**`composer.enhance`**), it is disabled for a blank draft and while a request or generation is active, calls **`POST /foxxycode/enhance-prompt`**, and replaces the draft only on success. **Ctrl+Z** / **⌘Z** restores the pre-improvement draft; a failure leaves it unchanged and shows an inline error.

## Settings sections: General & Appearance

The Settings screen leads with two synthetic client-side tabs before the
schema-derived config tabs:

- **General** — composer **send mode** and the live **status line** toggle.
  The default tab.
- **Appearance** — theme, the app-wide **Language** picker (see below), and the
  "Restart onboarding" button.

The raw `ui` config key is hidden from the schema-driven tabs (`HIDDEN_KEYS` in
`settingsSections.ts`) so the curated controls are not duplicated; the key still
round-trips through the footer Save because the whole config doc is PUT back.

## Appearance (theme + language)

- **Default:** dark theme on first visit.
- **Cookie:** **`foxxycode_ui_theme`** with values **`dark`** or **`light`** (path **`/`**, **`SameSite=Lax`**).
- **`?theme=<id>` query parameter** (IDE embeddings): accepted values are all 7 theme ids; precedence **query > cookie > default**. Applied by the inline bootstrap script in **`index.html`** before first paint and persisted to the cookie. Contract-tested in **`themeCssContract.test.ts`** (the inline **`VALID`** map must stay in sync with **`UI_THEME_IDS`**).
- **`window.foxxycodeUi`** global API (**`external/ui/src/ui/theme/foxxycodeUiApi.ts`**, installed in **`main.tsx`**): **`setTheme`** / **`getTheme`** / **`getThemes`** / **`onThemeChange`** — lets a host (IntelliJ plugin via JCEF `executeJavaScript`) switch themes live. See [`docs/intellij-embedding.md`](intellij-embedding.md).
- **Toggle:** **Settings** (**`#/settings`**) → **Appearance** → **Dark** / **Light** (**`data-testid="theme-toggle-dark"`**, **`theme-toggle-light`**).
- **Settings sub-panels (Appearance / Skills) are mutually exclusive** — opening one closes the other. Only one sub-panel may be expanded at a time.
- **Persistence:** switching theme writes the cookie and sets **`document.documentElement.dataset.theme`**; reload must keep the chosen theme.
- **CSS contract:** **`--text`** and **`--bg`** on **`[data-theme="light"]`** are **`#18181b`** and **`#f8f8fa`**; glass panels use **`rgba(255, 255, 255, 0.9)`** (not dark tint). Dark defaults remain on **`:root`** / **`[data-theme="dark"]`**.

### Language picker

- **Where:** one native **`<select>`** directly under the theme grid
  (**`data-testid="appearance-language-select"`**, labelled by
  **`appearance.languageLabel`**), rendered by **`AppearanceLanguagePicker`** in
  **`external/ui/src/ui/theme/AppearanceModal.tsx`**. Options are **Auto**
  followed by every locale registered in **`ui/i18n/locales.ts`** — the list is
  derived, not hardcoded.
- **Persistence:** unlike the theme picker beside it, this one writes the backend
  config (**`ui.locale`**, values **`""`** = Auto, **`en`**, **`ru`**) through
  **`persistUiLocalePreference`**. It is the only language switcher across
  browser, desktop, and the VS Code / IntelliJ plugins (see
  [`docs/intellij-embedding.md`](intellij-embedding.md)), which read that key.
  With the Settings config doc loaded the pick is mirrored back via **`setDoc`**,
  so a later footer **Save all** cannot restore a stale value.
- **Auto** clears the **`foxxycode_ui_lang`** cookie so the next bootstrap
  follows **`navigator.language`** again instead of the previously picked
  language.
- **Bootstrap precedence:** **`?lang=`** > config **`ui.locale`** >
  **`foxxycode_ui_lang`** cookie > **`navigator.language`**.
- **i18n engine:** **`external/ui/src/ui/i18n/`**. **`locales.ts`** is the single
  registry (dictionary, label key, id); **`i18n.ts`** resolves keys against it and
  falls back to the default locale, then to the key itself.
- **Adding a language:** add **`messages/<id>.ts`** and one entry in
  **`locales.ts`**. The picker, cookie validation, **`?lang=`** parsing and
  **`messagesParity.test.ts`** all follow automatically.

## Composer attachments

- **Paste** — an image on the clipboard is attached instead of pasted as text
  (**`onPaste`** in **`Composer.tsx`**). Browsers name every clipboard image the
  same, so pasted files are renamed **`pasted-<n>.<ext>`**. Plain-text pastes are
  left to the browser.
- **Drop** — a file dropped anywhere on the page still inserts an **`@path`**
  mention (see the file-drop rule in **`.claude/rules/ui-spa.md`**); it does not
  attach the file. Use the paste path or the attach button for image uploads.
- **Multimodal gate** — chips are always shown, but for a model without
  **`multimodal: true`** they render disabled (dashed, greyed) and are excluded
  from the request; a paste is refused with a transient
  **`composer-attach-hint`** notice. The backend fails closed too: **`inline_files`**
  for a non-multimodal model are dropped before the provider call.
- **Attachment-only send** — an attachment with no text is a valid message.
- **Lifetime** — the list lives in **`ChatScreen`** (**`attachedFiles`** /
  **`onAttachedFilesChange`**), because hero and docked are two branches of one
  ternary and the composer unmounts on the transition.
- **Thumbnails** — image chips preview the file through an object URL. The sent
  bubble shows the same preview optimistically (**`optimisticUserFiles`**), then
  swaps to the durable **`files[].preview_url`** from
  **`GET /foxxycode/sessions/{id}/messages`** once it arrives; the superseded
  blob URL is revoked (**`revokeSupersededUserMessagePreviews`**). The backend
  writes bounded PNG previews to **`assets/thumbnails/`** and serves them from
  **`GET /foxxycode/sessions/{id}/assets/{name}/thumbnail`**.

## Settings: Codex OAuth

- The first-run provider picker includes **Codex**. Selecting it replaces the API-key field with the same **Sign In with ChatGPT** device-flow card, keeps the optional proxy and model controls, and saves `type: codex` without credentials in the config document.
- After sign-in, **Fetch models** probes the Codex catalog using the server-side credential for the unsaved `codex` provider name; the browser never receives or resubmits the OAuth token.
- In **Settings → LLM Providers**, a row with **`type: codex`** hides the generic **API base URL**, **API key**, and **API key command** fields and renders **Sign In with ChatGPT**.
- The button starts **`POST /foxxycode/providers/{name}/codex-auth/device`**, opens the returned official verification page, displays the one-time code, and polls **`GET .../device/{loginID}`** until completion or failure. The displayed link remains available if the browser blocks the automatic tab.
- Connected state comes from **`GET /foxxycode/providers/{name}/codex-auth`**. **Sign Out** deletes only the FoxxyCode-managed credential; a server-side Codex CLI login may still appear as a compatibility connection.
- OAuth tokens never enter the settings document or browser. The server stores them under **`$FOXXYCODE_HOME/providers/<name>/codex-auth.json`**.


## Settings: NeuralDeep sign-in

- In **Settings → LLM Providers**, a row with **`type: neuraldeep`** keeps the manual **API key** field and additionally renders **Sign In with NeuralDeep** below the read-only API base (**`NeuralDeepAuthField`**). Signing in is the no-paste alternative; an explicit key always wins and the widget says so instead of pretending the login is active (**`source`** from the status endpoint).
- The button starts **`POST /foxxycode/providers/{name}/neuraldeep-auth/device`** (the hub's RFC 8628 device flow for client **`foxxycode`** — the browser and the FoxxyCode server may be different machines), opens the returned portal page with the pre-filled code, displays the one-time code, and polls **`GET .../device/{loginID}`** until completion or failure.
- Connected state comes from **`GET /foxxycode/providers/{name}/neuraldeep-auth`** (masked key only). **Sign Out** best-effort revokes the key on the hub, then deletes the local credential through **`DELETE`**.
- The key never enters the settings document or browser; it is stored by the server under **`$FOXXYCODE_HOME/providers/<name>/neuraldeep-auth.json`**. Tier models are added under **Logical models** (the model picker fetches the provider catalog using this login); the CLI flow (**`foxxycode providers login neuraldeep`**) appends them to the config automatically.
## Layout

Desktop layout

- **Brand** is **typography only** (**FoxxyCode** and **agent**). **No** circular logo or icon before the brand text, regardless of older reference images that include a circle.
- Desktop nav is a **vertical panel** with rounding on the **right** edge (not a full-height center-pill). On **`min-width: 1920px`**, the wide rail header includes an icon with **horizontal lines** used **only** to **collapse** to narrow rail, not as a global navigation drawer.
- Left rail opens **chat history** from **History** under the brand; brand click goes to the **start screen** (**new chat**).
- **Brand**, **History**, **Scheduler** (when linked), **Settings**, and each row in the **History** list use real fragment **`href`** values (**`#/`**, **`#/history`**, **`#/scheduler`**, **`#/scheduler/new`** (new job editor), **`#/settings`**, **`#/s/<sessionId>`**) so **middle-click** or **Ctrl/Cmd-click** opens a **new browser tab** on the same origin while another tab can keep streaming.
- Sessions list is **always** a **drawer overlay** with backdrop at **all** breakpoints and rail widths (**no** inline column beside the rail that would shrink the chat area). The panel heading and related chrome use the copy **History**.
- **Picking a session closes the drawer only inside an editor plugin** (**`isEditorEmbed()`**, decided by **`shouldCloseHistoryOnSessionPick`** in **`sessions/pickSessionGuard.ts`**): an IDE tool window is narrow enough that the drawer covers the whole chat, so leaving it open reads as "nothing happened". Browser and desktop shells keep it open so several conversations can be browsed in a row. While the transcript loads, the skeleton shows a named **`chat-skeleton-label`** row (**`sessions.loadingSession`**) rather than bare shimmer bars.
- **Project scope toggle** (**`sessions-project-only`**) renders in the drawer only when the caller passes a host project root — the folder the server was launched with, read once from **`GET /foxxycode/workspace/context`** **without** a session header (do **not** reuse **`workspaceCtx`**, which follows the *viewed session* and would flip the scope when a foreign session is opened). When on, the list request carries **`cwd=<root>`** and shows only sessions in that folder or beneath it. Default is **on** inside an editor plugin and **off** in the browser, persisted per root in **`localStorage`** (**`sessions/sessionsProjectFilter.ts`**). A session running in a linked worktree **outside** the project root is only visible with the toggle off.
- Optional rail **narrow versus wide** (icons plus labels) only when **`min-width: 1920px`**, persisted in **`foxxycode_nav_rail`** cookie (**`narrow`** default)
- Main chat area with streamed assistant output
- Right rail is out of scope for the current milestone

Wide screens

- **`min-width: 1920px`** may enable the rail widen control and cookie-backed layout (**see DESIGN.md**). **History** remains a **floating drawer** next to the measured nav column (**`--rail-shell-track-width`**); do not fix **`left`** with a static pixel constant for wide rails.

Mobile layout

- On mobile the left rail becomes a top bar to preserve horizontal space; the top bar is **`position: fixed`** at the viewport top (**`shell-main`** is padded with **`--foxxycode-mobile-top-inset`**) while **`body`** scrolls the chat.
- On mobile the brand stays on a single line.

Header links

- GitHub link to `https://github.com/hijera/foxxycode-agent` (**new tab**, `rel=noopener`).
- API docs link to `/docs/` (**new tab**, `rel=noopener`).
- Links live in the nav rail for this milestone.

Narrow-rail tooltips (desktop)

- When the rail has **no** wide labels, **hover tooltips** reinforce icon meaning (example **New Chat** on the brand, **History** on history). **Wide labeled rail** hides those tooltips; labels are the affordance.
- After opening **History**, the history trigger's tooltip must **not** stay visible if the pointer still hovers the rail (see **DESIGN.md**).

## Sessions

- Session id is generated client side only after the first message is sent from a new chat.
- Session id is persisted in the URL fragment.
  - Recommended format `#/s/<sessionId>`
- Unsent composer text may be kept as a client-only draft session.
  - Draft sessions use `#/draft/<draftId>` and are stored in `localStorage` under `foxxycode_draft_sessions_v1`.
  - History rows show a `Draft:` title prefix.
- Session id is sent in the `X-FoxxyCode-Session-ID` header for chat transport.
- **Editor embeds reopen the project's last session.** Inside a plugin webview (`?embed=…`, `isEditorEmbed()`), an empty hash on load means *continue where the user left off*, not *new chat*: the SPA reads `GET /foxxycode/project/last-session` and routes to `#/s/<sessionId>`. Any explicit hash (`#/s/…`, `#/draft/…`, `#/history`, `#/settings`, `#/scheduler`) wins over the restore, and the desktop app and plain browser tabs are unaffected. The viewed session is recorded back with `PUT /foxxycode/project/last-session`; going back to a new chat records an empty id so that sticks too. Logic lives in `external/ui/src/ui/sessions/lastProjectSession.ts`; the record is per project in `~/.foxxycode/projects.json`, because plugins bind a fresh random port on every IDE launch and browser storage is keyed by origin.
- Session id validation matches `internal/session/ValidateFolderSessionID`.
- Session persisted files live under the session directory and are deleted together when the session is deleted.
  - `tool_calls/` tool call history
  - `stats.json` token usage totals

### Parallel sessions and generation cancel

- Several sessions may **stream at once**, each with its own **`POST /v1/responses`** and **`X-FoxxyCode-Session-ID`**. The app keeps a **per-session shadow** transcript so rapid hash switches do not mis-route SSE updates; see **`pickStreamMutationBase`** in **`external/ui/src/ui/chat/streamMutationBase.ts`**.
- **Stop** uses **`POST /foxxycode/sessions/{id}/cancel`** and **`AbortSignal`** on the streaming **`fetch`**. The server persists **partial** assistant **`content`** for that turn when tokens had already arrived. **`GET /foxxycode/sessions/{id}/messages`** may return an older snapshot briefly; the UI **merges** with local shadow or visible rows when the response is only a prefix (**`mergeTranscriptPreferLocalSuffix`**, **`keepLocalTranscriptIfServerEmpty`** in **`external/ui/src/ui/chat/transcriptServerSnapshot.ts`**). The transcript is cleared on fetch failure **only** when the failed load targets the **currently viewed** session so Stop does not wipe the chat.

Session title

- UI shows the session title in the chat header.
- When the title is missing, UI shows `New chat`.
- Title is editable inline. On blur the UI saves via `PATCH /foxxycode/sessions/{id}`.

### Per-session model

- **New chat** defaults **Model** from cookie **`foxxycode_llm_model`**, then **`default_agent_model`** from **`GET /v1/models`**, then the first YAML row.
- **Opening a session** restores **Model** from **`GET /foxxycode/sessions/{id}/messages`** field **`model`** (session override on disk), not from the cookie.
- Changing **Model** writes the cookie (default for the next **New chat**) and **`PATCH`** **`selectedModelId`** on the active session. ReAct turns still send **`metadata.model`** on **`POST /v1/responses`**.
- **Many models / long names** — backend ids are **`vendor/model`**. When more than one vendor is configured the menu groups rows under an uppercase vendor header and each row shows only the model name (full id stays in the row tooltip). On desktop the list scrolls with a ~5-row cap. When there are **more than 5** backends a **filter input** appears at the top (auto-focused) that matches the vendor, model name, or full id (case-insensitive); **Enter** picks the first match, **Escape** closes, and an empty result shows a “No models match …” notice. Filter/group/threshold logic is in **`chat/llmModelMenu.ts`** (unit-tested in **`llmModelMenu.test.ts`**; menu wiring covered by **`ComposerModelMenu.test.tsx`**).
- **Mobile sheet** — on narrow/mobile shells (the **`max-width: 1199px`** shell-stack breakpoint) the **Mode** / **Model** / **Reasoning** menus open as a **full-width bottom sheet** over a dimmed scrim — the same pattern as the slash (**`/`**) and **`@`** pickers — instead of a cramped anchored dropdown. The filter and grouping still apply inside the sheet. Desktop keeps the anchored dropdown.

### Per-session reasoning level

- A **Reasoning** selector appears in the composer next to **Model** **only** when the active model exposes **`reasoning_levels`** from **`GET /v1/models`** (reasoning models such as gpt-5 / o-series / gpt-oss / qwen3 / Claude thinking models). Levels are derived from **`models[].reasoning_levels`** (auto-detected from the model id when unset) and propagated through **`ModelInfo.reasoningLevels`** → **`llmReasoningLevels`** in **`App.tsx`** → **`Composer`**.
- **New chat** defaults the level from cookie **`foxxycode_llm_reasoning`**, then the model's **`reasoning_default`**, then **`medium`** (or the first offered level). **Opening a session** restores it from **`GET /foxxycode/sessions/{id}/messages`** field **`selectedReasoning`**. Switching to a model that does not offer the current level clamps it to a valid one (see **`pickReasoningLevel`** in **`chat/reasoningSelection.ts`**).
- Changing the level writes the cookie and **`PATCH`** **`selectedReasoning`** on the active session; ReAct turns also send **`metadata.reasoning`** on **`POST /v1/responses`** so a brand-new session applies it on the first turn.

### Per-session workspace (folder / branch / worktree / svn chips)

- A chip row renders at the top of the composer card (**`WorkspaceChips.tsx`**, helpers in **`chat/workspaceContext.ts`**): **folder chip** (workspace basename, full path in tooltip), **branch chip** (current git branch; only when the workspace is a git repository), a **worktree checkbox**, and — when an svn working copy is detected — an **SVN chip** plus its **branch-folder checkbox**.
- Context loads from **`GET /foxxycode/workspace/context`** with **`X-FoxxyCode-Session-ID`** whenever the viewed session changes; without a session the server default cwd is shown.
- **Chosen once**: folder + branch + worktree are set before the conversation starts. Once the transcript has messages the chips lock (**`workspaceLocked`** — controls disabled, menus closed) and the server answers **409** to **`POST .../workspace`**.
- **Folder chip** opens the **Recent** menu (Claude Desktop style): MRU folders from **`localStorage`** **`foxxycode_workspace_recents_v1`** (**`chat/workspaceRecents.ts`**), current workspace marked with **✓**, then **`Open folder…`** at the bottom which opens the **folder browser modal** (**`WorkspaceFolderModal.tsx`**) fed by **`GET /foxxycode/workspace/folders?path=`**: rows navigate into folders, **`..`** goes up, **Open** picks the currently browsed folder, **Cancel** dismisses. Picking calls **`POST /foxxycode/sessions/{id}/workspace`** **`{"path"}`** — the session cwd switches and persists; skills, project rules, and slash commands re-derive from the new cwd.
- **Branch chip** opens the branch list (current first, marked selected). Picking one posts **`{"branch", "worktree": <checkbox>}`**: in-place checkout by default, a dedicated worktree under **`<home>/worktrees/<repo>/`** when the checkbox is on, or a jump to the worktree that already has the branch checked out (including back to the main checkout).
- **Worktree checkbox** (**`composer-worktree-checkbox`**, real **`input[type=checkbox]`**) is the worktree preference; when the session already runs inside a linked worktree it shows checked and disabled.
- **SVN chip** (**`composer-svn-chip`**) renders next to the git chip whenever **`is_svn_repo`** is true. Git and Subversion are detected independently, so a branch folder checked out from SVN that also holds a git repository shows both chips and each switches only its own VCS. The chip label is the svn branch (**`trunk`**, **`branches/<name>`**), with URL and revision in the tooltip; the menu lists the current branch first, then **`trunk`**, then the rest. Picking one posts **`{"branch", "worktree": <checkbox>, "vcs": "svn"}`**.
- **SVN branch-folder checkbox** (**`composer-svn-folder-checkbox`**) replaces the worktree idea for Subversion, which has none: off switches the working copy in place (**`svn switch`**), on checks the branch out into its own folder under **`<home>/worktrees/<wc>/`** (reusing an existing checkout) and moves the session there — the branch-folder workflow.
- With **`vcs.svn.enabled: false`** or no svn client installed, **`is_svn_repo`** is false and neither svn chip renders.
- **Pre-session (draft/home)**: picks are stored client-side, previewed via **`GET /foxxycode/workspace/context?path=`**, and applied to the new session id on first send before **`POST /v1/responses`**. Switching to another session drops pending picks.
- Errors (missing folder **400**, git conflicts / locked workspace **409**) keep the current chips; the context is re-fetched to stay truthful.
- Automated checks: **`chat/workspaceContext.test.ts`**, **`chat/workspaceRecents.test.ts`** (helpers), **`chat/WorkspaceChips.test.tsx`** (chips, menus, modal, lock); backend behavior is specified executable in **`features/workspace_switching.feature`** and **`features/svn_workspace.feature`** (godog).
## Session list

- **History** panel lists sessions via `GET /foxxycode/sessions` (still a **drawer**, not a persistent second column).
- Pagination uses `limit` and `cursor`, with **infinite scroll** for older rows.
- Optional **`q`** query string (**title substring or first **`user`** message content substring only**, case insensitive; **not** full-chat search). Search input updates use client debouncing.
- Indicators
  - A spinner appears on rows for sessions that are still generating in the background.
  - A violet dot appears only when a background session completed while it was not the active chat.
  - A question mark icon appears when a session is waiting for user permission.
- CRUD
  - Rename via `PATCH /foxxycode/sessions/{id}` setting `title`.
  - Delete via `DELETE /foxxycode/sessions/{id}`.
  - Create new chat starts on the home screen. Session id is created only on first send.

Session rename UX

- Title rename is done only in the chat header.
- On blur the UI saves via `PATCH /foxxycode/sessions/{id}`.

Session delete UX

- Each row has a trash icon button.
- Clicking delete shows one confirm dialog and then calls `DELETE /foxxycode/sessions/{id}`.
- If the deleted session is **not** the one currently shown in the main chat, remove it from the list (and refresh from the server) and **keep the History drawer open**. Do not change the URL or clear the transcript for the session that stayed on screen.
- If the deleted session **is** the one currently shown, navigate to **new chat** (empty start screen, session hash cleared), **close** the History drawer, and clear composer-related state as for a normal home transition.
- For a short interval after the user confirms delete, **ignore** shell **backdrop** pointer-driven close so a stray event from the native confirm does not dismiss History or alter the route.

## Chat transport

- Primary transport is `POST /v1/responses`.
- `stream: true` uses SSE.

Mode selection

- UI lets the user select the FoxxyCode profiles `agent`, `plan`, `docs`, and `ask` from `GET /v1/models`.
- Selected mode is sent as `model` field in `POST /v1/responses`.
- Ask uses the green mode outline and remains non-mutating. **Settings → Tools → Disable extended Ask tools** is a schema-driven checkbox for `tools.ask_disable_extended_tools`; it is off by default. When enabled, Ask retains repository read/search/tree, question, and skill tools but hides shell, MCP, web, and scheduler inspection.

SSE payloads

- Default SSE lines stream OpenAI like deltas.
- Named SSE events
  - `tool_call`
  - `tool_call_update`
  - `plan`
  - `token_usage`
  - `usage_update` (`used` / `size` for the current model context; emitted again after compaction)
  - Default (no `event:`): chat completion chunk deltas, including `delta.content` and optional `delta.reasoning_content`

## Composer primary action (`#btn-send`)

Context ring and breakdown popover

- **Hover** on **`.composer-context-tip-host`**: compact tooltip (percent, input/output/total, max context) unchanged.
- **Click** opens **`ContextBreakdownPopover`** beside the ring on wide viewports (**`context-breakdown-menu--portal`**); on stacked shell (**`max-width: 1199px`**) it uses the same bottom sheet + scrim as slash / **`@`** pickers (**`context-breakdown-menu--sheet`**, **`slash-sheet-backdrop`**). **Escape** or **Close** dismisses; hover tooltip returns when closed.
- Legend keys map to **`contextBreakdown`** on **`GET /foxxycode/sessions/{id}/stats`** (`systemPrompt`, `toolDefinitions`, `rules`, `skills`, `mcp`, `subagents`, `conversation`, `summary`). Live **`usage_update`** SSE replaces the displayed total immediately (including after `/compact` or automatic compaction), then the UI refreshes the detailed stats. Vitest: **`Composer.test.tsx`** (`click context ring opens breakdown popover`) and **`consumeComposerSseOrder.test.ts`** (`usage_update replaces the displayed current context after compaction`).

Shape and glyphs

- The control sits to the **right** of the context ring (**`.composer-icon`** on **`Composer.tsx`**).
- The hit target is a **perfect circle**: equal **width** and **height**, **`border-radius: 50%`**, **`box-sizing: border-box`** (currently **42×42px** in **`styles.css`**). Do **not** ship a rounded square or squircle for this control unless the visual spec explicitly changes again.
- **Play** (**idle**, draft non-empty): Unicode triangle **`▶`**, enlarged vs body text (**`~22px`** glyph via **`composer-send-glyph`**), slight horizontal nudge for optical centering.
- **Stop** (**while streaming**): filled square **`.composer-stop-square`** (**14x14px**, centered in the **42px** circle). Stays in **`composer-bar-actions`** on the right, next to the context ring.
- **Disabled** idle state when textarea is whitespace-only (**`:disabled`** on **`composer-send-play`**).

Behavior (unchanged summary)

- **Enter** submits when idle and not generating; **`Shift+Enter`** newline. No submit while **`generating`**.
- **Stop**: **`POST /foxxycode/sessions/{id}/cancel`** + **`fetch`** **`AbortSignal`**. The server may append a **partial** assistant message for that turn. **`GET /foxxycode/sessions/{id}/messages`** can lag; the bundled UI merges server rows with local shadow or on-screen items (**`transcriptServerSnapshot.ts`**). Details in **`DESIGN.md`** (**Multi-session streaming and Stop**) and **`docs/http-api.md`**.

Regression

- Automated UI checks (**Playwright MCP** or **`@playwright/test`**) MAY assert **`#btn-send`** **`offsetWidth`** **≈** **`offsetHeight`** and computed **`border-radius`** **≥ half** **`min(width,height)`** (within sub-pixel tolerance).

## Composer file attachments (multimodal)

- The paperclip button (**`data-testid="composer-file-input"`** hidden `<input type="file">` triggered by a visible icon button) appears in the composer **only** when the active model has **`multimodal: true`** from **`GET /v1/models`**. The flag is derived from **`models[].multimodal`** in YAML config and propagated through **`ModelInfo.multimodal`** → **`llmModelMultimodal`** in **`App.tsx`** → **`Composer`** prop.
- Attached files are held in **`attachedFiles: File[]`** state on **`Composer`**. Preview chips appear above the composer input showing file name and type icon.
- On send, **`App.tsx`** reads each file as a data URL via **`FileReader`** and includes **`inline_files: [{name, data_url}]`** in the **`POST /v1/responses`** body.
- **Agent / plan turns**: the server writes each file to **`~/.foxxycode/sessions/<id>/assets/`** (permissions **`0o444`**) and injects a **`<foxxycode_session_assets>`** XML block into the user message so the agent can **`read`** or **`cp`** those paths. Duplicate asset names get **`_1`**, **`_2`** suffixes (see `internal/session/assets.go` **`SavePartsToAssets`**).
- **Direct YAML model turns**: each file becomes an **`image_url`** content part sent inline to the provider.
- The user bubble strips the XML annotation via **`stripFoxxyCodeAttachmentsForUserDisplay`** in **`stripFoxxyCodeAttachments.ts`** and shows file chips (**`msg-user-files`** / **`msg-user-file-chip`** CSS classes). **`parseSessionAssetFiles`** re-derives chip metadata on page reload.
- After a **`PUT /foxxycode/config`** save in Settings, **`App.tsx`** bumps **`modelsEpoch`** → re-fetches **`/v1/models`** so the attachment button appears or disappears without a page reload.

| Case | Expected | Automated check |
|------|----------|-----------------|
| FA1 | Paperclip visible only when `llmModelMultimodal` is true | `Composer.test.tsx` |
| FA2 | File chips render in user bubble after send | `stripFoxxyCodeAttachments.test.ts` |
| FA3 | Chips persist on reload via `parseSessionAssetFiles` | `stripFoxxyCodeAttachments.test.ts` |

## Composer slash skills and mirror caret

Authoritative narrative and visual tokens live in **`DESIGN.md`** (slash picker, mirror contract, verification table). This section is the functional contract for regression.

Wire and draft

- **`textarea#composer`** holds **plain text** only. Invoked skills appear as **`/<name>`** tokens (space after picker selection). The UI **must not** persist **`[/<name>](foxxycode-skill:<name>)`** in the draft.
- First user turn on **`POST /v1/responses`** carries the same plain slash tokens as the composer value (no client-side markdown injection for skills in the request body).

Picker and segmentation

- Menu visibility and **`prefix`** derive from **`slashMenuDraftAtCaret`** in **`external/ui/src/ui/skills/draftSlash.ts`** (line-start or whitespace before **`/`**, optional suffix, not inside fences or blockquotes).
- Mirror highlighting uses **`segmentComposerSlashSpans`** in **`external/ui/src/ui/skills/segmentComposerSlashSpans.ts`** (mid-line **`/`** supported; **`x/foo`** is not a command token).

Mirror and caret alignment

- Non-empty drafts: textarea text is drawn **transparent**; **`.composer-mirror-inner`** shows the visible line including **`.composer-skill-chip-inline`** (**`data-testid="composer-skill-chip"`**).
- Composer chips **must not** use horizontal **padding**, **margin**, or a **border** that changes inline width. Use **`box-shadow`** for outline. **`font-family`**, **`font-size`**, **`line-height`**, **`font-weight`**, **`letter-spacing`** on chip and **`#composer`** must match so the caret lines up (**`ResizeObserver`** syncs scrollbar gutter).

Transcript vs composer

- **`user_message`** bubbles render **plain text** only (**`msg-user-body`**, **`white-space: pre-wrap`**). No Markdown pipeline, no transcript skill chips (**`foxxycode-skill-span`**). Slash tokens such as **`/path/to`** and YAML blocks stay exactly as persisted, with line breaks preserved.
- Composer mirror chips (**`composer-skill-chip`**) apply **only** while editing **`#composer`**, not in the transcript.
- Persisted user turns may carry hydrated attachments as **`foxxycode_attachment`** XML with **`path`**, **`name`**, and CDATA file bodies (**`internal/agent`**). **`stripFoxxyCodeAttachmentsForUserDisplay`** replaces each XML block with a compact **`@path`** **only when** that path is **not** already present as an **`@`** mention in the surrounding text (**avoids duplication** because the persisted turn already repeats the **`@`** in the user text plus the hydrated block).

Verification use cases

| ID | Expectation | Primary automated check |
| --- | --- | --- |
| UC1 | One chip for **`asdfasf /find-skills asdfasdf`**, plain **`textarea.value`** | **`external/ui/src/ui/chat/Composer.test.tsx`** (`composer highlights plain slash token as chip while editing`) |
| UC2 | Mid-line menu open after whitespace | **`draftSlash.test.ts`** (`slashMenuDraftAtCaret works after whitespace mid-line`) |
| UC3 | **`x/foo`** no chip for **`/foo`** | **`segmentComposerSlashSpans.test.ts`** (`segmentComposerSlashSpans skips letter before slash`) |
| UC4 | Line-leading **`/foo`** chip | **`segmentComposerSlashSpans.test.ts`** (`segmentComposerSlashSpans line start slash`) |
| UC5 | **`stripFoxxyCodeSkillMarkdownLinks`** on legacy paste | **`segmentComposerSlashSpans.test.ts`** (`stripFoxxyCodeSkillMarkdownLinks restores plain slash token`) |
| UC6 | User bubble keeps **`hi /demo there`** plain (no **`foxxycode-skill-span`**) | **`UserMessage.test.tsx`** |
| UC7 | Multiline YAML / paths keep **`\\n`** layout in **`user-message-body`** | **`UserMessage.test.tsx`** |
| UC7b | Display-only **`slugSlashes`** (plain **`/`** and legacy mix) | **`segmentComposerSlashSpans.test.ts`** (`slugSlashesForUserBubbleMarkdown …`; composer / legacy only, not transcript) |
| UC8 | Live **`foxxycode http`**: **`fontFamily`** parity chip vs **`#composer`**, caret **`selectionStart === value.length`** at EOL after fill | **Playwright MCP** **`browser_evaluate`** after **`make build TAGS="http ui"`** |
| UC9 | User bubble hides **`foxxycode_attachment`** bodies, shows **`@path`** only | **`UserMessage.test.tsx`**, **`stripFoxxyCodeAttachments.test.ts`** |

## Composer **`@`** workspace files

- **`textarea#composer`** keeps plain **`input`** including literal **`@path`** text. **`POST /v1/responses`** adds **`attachments`** (**`path`** only) parsed by **`extractAtFileAttachments`** in **`external/ui/src/ui/skills/draftAt.ts`** for **`agent`** / **`plan`** / **`docs`** / **`ask`**. Server-side **`HydratePromptContentBlocks`** uses **`ExtractAtFilePathsFromText`** (**`internal/session/at_paths_extract.go`**) after filling empty **`resource`** bodies so **`@path`** literals inside **`type: text`** blocks become extra **`resource`** rows when that path is not already hydrated (**matches HTTP **`attachments`** without duplicating**).
- **`@`** menu uses **`GET /foxxycode/workspace/files`** with **`dirs=true`** so **`kind`** **`dir`** rows drill down. Choosing a **`dir`** inserts **`@`** + **`path_rel`** (often ending in **`/`**) without hydrating file body. Choosing a **`file`** inserts **`@`** + **`path_rel`** plus a trailing ASCII space where appropriate. **`Composer`** defers two **`updatePickerMenus`** ticks after a row choice so the workspace dropdown does not immediately reopen (trailing space and **`MENU_PATH_CHAR`** still satisfy **`atMenuDraftAtCaret`** until the user edits again).
- Empty **`@`** prefix (caret right after **`@`**) loads recent rows from **`localStorage`** (**`workspaceAtRecents`**), keyed by **`sessionId`** (or **`__no_session__`** before the first assigned id), with no extra banner line (**`Type after @ to search`** only when the list is empty). Entries come from **`@`** row picks and **`extractAtFileAttachments`** on successful profile sends (**`migrateWorkspaceAtRecents`** merges when the client generates or the server rotates **`X-FoxxyCode-Session-ID`**).
- Fenced code blocks and Markdown blockquote lines suppress **`@`** menu parity with **`draftSlash`** ( **`inMarkdownFenceBeforeCaret`**, **`blockquoteLine`** ).
- Mirror **`@`** styling uses **`segmentComposerMirrorSpans`** (**`composer-at-chip-inline`**, **`data-testid="composer-at-chip"`**). **`listAtPathSpans`** (**`draftAt.ts`**) chips every completed **`@path`** atom even when prose follows (**`draftAt`** parity with **`extractAtFileAttachments`**), while text after the caret that is still inside **`MENU_PATH`** stays on the active token until the **`atMenuDraftAtCaret`** lexer breaks out.
- **`@`** search with zero matches keeps the picker open (**`No files`**) instead of collapsing the menu (**`composer-at-chip-inline`** hides for **`atNoMatch`**, same **`atIdx`**, **`prefix`** as the stale filter).
- Stacked-shell viewports (**`(max-width: 1199px)`**) render workspace and slash pickers as a **`slash-menu--sheet`** with **`slash-sheet-backdrop`** so the panel is usable on phones.
- Picker subtitle uses **`workspacePickRowSubtitle`** - second column shows **`parent/`** only when **`path_rel`** is nested, root entries omit it (empty string).

| Case | Expected | Automated check |
| --- | --- | --- |
| AT1 | Spaces inside paths ( **`readme copy.md`** ) work in picker draft and hydrate when attached | **`draftAt.test.ts`**, **`session/promptfiles_test.go`** (**`hello world.txt`**) |
| AT2 | **Prefix** substring filter (**case-insensitive**), empty **prefix** returns empty **`items`** on server | **`TestFoxxyCodeWorkspaceFilesGetPagingAndPrefixes`** |
| AT3 | Prose **`see @note.txt`** does not merge **`and`** into the path segment | **`draftAt.test.ts`** (**`extractAtFileAttachments`** connector words) |
| AT4 | **`@`** inside **`session/prompt`** text alone still hydrates (no duplicate when **`attachments`** or **`resource`** already has body text) | **`TestHydratePromptContentBlocksExpandsAtInText`**, **`at_paths_extract_test.go`** |
| AT5 | Picker second column shows **`parent/`** for nested **`path_rel`**, empty at workspace root (**`workspacePickRowSubtitle`**) | **`workspacePickRowSubtitle.test.ts`** |

## Transcript message types

The chat transcript renders a flat list of UI message blocks. Each block has a `type` and a minimal set of required fields.

- `user_message`
  - Plain user input text (**no Markdown**; **`pre-wrap`** preserves line breaks).
- `thinking`
  - Renders model reasoning as a lightweight disclosure row.
  - Status `in_progress` shows label `thinking...` and a spinner.
  - Status `completed` shows label `thinking` and preserves the text for review.
  - Multiple `thinking` blocks may appear in one turn (reasoning can resume after tool calls).
- `tool_call`
  - A single tool execution row, same disclosure chrome as **thinking** / **memory** (**chevron**, **`thinking-label`** with the tool name or kind, **`thinking-dur`** for duration or **`-`**).
  - While **`pending`** or **`in_progress`**, the summary label uses a **`...`** suffix (for example **`read_file...`**). **`startedAtMs`** drives a live duration until the tool finishes.
  - When a structured preview and **Result** are both present, they touch and share the outer corners as one continuous execution card; there is no gap or duplicate border between them.
  - Details reuse the permission card's tool-specific preview in a static mode: full diff / path / command content, but no copy, **More…**, or approval actions. **read**, **grep**, **glob**, and **print_tree** also receive compact structured argument previews; unknown tools keep a styled monospace fallback. The separate **Result** body is plain text only (rendered like **`<pre>`**, **no** Markdown pipeline). If **`resultPreviewTruncated`** is false / **`resultWasTruncated`** unset, there is no overflow toggle or fixed-height viewport (block height follows content). If truncated (19 content lines plus **`...`**), apply the capped viewport (~20 lines), with **overflow-y** hidden until **More…**. **More…** (**`data-testid="tool-result-more"`**) performs **GET `/foxxycode/sessions/{id}/tool-calls/{toolCallId}`**, then enables **overflow-y auto** at the same height and becomes **Less** (**`data-testid="tool-result-less"`**); **Less** restores the clipped preview without a second GET while **fullResultText** stays in memory. Both use the shared left-aligned **`tool-overflow-toggle`** tab button attached flush to the result panel's bottom border.

## Tool call card (bundled SPA, current)

Authoritative behaviour matches **`DESIGN.md`** tool timeline plus this checklist.

| Concern | Current behaviour |
| --- | --- |
| Component | **`ToolCallMessage.tsx`** - **`thinking-row foxxycode-tool-call-row`**, **`details.thinking-details.foxxycode-tool-details`**, **`data-testid`**: **`tool-details-{toolCallId}`** |
| Summary | Same pattern as **thinking** (**`thinking-summary`**, **`thinking-left`**, **`thinking-chevron`**, **`thinking-label`**, **`thinking-dur`**), **`aria-label="Tool summary"`** |
| Args | Shared **`PermissionToolPreview`** (no copy / approval actions); large **write** / **write_file**, **apply_patch**, and **edit** bodies keep measured **More…** (**`data-testid="tool-preview-more"`**) / **Less** (**`data-testid="tool-preview-less"`**) overflow controls |
| Result | **`div`** with **`tool-block tool-result tool-result-raw`**, **`aria-label="Tool result"`**, inner **`pre.tool-result-pre`** |
| Markdown | Not used for tool **result** or **user** bubbles; **assistant** still uses Markdown per below |
| List merge | **`App.tsx`** **`loadMessages`** merges **`GET /foxxycode/sessions/{id}/tool-calls`** rows into **`resultText`**, **`resultWasTruncated`**, timing |
| Full text | First result **More…**, or automatic incomplete-args recovery for restored **`apply_patch`** / **`write`** / **`write_file`** / **`edit`** cards in any status - **`GET /foxxycode/sessions/{id}/tool-calls/{toolCallId}`**, using JSON **`result`** and **`args`** (same object includes **`meta`**). Transcript reconciles never replace complete args with the truncated 200-char **`argsPreview`** (**`pickRicherToolArgs`**), so live cards keep full previews across permission answers |
| CSS | **`styles.css`**: **`.foxxycode-tool-call-row`**, transparent **`.foxxycode-tool-call-body`**, shared **`.permission-preview*`**, **`.tool-call-result-card`**, **`thinking-details:not([open])` body hidden**, plus result viewport / toggle classes above |

- `assistant_message`
  - Final assistant output text for the turn, after tool calls.

## Tool permission card

The inline approval gate is implemented by **PermissionPromptSection** and **PermissionPromptPreview**.

- Render the card only for a pending permission request. Read-only tools render their normal timeline row only; there is no informational no-approval card, checkmark, or explanatory sentence.
- Header: human action question plus one raw tool-id badge. The preview header is reserved for the path, shell, or operation scope so the tool name is not duplicated. The desktop notification toast reuses the same question text.
- Actions follow the server-provided option list and order (**Allow**, **Allow always**, optional **Always allow `<program>`**, **Reject**); a fourth button needs no client change beyond layout. Labels are localized by **`optionId`** in **`chat/permissionOptionLabel.ts`** rather than rendered from the backend's English text.
- The program-wide option only reaches the client for **run_command** on a single plain invocation. The backend label already names the exact grant (**`curl`**, **`git status`**), so the program name is carried through the translation verbatim rather than re-derived.
- Match the prompt to its **tool_call** by **toolCallId** and prefer that row's **argsText**; fall back to **Arguments:** content in the permission payload.
- **apply_patch** and **edit** render old/new line gutters and theme-aware added/deleted/context rows. Other filesystem mutation tools and **run_command** use compact structured previews rather than JSON.
- The collapsed preview is measured after layout. Show **More…** only when **scrollHeight > clientHeight**; keep the viewport bounded, switch it to internal vertical scrolling, and change the button to **Less**. Returning to the collapsed state restores clipping and re-measures overflow. The shared button is left-aligned; on phones it has a **36px** minimum height.
- Restored write permission prompts include **rm** and **rmdir** alongside the other filesystem mutation tools.
- All question / header / metadata strings come from **`t()`** with **`en.ts`** + **`ru.ts`** parity, so the card renders in the active UI locale.

Automated checks:

- **external/ui/src/ui/chat/permissionToolPreview.test.ts**
- **external/ui/src/ui/chat/PermissionPromptSection.test.tsx**
- **external/ui/src/ui/chat/permissionPromptPreviewCss.test.ts**
- **external/ui/src/ui/messages/MessageList.test.tsx**
- **external/ui/src/ui/messages/toolCallConnectedResultCss.test.ts**

## Background tasks panel

The panel is docked **inside the session**, to the right of the transcript (`.bgtasks-panel`), not a shell drawer: a task belongs to the chat that started it. Routes are `#/s/<sessionId>/tasks` and `#/s/<sessionId>/tasks/<task_id>`, so a reload restores the chat and the panel together; closing writes `#/s/<sessionId>` back. Backed by `/foxxycode/sessions/{id}/background-tasks*` (see `docs/background-tasks.md`).

- It **polls** rather than listening on SSE, because a background task outlives the turn that started it: every 2.5s while anything runs, every 15s otherwise. A poll against an unreachable server yields a normal error result, never an unhandled rejection.
- **Running** is a section of cards (status dot, command, elapsed against the estimate, Stop). A progress bar appears only while running **and** when the model supplied `expected_seconds`.
- **Finished N** is a counter; expanding it lists one line per task, capped at 40 rendered rows with a note naming what stays on disk. **Clear** drops the finished history for the session.
- Ordering is purely by start time, newest first, in both sections.
- The **opener** is a chip at the end of the transcript (under the last message, above the composer), not a nav rail entry: `N running tasks` while work is in flight, `N background tasks` otherwise, and nothing at all in a chat that never ran one.
- On `max-width: 1199px` the panel takes the screen and finished rows grow to a 40px touch target.
- A transcript `run_command` row that started a task keeps a live chip in its **collapsed** summary and gains **Open in Tasks** / **Stop** when expanded, driven by the same poll.

Automated checks:

- **external/ui/src/ui/tasks/taskStatus.test.ts** (timing, progress, overdue, poll cadence, start-time ordering, grouping)
- **external/ui/src/ui/tasks/BackgroundTasksPanel.test.tsx** (sections, finished counter, Clear, detail pane, empty and error states)
- **external/ui/src/ui/tasks/api.test.ts** (paths, headers, offline degradation)
- **external/ui/src/ui/tasks/BackgroundTasksChip.test.tsx** (counts, singular/plural, history fallback, empty chat)
- **external/ui/src/ui/tasks/backgroundTaskCss.test.ts** (chip tokens, panel docking, reduced motion)
- **external/ui/src/ui/messages/ToolCallMessage.test.tsx** (transcript ticker chip)

## Live token usage

- UI must show token counters while the agent is working.
- Counters update when SSE event `token_usage` arrives.
- Update granularity is per completed backend model call, not per generated token.
- UI restores token counters after restart via `GET /foxxycode/sessions/{id}/stats`.
- **What the number means: tokens spent by this chat since it was created.** `/stats` reports
  `tokenUsageTotal` cumulative for the session; the SSE `token_usage` of the running turn is
  cumulative for that turn and is added on top of the baseline read **before** it started.
  The two must not be mixed: `applySessionStatsPayload` (`App.tsx`) routes the payload through
  **`planSessionStatsApply`** (`chat/sessionTokenTotals.ts`), which refuses to reseed the
  baseline while a composer stream is attached to that session — the poll runs every 800ms, so
  reseeding from a total that already counts the turn double-counted it, and compounded on
  every poll. Regression: **`chat/sessionTokenTotals.test.ts`**.
- **The context breakdown is exempt from that gate** and is applied whenever it arrives: it is a
  live estimate, not an accumulator, and it is the only thing that reports compaction shrinking
  the window, so the ring has to be able to drop mid-turn.
- **The poll follows the session, not this client.** Stats refresh while `generating` **or**
  `viewedTurnActive` (raised by the `turnActive` probes in `startDiskFallbackPoll` and
  `reconnectLiveStreamIfActive`). A turn that outlived its request, or the autonomous turn a
  finished background task woke, burns context the same way; keying the poll on `generating`
  alone left the ring frozen until the turn ended.

## Live status line

- The row next to the typing dots (`TypingDotsMessage`, phrase from `chat/liveStatus.ts`) is
  derived from the transcript, so it is only as fresh as the transcript is.
- **While re-attaching, the transcript is stale by construction.** `rejoinComposerLiveStream`
  therefore keeps the session flagged through `markReconnecting` until the relay delivers its
  first byte, and only then calls `markConnected`; `deriveLiveStatus` shows
  `status.reconnecting` meanwhile. `addActiveComposer` clears that flag on attach, which is
  correct for a fresh `POST /v1/responses` (its baseline is the message just sent) and wrong for
  a rejoin — a rejoin that showed the pre-turn transcript's last tool is what froze the row for
  the whole time the server held the request open with no relay to attach to.
- The `background_*` tool family has phrases of its own (`status.backgroundWait` and friends).
  `background_wait` parks for up to a minute, and the generic "Running a tool" read as a status
  line that had stopped moving.

## Markdown rendering

- Tool outputs are excluded; they stay raw monospace text (**`ToolCallMessage`**).
- **User** messages are plain text with preserved line breaks (**`UserMessage`**).
- **Assistant** messages may contain Markdown.
- UI renders Markdown with fenced code blocks and syntax highlighting.
- Each code block has a copy button that copies only that block content.

## Markdown line editor (shared)

Implemented as **`MarkdownLineEditor`** (`external/ui/src/ui/markdown/MarkdownLineEditor.tsx`). Used for:

- Scheduler job **`body (markdown)`** (`SchedulerJobEditorSheet`, default **`minRows`** **10**).
- Plan document card markdown mode (`PlanDocumentSection`, **`minRows`** **4**, class **`md-line-editor--plan`**).

Behaviour (see **`DESIGN.md`**, **Markdown line editor**):

- Full parent width; editor height follows content (minimum logical rows); **no** scrollbar on the inner **`textarea`**.
- Gutter shows one number per **logical** line (`\n`-separated). Wrapped visual lines leave **blank** gutter cells (no duplicate numbers).
- Caret logical line: highlight spans **all** visual rows of that line; active gutter number tinted.
- Wrap measurement uses a hidden probe with the same font and text width as the textarea; visual rows = **`ceil(height / lineHeight)`**.
- Long unbreakable tokens wrap (**`overflow-wrap: anywhere`**); no horizontal scroll inside the editor.

Automated checks:

- `external/ui/src/ui/markdown/MarkdownLineEditor.test.tsx`
- `external/ui/src/ui/markdown/markdownLineGutter.test.ts`

## Plan document card (plan mode transcript)

Transcript type **`plan_document`** renders **`PlanDocumentSection`** in the main chat column (not a right rail).

Data and API:

- Persisted in **`messages.json`**; hydrated fields include **`slug`**, **`name`**, **`overview`**, **`content`**, optional **`body`**, **`path`**, **`discarded`**.
- Live during a turn: SSE **`event: plan`** whose **`_meta`** holds **`foxxycode.dev/planKind: design`** and **`foxxycode.dev/planSlug`**; the SPA then loads the document from **`GET /foxxycode/sessions/{id}/plans/{slug}`** and upserts the card by slug.
- Body edit: **`PUT /foxxycode/sessions/{id}/plans/{slug}`** with **`{ "body": "<markdown>" }`** (debounced autosave).
- Discard: **`DELETE /foxxycode/sessions/{id}/plans/{slug}`** sets **`discarded: true`**; card remains visible, controls disabled.
- Run plan: client triggers implementation run (metadata / prompt; see **`docs/acp-protocol.md`**).

UI requirements:

- **Always the last row of its turn**, below the assistant text that introduces it (**`pinPlanDocumentsToTurnEnd`**), during streaming and after the transcript rebuild. The server emits the row mid-turn, so message order alone would put the card above the answer.
- Rebuilds keep the card's identity by **slug**: the expanded state survives, and an unsaved markdown draft is not overwritten while a save is pending.
- Collapsed: title, one-line description, **Discard** and **Run plan** in footer; title **`title`** tooltip = absolute plan file path when known.
- Expanded: **Preview** default (rendered markdown via **`Markdown`**); eye toggle switches to **`MarkdownLineEditor`**.
- Content pane grows with document length for **both** preview and markdown (**no** inner max-height scroll on the pane).
- Expanded desktop (**`min-width: 640px`**): title row and action buttons share the top row; body full width below.
- Editor body excludes YAML frontmatter (client **`planEditorBody`**); preview uses the same body text.

Automated checks:

- `external/ui/src/ui/chat/PlanDocumentSection.test.tsx`
- `external/ui/src/ui/chat/planDocumentPlacement.test.ts`
- `features/plan_card_placement.feature` (godog steps in `external/httpserver/bdd_plan_card_test.go`)

## Plan and todo list (legacy rail)

- Optional right-rail plan entries (if present in a build) use **`GET /foxxycode/sessions/{id}/plan`**, **`PUT`**, **`POST .../plan/archive`**.
- Distinct from the **`plan_document`** transcript card above.

## Long term memory

Memory tree roots

- `global`
- `workspace`

Tree API

- `GET /foxxycode/sessions/{id}/memory/tree`
  - Without `root` returns the roots list.
  - With `root` and optional `path` lists children.
- Only `.md` and `.txt` files are listed.
- Path traversal must be rejected.

File API

- `GET /foxxycode/sessions/{id}/memory/file` reads.
- `PUT /foxxycode/sessions/{id}/memory/file` writes.

## MCP servers (Settings tab)

Functional checklist for the Settings -> MCP servers tab (`MCPSection.tsx`,
section kind `mcp`; visual contract in `DESIGN.md`):

- `GET /foxxycode/mcp` backs the list: merged `config.yaml` + global `~/.foxxycode/mcp.json`
  + project `./.foxxycode/mcp.json` servers, each with `source` (`global` / `local`
  scope badge), `origin` (`config` / `home` / `project` — drives the badge
  tooltip naming the owning file), `readonly` (config.yaml entries), probe
  `status`, and its tool inventory.
- Status dot per server: connected (green), error (red, tooltip shows the probe
  error), disabled (gray), unknown transport type (amber, `unsupported`).
- Server switch toggles `POST /foxxycode/mcp/{name}/enable|disable`; the change
  persists into the file that defines the server.
- Expanding a row lists tools with per-tool switches
  (`POST /foxxycode/mcp/{name}/tools/{tool}/enable|disable`); tool switches are
  locked while the server is disabled.
- Edit and Delete are locked for `readonly` (config.yaml) rows; mcp.json rows
  of both scopes stay editable. Delete calls `DELETE /foxxycode/mcp/{name}`, Edit
  opens the JSON editor card inline with the scope pinned to the owning file.
- Add server opens the editor prefilled with a Cursor-style entry template and
  a Local/Global scope picker (default Local); Save issues
  `PUT /foxxycode/mcp/{name}?scope=local|global` after client-side validation
  (`mcpServerJson.ts`: JSON object, `command` or `url` required, name without
  `__`, spaces, or path separators).
- Refresh re-probes all servers via `GET /foxxycode/mcp?refresh=1`.
- An **MCP discovery** fieldset above the list carries `mcp.project_trust`
  (`POST /foxxycode/mcp/project-trust`, values `ask` / `allow` / `deny`). It never
  joins the settings-document Save all flow.
- A project-local server the workspace trust gate holds back shows `status`
  `needs_approval` (amber dot), exposes **no tools** — it is reported, not probed —
  and opens a note listing the declaration an approval would cover: transport,
  the command with arguments or the URL, env and header **names** (never their
  values), and the workspace. `denied` renders the same place with the policy
  explanation instead.
- The per-server **shield** toggles `POST /foxxycode/mcp/{name}/trust|untrust`.
  It renders only under the `ask` policy, since `allow` and `deny` leave no
  per-server decision to make.
- List refreshes never unmount the list (initial-load-only placeholder), so the
  drawer scroll position is preserved.
- The tab does not participate in the settings document Save all flow.

## Swagger

- Swagger UI is served under `/docs/`.
- OpenAPI spec is served under `/openapi.yaml` and `/openapi.json`.
- Swagger UI assets must be embedded, no CDN.

## Development workflow

- Edit TypeScript sources under `external/ui/src/`.
- Use `npm --prefix external/ui run dev` to iterate without rebuilding the Go binary.
- Build and sync embed assets with `npm --prefix external/ui run build:go`.
- **`make build TAGS="http ui"`** runs the UI build step (**make ui-build**) before linking the embedded bundle.

## Reference images

Store the provided design reference images under `docs/assets/`.

When describing a specific element, link to the relevant image file.

- Full HD UI tour (README): `docs/assets/screenshot-fullhd-start.png`, `screenshot-fullhd-chat.png`, `screenshot-fullhd-history.png`, `screenshot-fullhd-scheduler.png`, `screenshot-fullhd-settings.png`
- Mobile UI tour (README): `docs/assets/screenshot-mobile-start.png`, `screenshot-mobile-chat.png`
- Home layout: `docs/assets/ref-home-1.png`, `ref-home-2.png`, `ref-home-3.png`
- Home scroll state: `docs/assets/ref-home-scroll.png`
- Composer state: `docs/assets/ref-home-composer.png`
- Left rail icon states: `docs/assets/ref-rail-states.png`
- Chat history view: `docs/assets/ref-history.png`
- Chat transcript view: `docs/assets/ref-chat.png`
- Flow montage: `docs/assets/ref-flow.png`

## UI test scenarios

These scenarios are intended to be automated via Playwright against the Vite dev server.
Run `npm run test:layout` from `external/ui` for the bounded 390px/1280px
column-alignment smoke. The test owns Vite and headless Chrome, waits at most
10 seconds for readiness, limits the browser phase to 20 seconds, and cleans up
both processes with headroom under a 45-second outer timeout.

- Desktop navigation has no width toggle
  - Given viewport width is at least 1024px
  - When the app loads
  - Then `data-testid="nav-menu"` is visible
  - And `data-testid="nav-toggle-width"` is not present

- Sessions are drawer only
  - Given any desktop viewport
  - When the app loads
  - Then `data-testid="sessions"` is not visible
  - When user clicks `data-testid="nav-menu"`
  - Then `data-testid="sessions"` becomes visible
  - When user clicks `data-testid="sessions-close"`
  - Then the sessions drawer is hidden

- Mobile uses top bar and single line brand
  - Given viewport width is at most 1199px
  - When the app loads
  - Then the nav width toggle is not present
  - And the nav rail height is 78px
  - And sessions can still be opened from the menu button

- Tool calls survive restart
  - Given a session has tool calls executed
  - When the user reloads the page
  - Then tool call cards are visible in the transcript
  - And expanding a tool card shows a structured args preview and a separate raw **Result** panel, without approval buttons
  - And if the server marked the preview truncated, **More…** then **Less** behave as in the table above; if not truncated, there is no overflow-toggle row and no **`tool-result-viewport--tall`** on the result panel

- Tool result truncation (Playwright MCP)
  - Given a persisted session whose tool output on disk exceeds the preview line cap
  - When the user opens the tool card and clicks **More…**
  - Then the button becomes **Less**, full lines are available inside the same max-height scrollable panel, and **`.tool-result-viewport--scroll`** has **`scrollHeight`** greater than **`clientHeight`**
  - When the user clicks **Less**
  - Then the preview shows the capped text ending in **`...`**, **`overflow-y`** is hidden on **`.tool-result-viewport--clip`**, and **More…** appears again

- Token usage survives restart
  - Given a session has non zero token usage
  - When the user reloads the page
  - Then the token usage HUD shows the persisted totals

- Memory copilot row (Playwright MCP)
  - Given **`memory.enabled: true`** on the **`foxxycode http`** process and at least one Markdown file under global or workspace memory so recall can run
  - When the user sends a chat message that completes a full ReAct turn
  - Then an element with **`data-testid="memory-copilot-row"`** appears after that user bubble for the turn (grey **memory** foldout, same visual language as **thinking** per `DESIGN.md`)
  - When the user opens the details element
  - Then the streamed **memory** body shows the text merged into the main agent prompt for that turn (and optional saved-note preview when the copilot wrote `foxxycode_memory_save`)

For Playwright MCP against a live gateway, start **`make build TAGS="http ui"`** then **`./build/foxxycode http`** with a disposable **`--home`** so config can enable memory; open **`http://127.0.0.1:<port>/`**, navigate to a session, send a prompt, assert the snapshot contains **memory-copilot-row** and folded body text after expand.
