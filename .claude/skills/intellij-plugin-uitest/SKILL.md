---
name: intellij-plugin-uitest
description: Drive and visually verify the FoxxyCode plugin's UI in a real sandbox IDE with Remote Robot - launch the sandbox, dump the Swing component tree, click/type/screenshot via uiConsole scripts, and run the permanent uiTest suite. Use when asked to check how the plugin looks or behaves in the IDE, take plugin screenshots, or write/debug UI tests. For building, unit tests and runIde use intellij-plugin-gradle instead.
---

# UI testing the IntelliJ plugin (Remote Robot)

Everything below happens in `editors/intellij`. Environment rules from
`intellij-plugin-gradle` (JAVA_HOME, `-g H:/gradle-home`, proxy flags for online runs) apply to
every command here; that skill also owns building and the unit-test layer.

## What automation can and cannot reach

The tool window is a **toolbar plus a JCEF browser**. Remote Robot walks the *Swing* hierarchy,
and the browser's content is not Swing — so `tree`, `click <xpath>` and `assert-text` reach the
toolbar, the tool window chrome, IDE dialogs and popups, and **nothing inside the chat**. The
composer, the messages, the mention popup and every SPA control are invisible to XPath.

That is not a gap to work around; it decides how you verify:

- **Chrome, lifecycle, toolbar composition, dialogs** → assertions (`uiTest`).
- **Anything the SPA renders** → screenshots you read, plus raw keyboard/mouse if you need to
  drive it. Click into the panel by coordinates from a screenshot, then use `type` / `key`.
- If you truly need to assert on chat content, the escape hatch is JS on the IDE side:
  `FoxxyCodeBrowserPanel.forProject(project)` exposes the panel, and its `cefBrowser` can
  `executeJavaScript`. There is no return channel today, so that is a build-it-first task, not
  something to reach for casually.

## Pick the cheapest sufficient layer first

| Layer | Command | Catches | Cost |
|---|---|---|---|
| 0. Logic | `gradlew test` (plus `go test ./editors/intellij/`) | reflection helpers, path math, plugin.xml / bundle drift | ~20 s |
| 1. Interactive | sandbox + `gradlew uiConsole -PuiScript=…` | what the UI actually looks like and does | ~30 s per script |
| 2. Regression | sandbox + `gradlew uiTest` | a codified scenario broke | ~1 min |

Do not launch an IDE for something layer 0 already catches. Layers 1–2 exist for what layer 0
cannot see: real rendering, focus, themes, toolbar composition, popup behaviour.

## One-time: start the sandbox (keep it running)

```bash
cd editors/intellij && JAVA_HOME="/c/Program Files/JetBrains/PyCharm Community Edition 2023.3.2/jbr" ./gradlew -g H:/gradle-home --no-daemon --offline runIdeForUiTests
```

**Run it in the background** — the task blocks until the IDE closes. Ready when
`curl -s --noproxy '*' http://127.0.0.1:8580/` returns 200 (30–60 s; first ever run also
builds the Go binary + SPA and downloads the robot-server plugin, so drop `--offline` and add
the proxy flags then). Check the port *before* starting: 200 means a sandbox is already up —
reuse it. Kill it by PID from `netstat -ano | grep 8580`.

The task pre-configures everything automation needs: opens a copy of `uitest-project/`
(a project must be open or the tool window cannot exist), seeds `firstRunCompleted` so the
FirstRunDialog modal never appears, points the backend at a throwaway
`build/uitest-foxxycode-home` (isolating tests from `~/.foxxycode` — its `ui.locale`, sessions,
history), bundles the freshly built `foxxycode` binary into the sandbox, and pins English.
**The IDE needs a real, unlocked display** — java.awt.Robot clicks land on whatever is visible;
a locked screen or headless RDP session breaks interaction.

Give the panel time on the first open: the plugin spawns `foxxycode http` and polls it before
the SPA paints. `sleep 8` after `toolwindow FoxxyCode` is a realistic floor.

## The loop: script → run → look

1. Write a script (see `uitest-scripts/*.uiscript` for working examples):

```
toolwindow FoxxyCode
sleep 8
screenshot opened
tree FoxxyCode
```

2. Run it: `… ./gradlew -g H:/gradle-home --no-daemon --offline uiConsole -PuiScript=uitest-scripts/my.uiscript`
3. **Read the PNGs** in `build/uiconsole/` (numbered per step, plus `console.log` and
   `NN-tree.txt`). Reading the screenshot is the point — that is you looking at the UI.

### Commands

| Command | Does |
|---|---|
| `toolwindow <id>` / `toolwindow-hide <id>` | open/close a tool window by plugin.xml id |
| `action <ActionId>` | fire an action by id — prefer over menu-walking |
| `click <xpath>` (also `doubleclick`, `rightclick`) | click a component |
| `type <text>` / `key <ENTER\|CTRL+A\|…>` | keyboard into the focused component (KeyEvent.VK_ names) |
| `wait <xpath> [sec]` | poll until the component exists |
| `sleep <sec>` | fixed pause (UI animations, backend startup) |
| `tree [filter] [depth]` | dump Swing tree → `NN-tree.txt` (echoes the filtered part) |
| `text` / `assert-text <s>` / `assert-no-text <s>` | visible-text probe over the Swing tree |
| `screenshot [name]` | PNG cropped to the IDE windows |
| `width <px> [toolWindowId]` | resize tool window (the 320 px wrap check) |
| `theme <light\|dark>` | switch IDE LaF |
| `js <rhino>` | escape hatch: ES5 on the IDE side, runs on the EDT |

Every interacting command auto-screenshots; a failure screenshots too, then stops the script.

### Finding locators

`tree FoxxyCode` (or any filter substring, matched against class and accessible name) is the
fastest path: each line shows `Class name="…" text="…"`, which maps 1:1 onto XPath —
`//div[@class='FoxxyCodeBrowserPanel']`, `//div[@myicon='refresh.svg']`. While the sandbox runs,
`http://127.0.0.1:8580/` serves the same tree as HTML with a built-in XPath tester.

**Prefer `@myicon` for toolbar buttons and `@class` for panels; avoid `@accessiblename`** —
names come from FoxxyCodeBundle and change with locale. Toolbar icons in
`FoxxyCodeBrowserPanel.createToolbar()`, in order: `restart.svg`, `refresh.svg`, `web.svg`,
`console.svg`, `settings.svg`. The JCEF surface shows up as `JBCefOsrComponent` or a
`JBCefBrowser` wrapper depending on the IDE runtime — run `discover.uiscript` and read the tree
rather than assuming which one this machine uses.

## The permanent suite

```bash
cd editors/intellij && JAVA_HOME="/c/Program Files/JetBrains/PyCharm Community Edition 2023.3.2/jbr" ./gradlew -g H:/gradle-home --no-daemon --offline uiTest
```

Needs the sandbox already running (same as uiConsole). Sources in `src/uiTest/`:
`BrowserPanelUiTest` (tool window opens without a modal, the toolbar carries every action, the
backend reaches the SPA instead of the start-error card, reopening keeps a working panel),
`fixtures/FoxxyCodeToolWindowFixture` (the PageObject — extend it rather than sprinkling raw
XPath), `IdeControl` (tool window / action / theme scripts shared with uiConsole). JUnit 4,
same as the rest of the project. Failures: `grep -A6 "<failure" build/test-results/uiTest/TEST-*.xml`.

**Codify a scenario as a test when it guards a behaviour that must not regress** and when it is
reachable from Swing. One-off "does this look right after my change" stays a throwaway uiConsole
script — do not accumulate brittle tests for visual judgement calls, and do not try to assert on
chat content from here.

## The visual checklist

Source of truth: the list in `intellij-plugin-gradle` → Visual verification. Ready-made scripts:
`checklist-1-narrow` (item 1), `checklist-2-themes` (item 2). Not scriptable: HiDPI (needs an IDE
restart with a different scale factor) and a live agent turn (needs a real API key; the isolated
test home deliberately has none). Report those as not covered instead of faking them.

## Known traps

- **Modal dialogs freeze everything.** If a script hangs on `find`, screenshot first — a modal
  (FirstRunDialog, error dialog) is probably parked over the UI. The seeding normally prevents
  FirstRunDialog; if it shows anyway, delete `build/idea-sandbox/config-uiTest/options/foxxycode.xml`
  and restart the sandbox so it reseeds.
- **`-PuiScript`, not `-Pscript`** — Gradle resolves `findProperty("script")` against the
  Project bean and returns `false`.
- **Screenshots capture the whole desktop** before cropping; the crop uses the IDE's own window
  bounds. If a screenshot looks like the browser, the IDE window was minimised — bring it up.
- **EDT**: any `js` touching Swing/ToolWindow/DataContext must run on the EDT — the console's
  `js` command already does; in test code use `robot.runJs(script, true)` (the `true` is the
  EDT flag) or go through `IdeControl`.
- **State leaks between runs**: the sandbox persists the IDE theme and the tool window width
  across scripts and test runs. Scripts must establish what they need and put the default back
  (dark theme) when done.
- **Gson vs JDK 17**: the `--add-opens java.base/java.lang=ALL-UNNAMED` on uiConsole/uiTest is
  load-bearing; without it every robot call dies with "Unable to create converter for
  RetrieveResponse".
- **After changing plugin sources**, restart the sandbox — it runs the plugin as packaged at
  launch. Kill by PID, rerun `runIdeForUiTests` (it rebuilds via prepareUiTestingSandbox).
- **Kill the backend too.** Killing the sandbox IDE by PID skips `appWillBeClosed`, so its
  `foxxycode.exe` child keeps running. It no longer breaks the next `runIdeForUiTests` — the
  plugin runs a *staged* copy under `idea-sandbox/system/foxxycode-bin/`, not the one
  `prepareUiTestingSandbox` writes — but it still holds a port and eats memory. Clean up with
  `Get-Process foxxycode | Where-Object { $_.Path -like "*idea-sandbox*" } | Stop-Process -Force`.
  Exiting through `action Exit` instead reaps the backend by itself.
- The `clean`-then-build file-lock flake and the mojibake worker crash from
  `intellij-plugin-gradle` apply to these tasks too.

## CI

Not wired up: these tasks need a display and an interactive session. Layer 0 already runs in
`.github/workflows/intellij-plugin.yaml`; a windows runner job for `uiTest` is a possible later
step, not part of this setup.
