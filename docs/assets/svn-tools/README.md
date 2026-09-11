# SVN tool card verification

The screenshots render the production `ToolCallMessage` and `PermissionToolPreview` components with recorded SVN tool inputs and outputs. They do not execute repository mutations.

- `before/`: original generic tool presentation, expanded and collapsed, at 390px and 1280px in dark and light themes.
- `after/`: all thirteen SVN operations, in-progress and cancelled calls, an SVN error returned with completed transport status, and a commit permission preview, at both widths and in both themes.
- Individual `svn-1`, `svn-2`, `svn-3`, and `svn-4` images show status, diff, commit, and merge failure. `expanded` images show the long-output control open.

## Checks

- `make build TAGS="http ui"`: passed; embedded assets regenerated and Chromium 104 compatibility checked.
- UI typecheck: passed. After synchronization with `main`, the full Vitest suite passed: 204 files / 1318 tests, including the ACP approval restoration regression.
- `TestSVNToolPresentationFeature`: passed in Chrome, including actual More/Less interactions and screenshots. The original implementation failed the new feature and component tests before implementation.
- `TestUILayoutColumnsAlign`: passed at 390px and 1280px.
- The built HTTP server rendered `/` with a composer and send button using an isolated temporary profile; the server was stopped after the smoke check.
- `make lint`: all configured passes completed with zero issues.
- `make test`: default and `memory` passes completed successfully. The long full matrix was stopped during `cli`; the complete matrix has **not** been verified.

PR preparation on 2026-09-11 synchronized the change with `main` at `b329ecf1`. UI typecheck, the full Vitest suite, the embedded build and both browser checks were repeated. The first Go build attempt ran out of space on C:; the successful retry used a dedicated temporary build/cache directory on D:.

Run the feature and regenerate screenshots from the repository root with `FOXXYCODE_SVN_SCREENSHOTS` pointing to the desired output directory:

```sh
go test -tags=http,ui,browser ./external/ui -run '^TestSVNToolPresentationFeature$' -count=1 -timeout=120s
```

`FOXXYCODE_UI_BROWSER` can specify the Chrome executable. The test owns and stops its Vite and browser processes. The fixture is available at `/svn-tools-check.html` on the Vite development server.
