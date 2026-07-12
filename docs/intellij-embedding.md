# Embedding the UI in an IntelliJ / PhpStorm plugin (JCEF)

The foxxycode-agent web UI is designed to run inside JCEF (the Chromium browser
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
`data-theme` attribute on `<html>`, persisted in the `foxxycode_ui_theme`
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

### 2. `window.foxxycodeUi` JS API (live switching)

Registered by the SPA at startup (`external/ui/src/ui/theme/foxxycodeUiApi.ts`):

```ts
window.foxxycodeUi: {
  version: 1;
  setTheme(theme: string): boolean;   // applies + persists; false on unknown ids
  getTheme(): string;                 // currently applied theme id
  getThemes(): string[];              // all valid ids, display order
  onThemeChange(cb: (theme: string) => void): () => void; // returns unsubscribe
  setLocale(locale: string): boolean; // "en" | "ru"; applies + persists; false on unknown ids
  getLocale(): string;                // currently applied locale id
  onLocaleChange(cb: (locale: string) => void): () => void; // returns unsubscribe
}
```

`setTheme` goes through the same code path as the in-UI theme picker: it
updates `data-theme`, `color-scheme`, the cookie, and every subscribed React
component re-renders.

`setLocale` updates `<html lang>`, the `foxxycode_ui_lang` cookie, and re-renders
every component that uses the i18n provider (same path as changing language in
Settings | Tools | FoxxyCode in the plugin).

## UI language (`?lang=` and `setLocale`)

Supported SPA locales: `en` (default), `ru`. The active locale is the `lang`
attribute on `<html>`, persisted in the `foxxycode_ui_lang` cookie.

**Single source of truth:** the UI language is stored once, in the backend
config (`ui.locale` in `config.yaml`, values `""` = auto, `"en"`, `"ru"`), and
edited from one place only — the SPA **Settings → General** language picker. The
IntelliJ and VS Code plugins no longer have their own language setting; they read
`ui.locale` from the backend and follow live changes made in the SPA.

### 1. `?lang=` query parameter (initial load, pre-first-paint)

```text
http://127.0.0.1:<port>/?theme=dark&lang=ru&embed=intellij
```

Precedence: query parameter > cookie > default (`en`). A valid query value is
applied before the first paint and written to the cookie.

On server start the plugin fetches `GET /foxxycode/config`, reads `ui.locale`,
and resolves the `?lang=` value: an explicit `en`/`ru` from the config, otherwise
the host default (`Locale.getDefault()`, Russian when the JVM default language is
`ru`). Passing `?lang=` on load keeps the SPA's first paint and the plugin chrome
in agreement even in auto mode — the URL param intentionally beats the SPA cookie,
while the SPA's own picker still shows "Auto" because it renders the config value,
not the active locale.

### 2. Live switching, both directions

- **Plugin → SPA:** when the plugin locale changes (config re-read on restart, or
  a change relayed from the SPA), it injects
  `window.foxxycodeUi.setLocale('<en|ru>')` after each page load. The SPA updates
  without a full reload.
- **SPA → plugin:** when the user flips the language in **Settings → General**, the
  SPA notifies the host. In JCEF the plugin subscribes via
  `window.foxxycodeUi.onLocaleChange(...)` wired to a `JBCefJSQuery`; the callback
  adopts the new locale and publishes `FoxxyCodeLanguageListener.TOPIC` so the
  toolbar and any status/error panels re-localize. (In the VS Code webview, where
  the SPA runs in a cross-origin iframe, the SPA `window.parent.postMessage`s the
  same payload — see below.)

`setLocale` is a no-op when the locale is unchanged (it does not notify
listeners), so the plugin→SPA→plugin round-trip cannot loop.

**VS Code postMessage contract** — the SPA (`embedLocaleBridge.ts`) posts to the
webview wrapper, which forwards to the extension host:

```jsonc
{ "type": "foxxycode:locale", "locale": "en" | "ru" }
```

### Kotlin example (theme + locale bridge)

```kotlin
import com.intellij.ide.ui.LafManagerListener
import com.intellij.ui.JBColor
import com.intellij.ui.jcef.JBCefBrowser
import com.intellij.ui.jcef.JBCefBrowserBase
import com.intellij.ui.jcef.JBCefJSQuery

fun ideTheme(): String = if (JBColor.isBright()) "light" else "dark"

val browser = JBCefBrowser("http://127.0.0.1:$port/?theme=${ideTheme()}&lang=${spaLang()}")

// Follow IDE theme changes (Settings > Appearance, quick switch, etc.).
project.messageBus.connect(disposable).subscribe(
    LafManagerListener.TOPIC,
    LafManagerListener {
        browser.cefBrowser.executeJavaScript(
            "window.foxxycodeUi && window.foxxycodeUi.setTheme('${ideTheme()}')",
            browser.cefBrowser.url,
            0,
        )
    },
)

// SPA → plugin: adopt locale changes made in the SPA Settings → General picker.
val localeQuery = JBCefJSQuery.create(browser as JBCefBrowserBase)
localeQuery.addHandler { locale -> adoptLocale(locale); null }
// After each page load, subscribe once:
//   window.foxxycodeUi.onLocaleChange(function (l) { ${localeQuery.inject("l")} });
```

Any of the 7 theme ids can be substituted for `light`/`dark` — e.g. map the
IDE's Darcula to `midnight` if that fits the plugin's visual language
better.

## Embed mode (`?embed=intellij`)

Pass `&embed=intellij` on the initial URL to opt the SPA into a flatter, more
native host-IDE look. The SPA mirrors the value into
`<html data-embed="intellij">` (validated as `[a-z0-9_-]+`, lowercased) before
first paint, and CSS overrides keyed on `[data-embed="intellij"]` then:

- flatten the composer card (6px radius, solid 1px border, no frosted-glass
  halo or backdrop blur) so it reads as an IDE input field;
- drop the docked vignette above the composer;
- tighten hero/composer spacing.

Only the visual chrome changes; behaviour and the `window.foxxycodeUi` theme
contract are unchanged. Other embeddings may pass their own id, but
`intellij` is the only id the shipped CSS currently specialises.

```text
http://127.0.0.1:<port>/?theme=dark&lang=ru&embed=intellij
```

## Verifying against real Chromium 104

Playwright 1.24 bundles Chromium 104. To smoke-test the built UI without an
IDE:

```bash
cd external/ui && npm run build:go
# in a scratch directory:
npm i playwright@1.24 && npx playwright install chromium
# drive http://127.0.0.1:<port>/ served by `foxxycode http` (build tag "http ui")
```

## Embedding the UI in a VS Code extension (webview)

The VS Code extension at `editors/vscode/` follows the same contract: bundle a full-feature
`foxxycode` binary, start `foxxycode http --cwd <workspace>` on a free port, and embed the SPA.
Differences from the IntelliJ embedding:

- **Host element:** VS Code webviews load external URLs only via an `<iframe>` inside the webview
  HTML. The extension cannot `executeJavaScript` into a cross-origin iframe (unlike JCEF), so live
  theme switching is done by reloading the iframe with an updated `?theme=` parameter. Initial load
  is still flash-free thanks to `?theme=` being applied before first paint.
- **Locale:** like IntelliJ, the extension has no language setting — it reads `ui.locale` from the
  backend config on start and resolves `?lang=`. SPA-driven changes come back via the
  `{ type: "foxxycode:locale", locale }` `postMessage` the iframe sends to the webview wrapper,
  which forwards it to the extension host; the host adopts the locale and refreshes command titles
  via the `foxxycode.locale` context key, without reloading the iframe.
- **CSP:** the webview HTML sets `frame-src http://127.0.0.1:* http://localhost:*;` so the iframe
  can load the loopback foxxycode server on its auto-picked port.
- **Embed id:** the extension passes `?embed=intellij` because the SPA currently specialises only
  that id in CSS. A dedicated `embed=vscode` id and matching CSS overrides are a TODO.
- **Native inline diffs:** the extension host subscribes to `GET /foxxycode/ide/events` (Node `http`
  SSE reader) and renders decorations via `vscode.window.createTextEditorDecorationType`, with
  Accept/Reject/Revert/Show-diff notifications posting to
  `/foxxycode/sessions/<id>/permission` and `vscode.diff` respectively. This is the direct peer of
  the IntelliJ `FoxxyCodeIdeDiffService`.

The `?theme=`, `?lang=`, and `window.foxxycodeUi` contracts described above are unchanged.
