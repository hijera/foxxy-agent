# Brand assets

Everything FoxxyCode is recognised by comes from one drawing: an orange fox head with `{/}` on its
forehead. It appears as the browser tab icon of the web UI and the website, the social preview of
every shared link, the Windows executable and desktop window icon, and the icon of both editor
plugins.

One vector is the source of all of it, and `make brand` regenerates every file that derives from it.
Nothing here is built during an ordinary build or in CI - the outputs are committed.

## The source chain

| File | What it is |
|------|------------|
| [`docs/assets/brand/foxxycode-mark-1024.png`](../assets/brand/foxxycode-mark-1024.png) | The raster master, 1024x1024 on transparency. The drawing as it was first made; kept for provenance and as the thing the vector is checked against. |
| [`docs/assets/foxxycode-logo-glyph.svg`](../assets/foxxycode-logo-glyph.svg) | **The source.** The master traced into three flat layers, with a viewBox tight around the drawing. Everything else is built from this file. |
| `docs/assets/brand.lock.json` | The hash of every source and of every file generated from it. |

The palette, measured from the master: orange `#FC7E05`, navy `#20313F`, white `#FFFFFF`. The dark
plate the mark sits on keeps the interface accent, violet `#9333EA` (see [Web UI design](../../DESIGN.md)).

## Regenerating

```bash
make brand   # every asset below, plus the lock file
make icon    # the same, then the Windows .exe resource through rsrc
```

Both need a local Chrome: the generator, `scripts/brand/gen.go`, rasterises through chromedp. It

1. injects the glyph into every SVG that marks a slot for it, between
   `<!-- foxxycode:glyph:start width="…" -->` and `<!-- foxxycode:glyph:end -->`, so no file carries
   its own copy of the drawing;
2. renders the compositions and assembles `favicon.ico` and `build/foxxycode.ico`;
3. writes the lock.

A slot may ask for `mode="mono"`, which merges the silhouette and the white markings into one
`evenodd` path in a single colour - what an editor's activity bar and tool window require, and what
stays legible at 13 and 16 pixels.

### When the artwork itself changes

Replace the master and re-trace it before running the generator:

```bash
python scripts/brand/trace.py docs/assets/brand/foxxycode-mark-1024.png docs/assets/foxxycode-logo-glyph.svg
```

The tracer needs Pillow, numpy and `potracer` (`pip install potracer`) and is the one step outside
the Go toolchain; its output is committed, so it runs only when the drawing does change. It snaps
every pixel to the three brand colours and traces each layer cumulatively - the navy layer carries
the whole silhouette, orange carries orange plus white, white carries only itself - because tracing
them as disjoint regions leaves hairline transparent seams where two independently smoothed curves
fail to meet.

## What is generated

| File | From | Used by |
|------|------|---------|
| `docs/assets/foxxycode-logo-mark.svg` | the glyph | the mark with a halo, for documentation |
| `docs/assets/foxxycode-logo-mark-flat.svg` | the glyph | the mark on the dark plate, for documentation |
| `docs/assets/foxxycode-favicon.svg` | the glyph | the browser tab icon of the website and the SPA. The only file that carries a `prefers-color-scheme` media query: a tab strip is light or dark by the reader's own setting, so the plate follows it while the fox stays orange. The PNG and ICO beside it keep the dark plate - browsers old enough to need them ignore `media` on an icon link |
| `docs/assets/foxxycode-logo-mark-icon.svg` | the glyph | the square full-bleed plate every raster icon is rendered from |
| `docs/assets/foxxycode-logo-mark-light.svg` | the glyph | the mark with no plate, for light surfaces |
| `docs/assets/foxxycode-logo-wordmark.svg`, `-wordmark-light.svg` | the glyph | the sign-in screen and `DESIGN.md` |
| `docs/assets/foxxycode-logo-social.svg` | the glyph | the source of the social preview, in light tones: a shared link renders on somebody else's surface with no theme to follow |
| `docs/assets/foxxycode-logo-social-1280x640.png`, `-640x320.png` | the social SVG | `og:image` and `twitter:image` of every page of the website |
| `docs/assets/favicon-32.png`, `favicon.ico`, `apple-touch-icon.png` | the icon plate | the website and the embedded SPA |
| `external/ui/public/*` | the four favicons above | Vite's input; `make ui-build` copies them on to the `external/ui` root for `go:embed` |
| `editors/vscode/media/foxxycode.svg` | the glyph, mono | the VS Code activity bar |
| `editors/vscode/media/foxxycode-color.svg` | the glyph | the walkthrough, which renders it as an `<img>` where `currentColor` would fall back to black |
| `editors/vscode/media/icon.png` | the icon plate | the Marketplace listing (`icon` in `package.json`) |
| `editors/intellij/…/icons/foxxycodeToolWindow.svg`, `…_dark.svg` | the glyph, mono | the JetBrains tool window stripe, light and Darcula |
| `editors/intellij/…/META-INF/pluginIcon.svg` | the glyph | the JetBrains Marketplace listing and `Settings \| Plugins` |
| `build/foxxycode.ico` -> `cmd/foxxycode/rsrc_windows_amd64.syso` | the glyph | the `.exe` file icon and the desktop window |

## The guard

`internal/brand` holds the lock file's format and the test that reads it. The test re-hashes every
path the lock names and fails when one has moved on without the other.

This is not a hypothetical failure. The social preview the website served said **Coddy agent** for
months after the fork was renamed: `foxxycode-logo-social.svg` had been rebranded, its PNG export had
not, and nothing compared the two. Verifying hashes needs no browser, so the check runs in the
ordinary suite while the generator that produced the files needs Chrome.

When it fails, the fix is to run `make brand` and commit the result.
