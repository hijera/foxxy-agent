# Browser scroll window

Captured from the running React browser-tool fixture in Chromium at 390 px and
1280 px, with the same scroll action before and after the window-frame change.
The border encloses both the directional arrow and its signed X/Y offsets; the
decorative toolbar identifies the rectangle as a browser window.

| Theme and viewport | Before | After |
| --- | --- | --- |
| Light, 390 px | [Before](before/browser-6-light-390.png) | [After](after/browser-6-light-390.png) |
| Dark, 390 px | [Before](before/browser-6-dark-390.png) | [After](after/browser-6-dark-390.png) |
| Light, 1280 px | [Before](before/browser-6-light-1280.png) | [After](after/browser-6-light-1280.png) |
| Dark, 1280 px | [Before](before/browser-6-dark-1280.png) | [After](after/browser-6-dark-1280.png) |

Reproduce with `FOXXYCODE_BROWSER_SCREENSHOTS` set to an output directory:

```sh
go test -tags=http,ui,browser ./external/ui -run '^(TestBrowserToolPresentationFeature|TestUILayoutColumnsAlign)$' -count=1 -timeout=100s
```
