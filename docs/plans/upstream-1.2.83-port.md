# Upstream 1.2.80–1.2.83 port

## Scope

Port the complete net range
`cd42d7df06fb3c64c1b482e7c4463f3fc239ebb4..22f03f281f34b30f8713701d4c2a620e505d78cb`
from coddy-project/coddy-agent through tag `1.2.83`, onto current
`origin/main` `5bc8593ba` (the completed 1.2.79 port). The range contains 11
commits: seven constituent changes and four integration/release merges.

- Keep the composer mirror aligned with the masked textarea at fractional page
  zoom and screen densities, with component coverage and a real-engine
  `npm run check:caret` harness.
- Name every `instructions.files` document in the prompt, count it with rules,
  and report unreadable configured files in the log, console header and dry run.
- Fetch every configured Git remote before the branch menu opens, show
  remote-only branches alongside local ones, create tracking branches on pick,
  and preserve cached rows with a warning when the refresh fails.
- Move tgfake to `github.com/EvilFreelancer/tgfake v1.0.0`, imported through
  `pkg/server`, `pkg/botapi` and `pkg/llmstub`, with the same version exposed as
  a Go tool and installed by its release action in the dedicated CI stand.

## Fork adaptations

Preserve FoxxyCode's SVN workspace controls, managed-worktree recovery,
localized UI, session behavior, build tags and Chromium 104 baseline. The new
fetch route and its OpenAPI/schema documentation use the existing
`workspaceContextPayload`, including SVN and shell fields. `go-git` checkout
keeps the fork's stricter dirty-worktree protection and origin fallback while
remote tracking selection and `gitDirs`' four return values are reconciled.

All new user-visible strings have English and Russian dictionaries. The
FoxxyCode-specific detached-label translation is filled by the new localization
scenario. `turn_context` is stripped in tgfake's scripted models, and the model
name remains `foxxycode-demo`; the environment override is
`FOXXYCODE_TGFAKE_BIN`.

Upstream's whole-SPA Prettier sweep is not copied: this fork deliberately keeps
`styles.css` and `App.tsx` outside that sweep and formats only files touched by a
change. The existing staged-file Prettier gate remains. New and changed SPA
files in this port are formatted individually.

Upstream-branded screenshots are not copied. The three branch-menu captures in
`docs/assets/ui-branch-menu/` come from the real FoxxyCode `WorkspaceChips`
component on an isolated Vite stand at 1280×800 in the dark theme.

## Validation

- Focused Go suites pass for session, rules, agent, dry-run, gitws and tools;
  HTTP workspace/OpenAPI tests; the complete Telegram gateway package; and the
  console instruction/header scenarios.
- TypeScript checking and 174 focused SPA tests pass, including caret
  synchronization, branch refresh/remote rows/failure handling and Russian
  localization.
- `make build TAGS="http ui"` and the full-tag documentation build pass;
  generated embedded assets and Chromium 104 checks are included.
- `npm run check:caret` passes every real-Chromium case: desktop and phone,
  device scale factors 1–3, page zoom 110–175%, scrolling, runtime zoom and
  runtime density changes, with zero differing pixels.
- The branch-menu component was inspected in the built-in browser and both
  documentation captures were read back after generation.
- `make docs`, `make docs-check` and `make site-schema-check` pass, as do
  `make lint`, `make check-windows`, `make lint-windows`, fork/config guards
  and the agent-rule adapter suites.
- The full SPA regression passes 378 files / 3,798 tests. The complete
  shipped-tag `make test` regression passes the entire Go tree after the
  console-window guard was added to the new Git fetch process.
- The offline Telegram scenario reaches the background wake and the expected
  answer on both this branch and an unmodified `main` baseline, but its harness
  exits at the exact `failed, exit 2` assertion because the task row says
  `failed, exit 1`. The identical baseline result confirms a pre-existing
  Windows issue outside this port; the assertion was not weakened.

Work is isolated in
`H:/kimi/foxxy-agent-upstream-1.2.83` on `codex/upstream-1.2.83`; nothing is
pushed by the local port.
