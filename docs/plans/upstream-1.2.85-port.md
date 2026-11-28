# Upstream 1.2.84–1.2.85 port

## Scope

Port the complete net range
`22f03f281f34b30f8713701d4c2a620e505d78cb..f573a2bbb5ee15fa4f06361aa8232b500c2c289e`
from coddy-project/coddy-agent through tag `1.2.85`, onto current `main`
`b6665daf3caaf7c63f5069bd298458872b1045cf` (the completed 1.2.83 port).
The range contains 34 commits: 27 constituent changes and seven
integration/release merges. Constituent changes are adapted; merges are skipped
as redundant once their contents are represented by the net port.

- Add `/goal`, the persisted session goal, evidence-based supervisor checks,
  bounded continuations and recovery, the read-only workspace verifier and the
  stall/loop watchdog. Carry the console, remote console, web UI, ACP, HTTP,
  Telegram and Pachca surfaces, their events, tests and documentation.
- Add `/compact -m/--model` and `-r/--reasoning`, including per-call validation,
  fallback behavior and the web command option picker.
- Install skill sources and marketplace plugins without a system Git binary or
  a writable system temp directory, while keeping the manifest clone
  restrictions and the cross-process staging locks.
- Build the Files window once when a chat folder arrives late, fit the branch
  name to the available plate space, and include the console Resume race and
  code-scanning fixes.

## Fork adaptations

Preserve every registered divergence: the continuation and retry budgets,
complete tool calls after a cut stream, queue priority and output-cap behavior,
cross-process session state, stop notices, cumulative turn usage and the shared
stall guard. Keep both compaction engines, their hooks and the existing model
override chain; reasoning applies to the selected summarizer and only to
fallbacks that offer the requested level.

Keep the fork's localized UI, SVN workspace controls, editor integrations and
Chromium 104 baseline. All new dictionaries include English and Russian strings,
including plural categories and the schema overlay. The existing larger app.js
budget remains unchanged because it already exceeds upstream's raised budget.

The standalone `foxxycode http` manager also installs the workspace verifier,
matching `serve`, the console and ACP. The live standalone scenario exposed its
absence before this adaptation. The test harness generates tool-call IDs with an
atomic counter because Windows can give consecutive operations the same
`UnixNano` timestamp, which made the watchdog see redelivery instead of a cycle.
The marketplace fixture writes standards-compliant `file:///C:/...` URLs, and
the built-in file transport translates go-git's Windows drive host (`C` plus
`/...`) back to `C:/...`, leaving UNC and POSIX paths intact. The scenario passes
with both the system Git backend and the no-Git/no-system-temp backend.

The goal badge border reads `var(--goal-tone)` directly: the static color-mix
build cannot evaluate a runtime status class. The goal sheet keeps a preceding
`max-height: 80vh` before its `100dvh` calculation. Both rules are checked by the
embedded build's Chromium 104 gate.

Upstream-branded screenshots are not copied. The web captures come from a real
FoxxyCode standalone HTTP server with a scripted OpenAI-compatible provider. A
goal was submitted through the composer, checked by the supervisor and confirmed
by the verifier; the final API snapshot has `verified: true`. Dark, Light and
390px Phone menus, the Session goal settings and the compact reasoning picker
come from that stand. The console capture runs the real CLI scenario in WSL and
renders its resulting screen through local Chrome.

## Validation

- Focused session, agent, config, skills, remote, platform, ACP, HTTP and console
  suites pass. Full `internal/agent` and `internal/session` packages pass on
  native Windows.
- TypeScript checking, focused goal/i18n tests and the complete SPA regression
  pass: 387 files / 3,874 tests.
- The fork registry and behavioral guards pass, including cumulative usage,
  stream stalls, queue/output limits, continuation/retry behavior, cut streams,
  cross-process state and stop notices.
- `make build TAGS="http ui"`, the full-tag documentation build, embedded asset
  generation and Chromium 104 checks pass. `make docs`, `make docs-check`,
  `make site-schema`, `make site-schema-check` and the agent-rule adapter suites
  pass.
- The final sequential `make test`, `make check-windows` and `make lint-windows`
  gate passes on native Windows, including every full-tag Go package. `make lint`
  passes on the resulting tree.

Work is isolated in `build/upstream-1.2.85` on `codex/upstream-1-2-85`; nothing
is pushed by the local port.
