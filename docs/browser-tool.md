# Interactive browser tool

FoxxyCode ships an optional interactive browser tool set that lets the agent drive a
real Chrome/Chromium instance: open pages, click, fill forms, hover, scroll, run
JavaScript, and — crucially — **see screenshots of the page**. Unlike `webfetch`
(which returns static HTML), the browser tool gives the model visual feedback after
every action, so it can verify web UIs, log into apps, and do visual end‑to‑end
checks.

It is built on [chromedp](https://github.com/chromedp/chromedp) (pure Go, over the
Chrome DevTools Protocol — no Node.js required) and is **disabled by default**. It is
gated behind the `browser` build tag and the `browser.enabled` config flag.

## Requirements

- A Chrome or Chromium binary installed on the machine. chromedp auto‑detects a
  common install; override the path with `browser.executable_path` if needed.
- A binary built with the `browser` build tag.

## Enable it

Build with the tag (combine with your other tags):

```bash
make build TAGS="http ui browser"
# or
go build -tags=http,ui,browser ./cmd/foxxycode
```

Turn it on in `config.yaml`:

```yaml
browser:
  enabled: true          # off by default
  headless: true         # false to watch the automated session in a real window
  executable_path: ""    # "" = auto-detect; or a path to a Chrome/Chromium binary
  timeout_seconds: 30     # per-action timeout
```

See [config-reference.md](config-reference.md#browser) for the full field list.

## Tools

All tools require permission (except `screenshot` and `close`) and act on one browser
session per agent session (cookies/storage persist under
`<sessionDir>/browser-profile/`). A screenshot is captured after each action and shown
to the model.

| Tool | Arguments | Purpose |
|---|---|---|
| `foxxycode_browser_navigate` | `url` | Open an `http(s)` URL (localhost allowed). |
| `foxxycode_browser_click` | `selector` | Click the first element matching a CSS selector. |
| `foxxycode_browser_fill` | `selector`, `text` | Set the value of an input/textarea. |
| `foxxycode_browser_hover` | `selector` | Move the mouse over an element (triggers `:hover`). |
| `foxxycode_browser_scroll` | `selector` or `x`,`y` | Scroll an element into view, or by a pixel offset. |
| `foxxycode_browser_screenshot` | — | Capture the current page. |
| `foxxycode_browser_evaluate` | `expression` | Evaluate JavaScript and return its JSON result. |
| `foxxycode_browser_close` | — | Close the browser for this session. |

## How the model sees screenshots

Tool results are text‑only, so screenshots are delivered to the model as a
**user‑role vision block** injected right after the browser tool round (reusing the
same image path as pasted images). The screenshot is also saved to the session
`assets/` directory and served to the UI via
`GET /foxxycode/sessions/{id}/assets/{name}` (see [http-api.md](http-api.md)), where
the transcript renders it inline in a browser‑action card.

## Security notes

- The navigate target must be `http`/`https` and must not embed userinfo
  credentials. Unlike the `webfetch` SSRF guard, localhost/private hosts **are**
  allowed, because driving a local dev server is the primary use case — the tool is
  already opt‑in behind the build tag and `browser.enabled`.
- Keep `browser.enabled: false` (or omit the `browser` build tag) in environments
  where you do not want the agent launching a browser.
