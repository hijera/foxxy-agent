# Coddy Agent UI Specification

Purpose: authoritative reference for the embedded SPA built from `external/ui/`. Tokens and layouts live here before CSS tweaks land in production stylesheets.

## Design references

Store the design reference images under `docs/ui/assets/` and link to the specific file when describing a pixel sensitive UI detail. Navbar parity with Cursor - style mockups lives in [`docs/ui/assets/INDEX.md`](docs/ui/assets/INDEX.md) (see Navbar section).

## Foundations

### Color

| Token | Hex | Usage |
|-------|-----|-------|
| background | `#121212` | main canvas |
| nav rail | `#252525` | icon rail |
| session list | `#1E1E1E` | secondary column |
| accent | `#9333EA` | actions, pills, emphasis |
| text primary | `#FFFFFF` | default copy |
| text muted | `#9CA3AF` | captions, timestamps |
| user bubble | `#2D2D2D` | outgoing chat |

### Typography and spacing

- System stack: **`system-ui`**, `-apple-system`, **`Segoe UI`**, **`sans-serif`**
- Comfortable padding: **`12px`** grid, radius **`12px`** (pill buttons **`999px`**)
- Density tuned for dashboards: desktop layout (single navigation style + fluid chat). Sessions are a drawer overlay.

### Token usage HUD

Muted footer row under composer shows **`input` / `output` / `total`** counts from streamed **`token_usage`** SSE payloads. Numbers update after each backend LLM pass (between tool executions), not per model token emitted over the wire.

Token usage totals are persisted per session and restored after restart.

## Layout

Left-to-right zones:

1. **Nav rail**: **History** opens the session list overlay, brand goes to the empty start screen; GitHub and **API docs** links (**API docs** opens **`/docs/`** in a **new browser tab**, same as GitHub **`target=_blank`** with **`rel=noopener`**). **Brand is text only** (**Coddy** plus **chat**), **no** circle or logo mark before the label, even if a reference mockup shows one. Optional **narrow vs wide rail** (**icons only vs icons plus labels**) on viewports **`min-width: 1920px`**, persisted in **`coddy_nav_rail`** cookie (**`narrow`** default).
2. **Session list**: **always a drawer overlay** with a dimming backdrop. It must **not** consume a second grid column or shrink the chat canvas (no inline sessions column beside the rail at any breakpoint). **Panel chrome title copy is History** (not "Chats"). There is **no** global hamburger that opens a separate app menu; the **stacked-lines control** in the wide rail header **only** collapses the rail to the narrow (icons-only) layout, matching the references.
3. **Chat canvas**: sticky header with editable title, scrollable transcripts, composer with **Send** (**Enter** submits, **`Shift+Enter`** newline).

The right insights rail is removed for the current milestone.

### Session identifier in URL

`#/s/<sessionId>` survives reload/share as long as the browser hits the **same Coddy http instance** backing the **`sessions`** root hash. SPA keeps **`X-Coddy-Session-ID`** synced with whichever id anchors the fragment.

### Responsive breakpoints

- Below **`min-width: 1920px`**: rail width toggle hidden; History opens the same **drawer + backdrop** as on larger viewports.
- At **`min-width: 1920px`**: user may widen the rail (**arrow**). Sessions remain a **drawer overlay** whether the rail is narrow or wide (wide rail changes label density only, not session placement).
- Mobile (**top bar**) keeps compact rail only; drawer for sessions history.

### Sessions drawer placement (implementation contract)

- **Horizontal alignment**: The **left edge** of the drawer is **`rail-column` right edge + gutter** (~**`--nav-floating-gutter`**). Do **not** hardcode **`left`** in **`px`** for "wide navbar" guesses (wide **`fit-content`** width varies).
- **Measured track width**: SPA sets **`--rail-shell-track-width`** on **`.shell`** to **`rail-column.offsetWidth`** (ResizeObserver in **`NavRail`**) before computing drawer **`left`** and **`width`** so narrow and labeled-wide rails stay flush with **`--nav-floating-gutter`** after the nav column.
- **CSS fallback**: When the variable is not yet set inline, **`--rail-shell-track-width`** defaults on **`.shell`** to **`calc(var(--rail-pill-track) + var(--rail-column-pad-end))`**.

### Narrow-rail hover tooltips

- Shown **only** when the rail is **narrow** (no wide labels column). Labels visible in wide rail substitute for tooltips; do not show floating tip rows there.
- **Copy**: brand area **New Chat**, **History** nav control **History**, external links match their labels. Reference accent chrome in **`docs/ui/assets/ref-navbar-narrow-tooltips-accent.png`**.
- **While a control owns an open overlay** (example **History** with the list visible and **`.is-active`** on the trigger), **hide that row's tooltip** even if the mouse still hovers (**nav stacking can sit above backdrop**).
- Tooltip **horizontal offset** must use the **same gutter math** as the History drawer (**column padding + nav floating gutter (+ border shim where needed)**), not a shorter offset from icon-only **`rail-tip-host`** width alone.

### Nav rail panel and wide layout (design contract)

- **Panel shape (desktop)**. The nav is a **tall rectangular column** along the **left viewport edge** with **rounding only on the right** (straight left edge flush with the browser). Avoid a centered **full-height capsule** that does not meet the edge.
- **Wide rail width**. Pill width is **content-driven** (**`fit-content`**) with a sensible **max-width** cap, not a legacy fixed pixel width guess.
- **Wide header brand**. **Coddy chat** is **one horizontal line** (**Coddy** + muted **chat**). Keep **breathing room to the right** of the label (**extra padding-right** on the brand control) so copy does not sit against the inner right edge of the panel.
- **Labeled rows (History, GitHub, API docs)**. Rows **share the same width** within the column (stretch to the **widest** row). Each row is a **single interactive surface** (icon + label), not a small icon hit target plus detached text.
- **Icon column alignment**. The first grid track for row icons matches the **collapse** toggle footprint (**44px** wide control). **Horizontal padding** on row hits stays **balanced** (avoid oversized **padding-left** and cramped **padding-right** at the label end).
- **Collapse vs global menu**. The **stacked-lines** control **only** narrows the rail. It is **not** a global app navigation drawer (see Nav rail item 2 above).

### Nav rail icons (implementation contract)

- **Collapse (hamburger glyph)**. Use **three equal-length** horizontal lines (**no** shorter third line). Prefer a **compact symmetric** **`viewBox`** (for example **20×20**), **round** line caps, and stroke weight that reads at **18px** output size.
- **Expand (narrow rail at XL)**. Keep a **chevron / chevron-pair** style control that reads as **widen rail**, not a second burger menu.
- **GitHub**. Use a **recognizable filled Octicon-style** mark (**`fillRule="evenodd"`** on a **24×24** path grid). **Do not** ship the old single-path **fractional-coordinate** silo that smears when scaled to **18px**.
- **Rendering**. Small rail SVGs should opt into **`shape-rendering` tuned for crisp curves** (for example **`geometricPrecision`**) and **`flex-shrink: 0`** so flex layout does not squash glyphs.
- **Regression art**. Use hover captures under **`docs/ui/assets/`** (for example **`pw-rail-icons-burger-hover.png`**, **`pw-rail-icons-github-hover.png`**) when checking outline and press states.

Sessions search uses **`GET /coddy/sessions?q=...`** (**title or first persisted user message substring only**); list uses infinite scroll toward older pages.

Desktop navigation wider than Full HD optionally shows labels on the expanded rail (**cookie** remembers preference).

Sessions list interactions

- Session list supports open on row click.
- Session list shows a small trash icon on hover.
- Renaming is done only in the chat header.

## Components

### Repo links

Repo links live in the nav rail: GitHub (**`https://github.com/coddy-project/coddy-agent`**) opens in a new tab; **API docs** points to **`/docs/`** and also opens in a **new tab** (embedded same-origin SPA still launches Swagger in a separate tab for convenience).

### Tool timeline

Captured via SSE (**`tool_call`**, **`tool_call_update`**). Rendered compact grey cards inside transcript.

Tool cards must include tool name, status, arguments, and result.

- Tool arguments arrive via `tool_call_update` status `in_progress` where `content[0].content.text` is raw JSON args.
- Tool result arrives via `tool_call_update` status `completed` or `failed` where `content[0].content.text` is the display result (possibly truncated by backend).

Tool call history is persisted per session under `tool_calls/` so it can be restored after restart.

### Transcript message types (technical)

The transcript UI is a flat list of message blocks (no nested threads). The runtime list lives in `external/ui/src/ui/chat/types.ts`.

Current block types:

- `user_message`
  - Raw user input text (Markdown allowed).
- `thinking`
  - Streaming model reasoning deltas (`delta.reasoning_content`) rendered as a disclosure row.
  - `thinking...` while in progress, `thinking` when completed.
  - **Summary row layout** - elapsed time stays immediately beside the **thinking** word, not pushed to the far right of the chat column. In **`ThinkingMessage.tsx`**, **`.thinking-dur`** nests inside **`.thinking-left`**; **`external/ui/src/styles.css`** sets **`.thinking-left { gap: 0 5px; }`** between the label and the timer. Do not use **`justify-content: space-between`** on **`summary.thinking-summary`** for that spacing. Avoid a wide summary flex that sends the timer to the opposite edge of the transcript.
  - Multiple `thinking` blocks can appear in one user turn. If the model resumes reasoning after tool calls, the UI starts a new `thinking` block and preserves ordering.
- `tool_call`
  - Tool execution timeline block (SSE `tool_call` and `tool_call_update`, enriched from `/coddy/sessions/{id}/tool-calls`).
  - Summary row stays compact; details show args and result.
  - Duration label is computed from persisted `tool_calls/<id>/meta.json` `startedAt` and `finishedAt` when available.
- `assistant_message`
  - Final assistant output for the turn. UI keeps it last and backfills it from `/coddy/sessions/{id}/messages` when streaming ends.

Ordering rules:

- `thinking` blocks appear wherever reasoning arrives in the stream.
- `tool_call` blocks appear where tool events arrive.
- Final `assistant_message` is appended after tools and any subsequent `thinking` blocks.

### Composer pill

Muted **Auto** pill tracks future modality toggles; UI copy stays English everywhere.

### Composer context meter

Ring to the **left** of **Send** in **`Composer.tsx`**.

- Do **not** put a percent label **on** the ring. Percentages and counters belong **only** in the tooltip (**`rail-tip`** family), above the ring, centered, wide enough via **`composer-context-tip`** CSS.
- Idle home (**`contextIdle`**): empty arc; tooltip **`No context usage yet`** plus **`Max context …`** only (no **`Model …`** line).
- Active session: arc fills from stats; the tooltip may include usage lines but **never** a **`Model …`** line that duplicates **Mode** (the mode dropdown).

See **`.cursor/rules/ui-spa.mdc`** for the full wording.

### Composer primary action (**Send** **/** **Stop**)

- Control **`#btn-send`** (**`.composer-icon`**) sits **directly right** of the context ring (**`.composer-context-tip-host`**).
- **Circular button** (**not** pill or squircle): fixed equal **width** and **height**, **`border-radius: 50%`**, **`box-sizing: border-box`**. Intended diameter **42px** in production CSS (**may track token scale**, but stays a **circle**).
- **Glyphs** live in **`composer-send-glyph`**. **Play** state uses **`~22px`** **▶**; **stop** uses **`~17px`** **■** (dense glyph, avoids clipping inside the circle). Keep contrast high (**`composer-send-play`** vs **`composer-send-stop`**).
- Idle **disabled** when message field empty; streaming shows **stop** affordance (see **`docs/ui/spec.md`**, section **Composer primary action**).

Composer mode selector

- **`GET /v1/models`** merges Coddy profiles and YAML backends in one list. Split by **`owned_by`**: **`coddy`** means session profiles **`agent`** and **`plan`** only. Any other **`owned_by`** marks a configured **`models[].model`** row (YAML backend).
- **`Mode`** lists only **`agent`** and **`plan`**. Default is **`agent`**. **`plan`** uses the orange outline treatment.
- Selected mode is sent as top-level **`model`** in **`POST /v1/responses`**.

Composer YAML **`models[].model`** selector

- **`Model`** sits immediately next to **`Mode`** in **`Composer.tsx`**. It lists only YAML backend rows (**`owned_by`** is not **`coddy`**). Opens **down** on the empty start screen (**`isEmpty`**) and **up** when docked over an active chat (same **`opens-down`** / **`opens-up`** convention as **`Mode`**).
- Default follows **`default_agent_model`** from **`GET /v1/models`** when that id is present among YAML rows; otherwise the first YAML row. Last choice persists in cookie **`coddy_llm_model`** (**`Path=/`**, long **`Max-Age`**) so **New chat** keeps the backend without forcing a re-selection.
- For ReAct (**`agent`** / **`plan`**), the UI sends **`metadata.model`** with the selected YAML **`id`**; the context-meter **`max_context_tokens`** for the ring follows that YAML row.

Composer does not show tools toggles in this milestone.

### Markdown

Messages may contain Markdown.

- Render fenced code blocks with syntax highlighting.
- Each code block has a copy button in the top right corner that copies only the block contents.

### Memory tree

Shows combined **global** (**`memory.dir`** / `$CODDY_HOME/memory`) and **workspace** (`<cwd>/memory`) hierarchies respecting backend filters (`.md` / `.txt` only).

Memory UI is removed for the current milestone.

### Component boundaries

The UI should be implemented as small React components with folder-enforced hierarchy.

- `ui/layout/Shell`
- `ui/nav/NavRail`
- `ui/sessions/SessionsSidebar`
- `ui/chat/ChatScreen`
- `ui/messages/MessageList`
  - `ui/messages/UserMessage`
  - `ui/messages/AssistantMessage`
  - `ui/messages/ToolCallMessage`

### Session overflow menu (`…`)

Opens lightweight rename/delete UX (prompt-first until richer modals arrive).

## States

- Idle composer: bordered textarea.
- Streaming assistant: progressively grows final assistant bubble; token HUD updates concurrently.
- Error streaming: surfaced inside assistant transcript with HTTP status text fallback.

## Non-goals for this milestone

Server-side SSR routes per session, BFF auth, CDN-hosted Swagger, and editing **`agentMemory`** via REST remain out-of-scope (`session.json` slot remains agent-managed).

## Dev workflow

To iterate on UI without rebuilding the Go binary:

Backend:

```bash
make build TAGS=http
./build/coddy http --config config.yaml --home /tmp/coddy-ui-dev-home --sessions-dir /tmp/coddy-ui-dev-sessions -H 127.0.0.1 -P 12345
```

Frontend:

```bash
npm --prefix external/ui install
npm --prefix external/ui run dev -- --host 127.0.0.1 --port 5173
```

