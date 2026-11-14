# Upstream 1.2.61–1.2.70 port

## Scope

Port the complete net range
`30fccb74140d42cc9b6a9e234d5eb831404fd3c5..706eeeecf6d9e4f17f1f509acc7f7a3e89d28d25`
from coddy-project/coddy-agent through tag `1.2.70`, onto fork baseline
`7173b71a434ee42b133f4b88a6efd2b2582b0048` (the head of PR #162, the 1.2.60
port with its review follow-ups). The range contains 38 commits: 36 adapted
changes and two redundant integration merges. No upstream change is
intentionally deferred.

- Render Mermaid and SVG fences as pictures and LaTeX as formulas, with
  theme-aware colours, source views and downloads, lazy renderer chunks and a
  turn-scoped web UI capability prompt.
- Add project scheduler jobs with fixed user/project folders, digest-bound
  trust receipts, containment checks, scoped REST/tools/UI and migration from
  `scheduler.dir`.
- Make memory note limits configurable and count characters rather than bytes,
  including correct truncation reporting for non-Latin text.
- Include pinned-row dragging, correctly timed UI-log notices, the Memory tab
  rename and grouping, relay file-transfer coverage and the contributor-rule
  updates. The queued-message alignment change and its revert cancel out.

## Fork adaptations and owner decisions

Preserve every registered divergence, including retry and continuation budgets,
queue behavior after output caps, transcript stop notices, cross-process state,
both compaction engines, Summary, provider templates, IDE context, SVN, exports
and localization. The generic UI-log timing change does not reintroduce a second
copy of FoxxyCode's streamed stop notice.

Keep legacy top-level `sched_` run rows only for user jobs; project jobs use the
new workspace-qualified session identity. Preserve legacy scheduler metadata,
`docs` and `debug` modes, and the fork's error-returning `session.NewSessionID`.
The new job session stores its canonical project workspace. Existing project
trust and approval behavior remains with the operator; no scheduler tool can
approve a job on the model's behalf.

Keep the Chromium 104 JCEF baseline and the fork's PostCSS/cache pipeline.
KaTeX uses woff2 fonts from content-hashed chunks instead of base64-inlined
font data, while brand and other content assets stay inline. Lazy Mermaid and
KaTeX chunks are copied, embedded and served immutable through the shared
`ui.Handler()`. The 137 generated chunk files are committed even though upstream
ignores them, because a checkout must contain the embedded renderers. The app
entry keeps the renderer-marker guard and uses the fork's 2.65 MB baseline
budget rather than upstream's smaller 1.85 MB one. Scheduler API helpers retain
the fork's offline-to-`ApiResult` handling, and the settings tests reset the
active locale after a Russian case.

Port the contributor rules by intent, preserving FoxxyCode's Android checks,
environment switching and automated layout test while adding the upstream
asynchronous-test, recoverable-send and SPA-tooling guidance. Update all
dictionaries by upstream-added or upstream-changed keys rather than moving
whole blocks: fork-specific translations and plural categories remain intact.
The Russian schema overlay covers the new memory and scheduler fields.

Do not copy Coddy-branded screenshots. Documentation uses eleven new captures
of the real FoxxyCode binary and its fixtures: five diagram states, four
scheduler states, Memory settings and a pinned-row drag. No PR-only evidence is
stored under `docs/assets/`.

## Validation and evidence

- Focused scheduler, config, session, agent and HTTP packages compile and pass.
  New project-job, memory, swarm and web UI prompt tests are included.
- SPA type checking and focused Vitest coverage for diagrams, formulas,
  scheduler project jobs, Settings, pinned rows, notices and localization pass.
- `npm run check:diagrams` passes all seven themes at 360px and 1280px,
  including decoded pictures, formulas and font loading, no horizontal overflow
  and on-demand renderer requests. All documentation captures were inspected.
- `make docs`, `make docs-check`, `make site-schema-check` and the Chromium 104
  compatibility check pass. `internal/forkguard` and every behavioral guard of
  the touched divergences pass. `make test-agent-rules` passes 18 cross-adapter,
  seven Kimi and eleven OpenCode tests.
- `GOFLAGS='-count=1 -p=2 -timeout=20m' make test` passes all 345 SPA files /
  3,452 tests and the complete shipped-tag Go tree. `make lint`,
  `make check-windows` and `make lint-windows` pass.
- IntelliJ changelog tests and all 166 VS Code tests pass, including the 18
  changelog contract tests. `python scripts/upstream.py ledger-check` validates
  the previous 1,393 entries; the 35 records of this wave are added next.

CI's tag matrix and race detector remain PR checks; this task deliberately
leaves a local branch and does not push or open a pull request.
