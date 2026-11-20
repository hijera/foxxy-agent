# Upstream 1.2.71–1.2.72 port

## Scope

Port the complete net range
`706eeeecf6d9e4f17f1f509acc7f7a3e89d28d25..b9f75122ff13706b8716b162b88a2bfb36a6203b`
from coddy-project/coddy-agent through tag `1.2.72`, onto fork baseline
`3e5b721c5ea71b8c732fa7c9d87b7251185c5dc1` (current `origin/main`). The range
contains eight commits: seven adapted changes and one redundant integration
merge. No upstream change is intentionally deferred.

- Show an open message edit in the composer, outline the edited prompt and fade
  the tail that sending will remove. Escape or the banner's cross restores the
  draft and attachments the pencil replaced.
- Keep the last rewound tail, its UI log and tool-call detail in a bundle-local
  undo snapshot. Expose undo availability on transcript reads and add
  `POST /foxxycode/sessions/{id}/rewind/undo`, cancelling the edited turn before
  restoring history. File changes are not reverted.
- Make swarm session deduplication choose equal-length routes by the same
  hop-by-hop order as topology, with ring, lease, warning and lifecycle coverage.

## Fork adaptations and owner decisions

Preserve all registered fork behavior: queue delivery after output caps,
transcript stop notices, retry and continuation budgets, both compaction
engines, scheduler and child-session read-only rules, paged transcript reads,
localization and the Chromium 104 baseline. In particular, retain
`refreshFromDiskIfChanged` after acquiring the prompt turn lock for both rewind
and undo, so a process does not restore a stale in-memory copy over a bundle
another process changed.

Keep the fork's existing JSON/CSRF guard on rewind and its full OpenAPI request
contract. Add the undo route, `rewindUndo` response field, human HTTP guide,
session guide, DESIGN contract and both editor changelog twins together with
the implementation. Adapt the existing fork rewind test to the new **Send edit**
accessible name.

Do not copy Coddy-branded screenshots. The two documentation captures come from
the real FoxxyCode binary against the offline scripted model, in English, Dark,
1280×800. The same live stand verifies that Undo restores both original turns.
Generated embedded SPA assets are rebuilt and committed.

## Validation and evidence

- Focused session, HTTP and swarm Go tests pass, including the new BDD feature
  and all upstream undo, ring, lease and session tests.
- Focused SPA coverage passes 64 edit/banner/message tests and all 60 existing
  stop/queue tests after the accessible-name adaptation. Type checking passes.
- `npm run test:layout`, the complete phone overflow grid in English and
  Russian, and the full transcript-window check pass. The latter covers a
  3,306-message session, editing an older page, two browsers and a swarm relay.
- All queue-output, disk-refresh and stop-notice behavioral guards pass, as do
  `internal/forkguard`, rebranding, `git diff --check`, documentation checks and
  both editor changelog suites.
- The full embedded build, Chromium 104 compatibility check and `make lint` pass.
  `GOFLAGS='-count=1 -p=2 -timeout=20m' make test` passes all 351 SPA files /
  3,499 tests and the complete shipped-tag Go tree.

CI's tag matrix and race detector remain PR checks; this task deliberately
leaves a local branch and does not push or open a pull request.
