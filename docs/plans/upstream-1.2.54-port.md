# Upstream 1.2.46–1.2.54 port

## Scope

Port the complete net range
`bd28a329b9bbc53874556403bb2fd00f3f01c483..cc012f2b890a5a271b40dd15566a43b922669ce3`
from coddy-project/coddy-agent, through tag `1.2.54`, onto fork baseline
`570aa4a53bb6270f4e2d1061213751ce72f180ee`. The range contains 40 commits.
The normalized three-way preview covered 207 paths; conflicts were reconciled
against the standing divergence registry rather than taking either entire side.

- Keep permission and question waits alive after their original client disconnects.
- Validate contiguous tool-call/result pairing before provider requests, repair
  only missing results in the outbound projection, and close interrupted or
  permission-resumed batches without moving real late results.
- Enforce the configured provider transport for device sign-in, refresh, revoke,
  account usage and discovery; preserve the fork's explicit loopback/NO_PROXY rules.
- Record error generations and pending questions in History, retain collapsed
  groups, expose the global active count, and cache message counts against the
  transcript file identity with legacy/stale fallback before sorting or paging.
- Project valid Anthropic assistant turns, including the trailing empty case.
- Reload workspace-local catalogs on folder changes, warm configured MCP on
  explicit chat activation, scope MCP Settings to that chat, and recover sessions
  whose managed worktree was deleted to the main checkout.
- Share immutable session-owned files with inline cards, authenticated downloads,
  image previews and source actions. Keep 25 MiB per file, 100 MiB per session,
  and the prohibition on sharing from child sessions.
- Improve subagent arguments/transcript links, Tasks, copy actions, folder/branch
  filters, context popovers, paste handling and fenced-code shortcuts.
- Vendor Crossreview 2.0 from its own source repository, including project rosters,
  custom reviewer commands, documentation and reproducible branding adaptations.
- Carry the upstream contributor workflow and screenshot-evidence contract.

The model-check deployment document and its explicit revert have no net effect.
The ledger records three merges with their own resolutions as adapted; seven
redundant integration merges are represented by their constituent commits.
Together with the reverted document pair, this yields 31 adapted and nine skipped
records, with no remaining work in the range.

## Owner decisions

The owner requested all changes through 1.2.54 and approved upstream Crossreview
roster priority, arbitrary approved reviewer commands, activation-time MCP warming
with the fork's trust and asynchronous execution, deleted-worktree recovery and
all upstream file-sharing functionality within its limits. Preserve the centered
empty-chat composer and footer placement. Deliver a separate branch, checks and PR.

## Fork adaptations

Retain every registered divergence and guard: ten silent retries, useful-step
accounting, chunk-based stall detection, continuation budgets, queue limits,
cross-process refresh, stop notices, transport debugging, shared text encoding,
SVN, export formats, both compaction engines, Summary, localization, configurable
send shortcuts and Chromium 104 compatibility.

MCP generation cancellation and trust decisions stay asynchronous. Passive reads
stay cold; workspace changes reload catalogs immediately and defer connection
work until activation or a turn. ACP-supplied clients survive workspace changes.
Provider-proxy tests route a non-loopback test issuer through the proxy rather
than weakening the loopback bypass. Interrupted calls with incomplete JSON stay
out of history; complete calls receive explicit cancelled results. Artifact
marker removal leaves marker-free text byte-for-byte unchanged, preserving
partial-answer line boundaries. The fork's export projection also removes UI-only
file placement tags while retaining the stored chat and user-authored text.

Message-count metadata is integrated with stale-save guards, and activity/error
counters are reconciled when another process changes a session. The first-token
clock starts at provider dispatch, after local projection and validation. The
healthy-stream test emits its initial progress synchronously so scheduler delays
before its first timer tick cannot impersonate a silent model.
The provider-import test harness carries the same item-route state as the real
Settings drawer. A stacked folder dialog uses a zero top-inset fallback when
mounted outside its shell, keeping its controls inside a 1024 by 300 viewport.

## Evidence and validation

Screenshots in `docs/assets/upstream-1.2.54/` and the History state-dot image were
captured from real before/after binaries on isolated homes, a local scripted
provider and actual background subagent execution. Additional 1280/390 and
light/dark evidence is published on the `screenshots` branch. The browser check
verifies the downloaded report bytes, image preview and Tasks-to-child navigation.

- `GOFLAGS='-p=2 -timeout=25m' make test` passed: all shipped-tag Go packages,
  TypeScript, all 3,245 SPA tests and the OpenCode rule bridge.
- `GOFLAGS='-p=1 -timeout=15m' make test-race` passed every non-UI shipped module
  without a reported data race, including the complete agent and session suites.
- `make lint`, `make lint-windows`, `make check-windows`, `make test-cache`,
  documentation generation/checking and `make site-schema-check` passed.
- The Crossreview vendor check against the upstream snapshot, branding check and
  fork guards passed. The canonical provider-proxy rule was probed through the
  Codex rule parser on core, HTTP and CLI paths.
- Real Chromium layout, nine-width transcript overflow and chevron checks passed.
  Chromium and WebKit folder scrolling and creation controls stayed inside every
  prescribed viewport, including 1024 by 300.
- The 3,306-message transcript harness passed every scenario with Chromium CPU
  throttling at four and budget scale two, including editing, retrying, prompting,
  two simultaneous browsers and a swarm relay. First visibility measured 1.77 s
  on phone and 1.56 s on desktop; the render window stayed within 128 rows.
- Before/after, narrow and theme evidence is published in the `screenshots` branch
  under `upstream-1.2.54/round-1/` and `round-2/` (commits `03fe31d8` and
  `c8bf2c64`). Fenced-code input was exercised, not just photographed.

Local tooling was Go 1.26.10, Node 24.11.0 and golangci-lint 2.15.0. Native
Windows execution and desktop file-manager integration remain platform/CI
coverage; the local Linux stand is headless. The rebuilt SPA passes the
Chromium 104 compatibility check.
