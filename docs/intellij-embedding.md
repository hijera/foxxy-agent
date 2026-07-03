# Embedding the UI in an IntelliJ / PhpStorm plugin (JCEF)

The foxxy-agent web UI is designed to run inside JCEF (the Chromium browser
embedded in JetBrains IDEs) as part of an IntelliJ IDEA / PhpStorm plugin.

## Supported browser baseline

| Component | Version |
| --- | --- |
| Minimum IDE | PhpStorm / IntelliJ IDEA **2022.3.3** |
| JetBrains Runtime | JBR 17.0.6 |
| JCEF | 104.5.2 |
| **Chromium** | **104** |

The frontend build targets Chromium 104 (`build.target` / `build.cssTarget`
in `external/ui/vite.config.ts`). Two build steps enforce the baseline:

- `external/ui/postcss-resolve-color-mix.mjs` — resolves every
  `color-mix()` (Chromium 111+) to precomputed per-theme values at build
  time; the build fails on expressions it cannot resolve.
- `external/ui/scripts-check-chromium104.mjs` — scans the built
  `dist/styles.css` and `dist/app.js` (including dependency code) for CSS/JS
  features newer than Chromium 104 and fails `npm run build:go` on findings.
  Run standalone with `npm --prefix external/ui run check:compat`.

Contributor rule: do not use CSS or JS features newer than Chromium 104 in
the shipped UI (details in `.claude/rules/ui-spa.md`). Notable off-limits
features: `:has()`, `oklch()`/`oklab()`, `@container`, native CSS nesting,
`Array.prototype.toSorted`, `Promise.withResolvers`, `URL.canParse`.
`dvh`/`svh` units are allowed only with a preceding `vh` fallback
declaration for the same property.

## Serving the UI to JCEF

Run the agent's HTTP server and point `JBCefBrowser` at it:

```text
http://127.0.0.1:<port>/
```

Use `127.0.0.1` (or `localhost`) — the UI calls `crypto.randomUUID()`, which
requires a trustworthy origin. Loopback HTTP qualifies; a non-loopback plain
HTTP host does not.

## Theme integration

The UI has 7 built-in themes: `dark` (default), `light`, `midnight`,
`solarized-dark`, `monokai`, `nord`, `rose-pine`. The active theme is the
`data-theme` attribute on `<html>`, persisted in the `coddy_ui_theme`
cookie.

JCEF does not propagate the IDE look-and-feel to `prefers-color-scheme`, so
the plugin drives the theme explicitly through two mechanisms:

### 1. `?theme=` query parameter (initial load, pre-first-paint)

```text
http://127.0.0.1:<port>/?theme=dark
http://127.0.0.1:<port>/?theme=light#/s/<sessionId>
```

Accepted values: any of the 7 theme ids. Precedence: query parameter >
cookie > default (`dark`). A valid query value is applied before the first
paint (no flash) and written to the cookie so later loads without the
parameter keep the theme.

Cookie persistence inside JCEF depends on the plugin's client/cache
configuration, so **always pass `?theme=` on load** and use the JS API below
for live switching; the UI stays themed even when cookies are not persisted.

### 2. `window.foxxyUi` JS API (live switching)

Registered by the SPA at startup (`external/ui/src/ui/theme/foxxyUiApi.ts`):

```ts
window.foxxyUi: {
  version: 1;
  setTheme(theme: string): boolean;   // applies + persists; false on unknown ids
  getTheme(): string;                 // currently applied theme id
  getThemes(): string[];              // all valid ids, display order
  onThemeChange(cb: (theme: string) => void): () => void; // returns unsubscribe
}
```

`setTheme` goes through the same code path as the in-UI theme picker: it
updates `data-theme`, `color-scheme`, the cookie, and every subscribed React
component re-renders.

### Kotlin example

```kotlin
import com.intellij.ide.ui.LafManagerListener
import com.intellij.ui.JBColor
import com.intellij.ui.jcef.JBCefBrowser

fun ideTheme(): String = if (JBColor.isBright()) "light" else "dark"

val browser = JBCefBrowser("http://127.0.0.1:$port/?theme=${ideTheme()}")

// Follow IDE theme changes (Settings > Appearance, quick switch, etc.).
project.messageBus.connect(disposable).subscribe(
    LafManagerListener.TOPIC,
    LafManagerListener {
        browser.cefBrowser.executeJavaScript(
            "window.foxxyUi && window.foxxyUi.setTheme('${ideTheme()}')",
            browser.cefBrowser.url,
            0,
        )
    },
)
```

Any of the 7 theme ids can be substituted for `light`/`dark` — e.g. map the
IDE's Darcula to `midnight` if that fits the plugin's visual language
better.

## Verifying against real Chromium 104

Playwright 1.24 bundles Chromium 104. To smoke-test the built UI without an
IDE:

```bash
cd external/ui && npm run build:go
# in a scratch directory:
npm i playwright@1.24 && npx playwright install chromium
# drive http://127.0.0.1:<port>/ served by `coddy http` (build tag "http ui")
```
