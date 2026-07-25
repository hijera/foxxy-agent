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
