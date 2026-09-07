# UI reference images

This folder contains reference screenshots used to align the embedded UI with the target design.

## Navbar (RPA-style references, May 2026)

Implementation note: **FoxxyCode does not render a circle or logo glyph** before the **FoxxyCode agent** brand in the embedded SPA. SVG logos under **`foxxycode-logo-*.svg`** are for README, **`logo-preview.html`**, and favicon (**`foxxycode-favicon.svg`** aliases **`foxxycode-logo-mark-flat.svg`**, same asset as [foxxycode.dev](https://foxxycode.dev/) **`assets/foxxycode-favicon.svg`**). Raster favicons **`favicon-32.png`**, **`favicon.ico`**, **`apple-touch-icon.png`** ship with the embedded SPA at the site root. **`foxxycode-logo-mark-icon.svg`** is square full-bleed plate fill with no rim stroke or corner radius; **`foxxycode-logo-mark-icon-2048.png`** is a 2048×2048 raster export; **`foxxycode-logo-social.svg`** (1280×640) is the GitHub repository social preview with wordmark and tagline, with **`foxxycode-logo-social-1280x640.png`** and **`foxxycode-logo-social-640x320.png`** raster exports; **`foxxycode-logo-mark.svg`** adds halo filters. Some references still show a circle, treat it as layout inspiration only.

- `ref-navbar-narrow-tooltips-accent.png` - narrow vertical rail, tooltips right, purple hover on icon
- `ref-navbar-narrow-icons-only.png` - narrow rail, icons only (FoxxyCode uses History + GitHub + API, not News or Projects)
- `ref-navbar-wide-with-labels.png` - wide rail with text labels next to items

## Playwright MCP (verification, May 2026)

Captured from local `vite` + `foxxycode http` with `FOXXYCODE_UI_BACKEND`.

- `pw-navbar-1440-narrow.png` - desktop under 1920px width, narrow rail (no widen toggle), no burger
- `pw-navbar-1440-history-hover.png` - History hover / pressed accent and tooltip styling
- `pw-navbar-1920-wide-labels.png` - min-width 1920px, wide rail (**rectangular panel**, rounded on the right only), header with **collapse** (stacked lines) plus **FoxxyCode agent** text-only brand, full-width rows icon plus label
- `pw-navbar-1920-github-hover.png` - wide rail, hover on **GitHub** row (label plus icon pick up accent)
- `pw-navbar-390-mobile-topbar.png` - max-width 1199px shell, rail as top bar row
- `pw-navbar-390-sessions-drawer.png` - History opens chats drawer overlay

## Full HD tour (README, May 2026)

Captured at **1920×1080** via Playwright MCP (`vite` + `foxxycode http`, `FOXXYCODE_UI_BACKEND`).

- `screenshot-fullhd-start.png` - new chat / hero start screen (README, above fold)
- `screenshot-fullhd-chat.png` - active session transcript (`#/s/...`)
- `screenshot-fullhd-history.png` - History drawer on a session (`#/s/...?history=1`)
- `screenshot-fullhd-scheduler.png` - scheduler list plus job editor (`#/scheduler/jobs/nightly-docs`)
- `screenshot-fullhd-settings.png` - settings drawer (`#/settings`)

## Primary

- `ref-home-1.png` - landing page with collapsed left rail and centered composer
- `ref-home-composer.png` - expanded left menu and composer action area
- `ref-chat.png` - in chat view with floating composer and left rail
- `ref-wide-1.png` - wide desktop layout with expanded left nav and sessions list
- `ref-wide-2.png` - wide desktop layout variant
- `ref-wide-3.png` - wide desktop layout with session context menu

## Mobile

- `ref-image-098475fd-f1e8-4722-9975-67890f85a2c8.png` - mobile rail states and expanded menu

## ConfirmDialog (verification)

Captures of the shared confirmation dialog that replaced the native `confirm()`.
Ported from upstream with the wave `0f2dbf1 → fa7ecf1`, so they show that build's
English UI rather than a FoxxyCode capture; the fork's own run is the source of
truth for how the dialog reads in Russian.

- `pw-confirm-delete-chat-1280-{dark,light}.png` - delete a persisted chat
- `pw-confirm-delete-chat-390-dark.png` - the same at the narrow breakpoint
- `pw-confirm-delete-draft-1280-dark.png` - delete a client-side draft
- `pw-confirm-escape-{before,after}-1280-dark.png` - Escape cancels the dialog without collapsing the drawer underneath
- `pw-confirm-history-before-1280-dark.png` - the sessions list the dialog returns to
- `pw-confirm-scheduler-before-1280-dark.png` - the scheduler job sheet before deleting
- `pw-confirm-scheduler-delete-1280-{dark,light}.png` - delete a scheduler job

## Upstream wave fa7ecf1 -> 12897ba (PR #34)

Captured from a headless Chrome against a real `-tags "http ui"` build; the
`-before-` frames come from a second build at `origin/main`, so each pair is a
readable visual diff. The UI is Russian because that is what the host locale
resolves to.

- `pr-34-appearance-{before,after}-1280-{dark,light}.png` - the language select lands under the theme grid
- `pr-34-appearance-{before,after}-390-dark.png` - the same at the narrow breakpoint
- `pr-34-general-{before,after}-1280-dark.png` - language leaves the General tab; send mode and status line stay
- `pr-34-composer-attachment-1280-{dark,light}.png` - a pasted image as a chip with a live thumbnail
- `pr-34-composer-attachment-390-dark.png` - the same at the narrow breakpoint
- `pr-34-composer-attach-refused-1280-dark.png` - the paste refused by a model without `multimodal: true`
- `pr-34-bubble-thumbnail-1280-{dark,light}.png` - the sent bubble carrying the persisted thumbnail

## Model vision flag (PR #37)

Captured from a headless Chrome against a real `-tags "http ui"` build serving a
stub catalog that publishes both advertised shapes (`capabilities.vision` and
`modalities.input`); the `-before-` frames come from a second build at `d66a8d3`,
so each pair is a readable visual diff. English unless the name says `ru`.

- `pr-37-settings-{before,after}-1280-dropdown.png` - Fetch models badges the entries that accept images
- `pr-37-settings-{before,after}-1280-vision-picked.png` - picking a vision model ticks Multimodal and explains why
- `pr-37-settings-{before,after}-1280-textonly-picked.png` - picking a text-only model clears it again
- `pr-37-settings-{before,after}-390-dropdown.png` - the same list at the narrow breakpoint
- `pr-37-settings-{before,after}-1280-ru-vision-picked.png` - the Russian strings for the same edit
- `pr-37-onboarding-{before,after}-1280-dropdown.png` - the provider picker's model list, badged the same way
- `pr-37-onboarding-{before,after}-1280-vision-picked.png` - the hint naming the flag the config will be saved with
- `pr-37-onboarding-{before,after}-1280-textonly-picked.png` - the same for a model the catalog lists as text-only
- `pr-37-onboarding-{before,after}-390-dropdown.png` - the dialog at the narrow breakpoint
- `pr-37-onboarding-{before,after}-1280-ru-vision-picked.png` - the Russian strings for the same pick

## Upstream wave 7123d25a -> 2c0872c4 (PR #56)

Captured from a headless Chrome (chromedp) against the live `-tags "http ui cli memory"`
build on `neuraldeep`, so the transcripts show real subagent and todo tool calls. Dark is
the default theme; light frames cover the surfaces that gained colours.

- `pr-56-tasks-agent-row-1280-{dark,light}.png` - Tasks panel: an `agent general` row with the «Агент» badge, `spawn_agent` call expanded with its CDATA report
- `pr-56-tasks-agent-detail-1280-dark.png` - task detail with the subagent name and «Открыть транскрипт»
- `pr-56-subagent-readonly-1280-{dark,light}.png` - a child `sub_…` transcript: read-only notice in place of the composer, «Открыть родительский чат»
- `pr-56-settings-subagents-1280-{dark,light}.png` - Settings → Субагенты (enable switch, definition dirs, project trust policy)
- `pr-56-settings-providers-neuraldeep-1280-dark.png` - NeuralDeep provider card with the API endpoint select
- `pr-56-settings-models-reasoning-{before,after}-1280-dark.png` - «Получить уровни ризонинга» before the fetch and with `low/medium/high` written back
- `pr-56-settings-skills-switchfield-1280-dark.png`, `pr-56-settings-tools-switchfield-1280-dark.png` - boolean settings on the shared SwitchField
- `pr-56-todo-preview-1280-{dark,light}.png` - `foxxycode_todo_item_update` calls rendered as localized todo cards
## Batch uploads

Files named `ref-image-*.png` are direct uploads from chat. They are kept as source of truth.

## Onboarding gate fix (September 2026)

Captured from headless Chrome at 1280×860 (dark, `?lang=ru`) against two live `http,ui` builds: `before` is the pre-fix `main`, `after` this branch. Config: a keyed `neuraldeep` agent provider plus a keyless `openai` row; the flow retypes `openai` to NeuralDeep in Settings and saves.

- `pr-onboarding-settings-load-{before,after}-1280-dark.png` - Settings on page load: before, the provider picker already covers it; after, no picker
- `pr-onboarding-retyped-unsaved-{before,after}-1280-dark.png` - the `openai` row retyped to NeuralDeep, not saved: before, the sign-in block shows «provider is not a NeuralDeep provider»; after, the Sign In button alone
- `pr-onboarding-after-save-{before,after}-1280-dark.png` - right after Save: before, the picker reopens over Settings; after, the saved banner and the sign-in block stay
- `pr-onboarding-home-reload-{before,after}-1280-dark.png` - the start screen after a reload: before, the picker; after, the composer

## VS Code embed id (September 2026)

Captured from headless Chrome (dark, `?lang=ru`) against the Vite dev server on this branch. The VS Code extension used to load the SPA with `?embed=intellij`; it now sends `?embed=vscode`, and the flat composer chrome is keyed on both ids, so `before` and `after` must look the same.

- `pr-vscode-embed-before-intellij-id-{1280,390}-dark.png` - the start screen with `?embed=intellij` (what the extension sent before)
- `pr-vscode-embed-after-vscode-id-{1280,390}-dark.png` - the same screen with `?embed=vscode` (what it sends now): flat 6px composer, no glass halo, identical to the intellij id
- `pr-vscode-embed-plain-browser-1280-dark.png` - no embed id, for contrast: the browser keeps the rounded glass composer
