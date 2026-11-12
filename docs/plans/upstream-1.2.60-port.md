# Upstream 1.2.57–1.2.60 port

## Scope

Port the complete net range
`1e1cd320a1a0408d4a52155c8d76e31d4550e83e..30fccb74140d42cc9b6a9e234d5eb831404fd3c5`
from coddy-project/coddy-agent through tag `1.2.60`, onto fork baseline
`56f8f643045a3a7f68b945c1941bc687aa526661` (the merged 1.2.56 port). The range
contains 58 commits: 52 adapted changes and six redundant integration merges.
No upstream change is intentionally deferred.

- Add the Telegram Mini App: signed admin launch, the menu button and `/app`,
  Telegram navigation, theme synchronization, half-open viewport and keyboard
  handling, offline tgfake client and end-to-end browser scenarios.
- Add the Pachca integration bot and its offline fake: direct chats, groups and
  threads, quoted replies, streaming edits, shared access policy and polling.
- Restrict non-admin messenger turns to reading within the workspace. Settings,
  model/session changes and privileged tools stay with bot admins, and approval
  grants of another member or a shared session are not inherited.
- Replace inferred reasoning-off support with the explicit per-model
  `allow_reasoning_off` setting, carried through YAML, JSON DTO, schema,
  Settings, `/v1/models` and session validation.
- Adopt vendor-neutral contributor rules, scalar and YAML-list `globs`, and
  transactional adapter delivery. The no-op `.codex/rules.md` index is removed.
- Include Telegram reply quoting and group addressing, admin-only group
  settings, dry-run probes and the bundled rpa-gen-rules 1.3.4 updates.

## Fork adaptations and owner decisions

Preserve all registered fork divergences, retry limit 10 and useful-step
accounting, continuation budgets, queue behavior after output caps, stop notices,
cross-process state, both compaction engines, Summary, provider templates, IDE
context, SVN, exports and localization. The new turn restriction is added beside
these paths rather than replacing them.

Keep the fork's `sessionstore.Get` / `Reset` error returns and four-value logger
API in Pachca and the new tests. `PathInWorkspace` uses the existing
`session.within` helper because the upstream helper lived in the export file that
FoxxyCode had already moved to `internal/export`. Dry-run reads web sign-in
environment names from `internal/webauth`, avoiding an import cycle.

Keep the Kimi Code adapter alongside Codex, OpenCode and ZCode. It receives the
same YAML parsing, repository-scoped state, locking and transactional delivery,
and is covered by `make test-agent-rules`. On Windows, a rule whose path cannot be
made repo-relative keeps its full unique path instead of failing after emission;
this preserves interprocess deduplication. Existing GitHub Pages documentation,
schema publication and the editor changelog contracts remain in place.

Preserve the Chromium 104 JCEF baseline: the Telegram dock uses the existing
`chat-bottom--docked` marker instead of `:has()`, and dynamic viewport units have
plain `vh` fallbacks. The Mini App end-to-end fixture names its test user as an
admin because `/app` and signed Mini App launch are admin-only.

Do not copy Coddy-branded screenshots. Documentation uses three new captures of
the real FoxxyCode binary and tgfake stand. Russian schema overlay entries cover
Pachca, Mini App and reasoning-off; the shared dictionaries retain the existing
fork-specific structure rather than merging upstream schema strings into it.

## Validation and evidence

- `make test-agent-rules`: 15 cross-adapter tests pass, including concurrent
  delivery, lifecycle state and Kimi Code; seven legacy Kimi tests also pass.
- Focused Go tests pass for agent, session, config, dry-run, rules and all
  gateway packages, including Pachca and Telegram. The full shipped-tag tree
  compiles. `internal/forkguard` and every behavioral guard of the touched
  divergences pass, including the stream-idle guards in `internal/llm`.
- UI type checking, localization parity, Telegram, Settings, layout and schema
  overlay tests pass. The embedded `http,ui` build passes Chromium 104
  compatibility checking. `make docs`, `make docs-check` and
  `make site-schema-check` pass.
- `npm run check:telegram` passes bot menu and `/app`, Mini App launch, half-open
  geometry, sheets and Back, light theme and cross-site sign-in explanation.
  The three documentation captures are real images of this stand; they were also
  inspected in the built-in browser.
- `python scripts/upstream.py rebrand-check` reports no structural branding
  errors. `ports.yaml` passes `ledger-check`. IntelliJ changelog tests and all
  18 VS Code changelog tests pass. `make lint`, `make check-windows` and
  `make lint-windows` pass.

The final express `make test`, lint and ledger recording are completed in the
closing commit; CI's tag matrix and race detector remain PR checks.
