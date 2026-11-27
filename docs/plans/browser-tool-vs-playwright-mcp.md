# Built-in browser interactions and Playwright MCP

Historical design record, September 2026. This records the motivation for expanding
`internal/tools/browser`, rather than a current feature inventory of Playwright MCP.
The supported FoxxyCode tools are documented in [Browser tool](../features/browser-tool.md).

## Starting point

FoxxyCode's original eleven tools could navigate, click, fill, hover, scroll,
evaluate JavaScript, take screenshots, read a page outline, inspect storage and
timing, read the page log and close the browser. They used chromedp and a local
Chrome installation without a Node.js process or an MCP server.

The browser belongs to an agent session. Its profile persists in that session's
directory, actions use a configured timeout, and mutating actions require the
same permission mechanism as the other agent tools. Action results include the
URL, page errors and an optional screenshot passed directly to the model.
`browser.screenshots: false` supports models that need text instead.

The original comparison with [Playwright MCP](https://github.com/microsoft/playwright-mcp)
identified gaps in ordinary verification work: forms submitting on Enter,
content appearing asynchronously, links opening another tab, native dropdowns
and JavaScript dialogs. Requiring the model to reconstruct these interactions
through `evaluate` made common tasks unnecessarily difficult.

## Interaction additions

The chosen approach extends chromedp without adding another browser stack:

1. `press_key` sends real keyboard events, including Enter, Escape and modifiers.
2. `wait_for` waits for page text, its disappearance, a selector or a delay.
3. `tabs` lists, opens, selects and closes pages, including site-created tabs.
4. `select_option` lists options and selects by value or visible label.
5. `handle_dialog` arms a one-shot answer before an action opens a dialog.
   Unarmed dialogs are answered automatically so the action can finish.
6. `navigate_back` moves backward or forward through history.
7. `upload_file` attaches files to a file input.
8. `resize` sets viewport dimensions for narrow and wide checks.
9. `drag` sends mouse events between selectors or viewport points.
10. `fill_form` changes several form fields in one action.
11. `find` searches the text outline for matching elements.
12. `network` journals requests and exposes response bodies; `route` supplies
    canned responses and toggles offline mode.
13. `storage_set` changes localStorage, sessionStorage and cookies. Coordinate
    clicks also extend the existing `click` tool for canvases and maps.

## Boundaries and deferred ideas

The page outline remains based on CSS selectors and a script running inside the
page. Stable accessibility references, code generation, reusable assertions,
tracing, video, PDF output and Firefox/WebKit support were outside this change.
Attaching to a user's existing browser, device emulation, proxy settings,
origin restrictions, secret masking and configurable selector conventions were
also left for separate decisions.

Playwright MCP remains an optional server an operator can configure through the
existing MCP integration. It is useful when a workflow needs that ecosystem;
FoxxyCode's built-in tools keep their session lifecycle, permission checks,
automatic page-error reporting and optional screenshots.
