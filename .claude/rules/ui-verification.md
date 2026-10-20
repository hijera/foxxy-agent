---
description: UI layout checks before claiming work is done
paths:
  - "external/ui/**/*"
---

# UI verification

Before you tell the user that a UI change is complete or merge-ready:

1. Run **`npm run test:layout`** in **`external/ui`** after **`npm ci`**. The test starts Vite on a free loopback port, requires readiness within **10 seconds**, checks **390px** and **1280px** viewports in headless Chrome/Chromium, and always stops both Vite and the browser. The browser phase has a **20-second** deadline and the outer Go test timeout is **45 seconds**, leaving cleanup headroom.
2. The test opens **`/layout-scroll-check.html`** and compares **`getBoundingClientRect()`** edges for **`.chat-header`**, **`.messages-inner`** first child, and **`.composer-card`**. Left and right must match within **1px**.
3. For feature-specific visual or interactive changes, also use Playwright MCP (or the repo browser tools) against **`/`** on a running FoxxyCode HTTP instance. Keep every navigation/evaluation bounded by the browser tool timeout; do not leave an unmonitored background Vite process.
4. If anything is off, fix CSS and re-check before reporting done.

This is required for changes to **`external/ui/src/styles.css`**, **`ChatScreen`**, **`Composer`**, or nav layout.

## Safari and other WebKit reports

A Safari-only report is reproducible without a Mac: Playwright's WebKit is the engine Safari is cut
from and its version tracks Safari's. When the change touches a capped, scrollable or **`position:
fixed`** surface, run **`external/ui/scripts/webkit-scroll-check.mjs`** against a live
**`foxxycode http`** (setup and env vars in **`docs/surfaces/web-ui.md`**, *Reproducing a Safari report without a
Mac*), and run it again with **`FOXXYCODE_ENGINE=chromium`** to tell a WebKit-only regression from a
layout bug every engine shares.

## The transcript at every width of the grid

Nothing in the transcript may be wider than the transcript: on the stacked shell its column is the
page, and one row that cannot wrap makes a phone scroll sideways (**`DESIGN.md`**, *Layout grid*,
*Tool card UI*, *Markdown*). When the change touches the transcript's rows, the Markdown styles or a
width query, run **`external/ui/scripts/phone-overflow-check.mjs`** against a **`vite`** dev server
(setup in **`docs/surfaces/web-ui.md`**, *Checking the transcript at every width of the grid*): it
mounts the rows that used to overflow and fails when the page scrolls sideways or anything sticks out
of **`.messages-inner`** at 360 to 1280px. A new width query goes into the grid first; the vitest
**`layoutGridCss.test.ts`** fails on one the grid does not name.

## The fold chevron

The chevron is an SVG whose ink is centred in its viewBox, never a text glyph: a glyph's ink moves
with the platform's font, which is why the reports of a chevron riding above its label kept coming
back (**`DESIGN.md`**, *Chevron*). jsdom has no layout, so vitest cannot see where it lands. When the
change touches the chevron, the rows it sits on or the type around them, run
**`external/ui/scripts/chevron-align-check.mjs`** against a **`vite`** dev server (setup in
**`docs/surfaces/web-ui.md`**, *Checking the fold chevron against its label*): it measures the
chevron's ink centre against the label's on a transcript row and on the Tasks drawer toggle, and
fails past **1px**.

## A long transcript

Only a bounded slice of a transcript is in the DOM, and a long session holds only the end of its
history until the reader scrolls up (**`DESIGN.md`**, *Transcript window*; **`docs/surfaces/web-ui.md`**,
*Long sessions*). jsdom has no layout, so vitest renders every row and cannot see the window at work.
When the change touches the transcript window, the rows, the messages or tool-calls routes, or what a
row renders, run **`external/ui/scripts/transcript-window-check.mjs`** against a binary built with
**`TAGS="http ui swarm"`** (setup in **`docs/surfaces/web-ui.md`**, *Checking a long transcript*): it
opens a 3306-message session under CPU throttling, scrolls it without a jump, edits a prompt of an
older page, runs a turn in a second browser, reads through a swarm relay, and fails past a budget.
Run it once more with **`FOXXYCODE_ENGINE=webkit`** when the change touches scroll anchoring: WebKit does
not anchor scrolling, so there the window's own correction is all that keeps the reader's row still.
