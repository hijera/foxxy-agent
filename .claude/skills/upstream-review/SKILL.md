---
name: upstream-review
description: Review a coddy-agent port into FoxxyCode against the original upstream changes, the fork divergence registry and the port ledger. Use to audit a prepared or implemented upstream wave for omissions, regressions and incorrect port status.
---

# Review an upstream port

Review the requested wave rather than importing additional upstream work. Read
`AGENTS.md`, `.cursor/rules/upstream-divergences.mdc`, `ports.yaml` and the
applicable scoped Cursor rules. Use `UPSTREAM_SYNC.md` when available for
historical context; it can be local and ignored by Git.

## Establish the comparison

Identify full upstream base/target SHAs and the fork base/result. A prepared
worktree has its bundle under its private Git directory in `upstream-port/`:
`manifest.json`, `scan.json`, `scan.md`, `original.patch` and `adapted.patch`.
Use `git rev-parse --absolute-git-dir` in that worktree to locate it. The manifest
pins the fork base and upstream endpoints. If there is no bundle, obtain the
range from the request or wave record rather than guessing from release names.

Run these using the helper from a checkout that contains it:

```bash
python scripts/upstream.py scan --from <upstream-base> --to <upstream-target>
python scripts/upstream.py check --base <fork-base>
python scripts/upstream.py ledger-check
```

Pass `--repo <fork-worktree>` before the subcommand when needed. These reports
are evidence for review, not a passed test suite. Inspect the original diff and
commit dependencies as well as the net range: changes later reverted, merge
commits, partial ports and earlier out-of-order ports need separate accounting.

## Compare behavior, not just matching hunks

- Account for each selected upstream change as implemented, deliberately
  adapted, already covered, deferred or skipped. Check that an omission has a
  reason and that partial/deferred work remains visible in the ledger backlog.
- Inspect each affected registry entry and its guard tests. A cleanly applied
  patch can still undo fork behavior; retained `fork(...)` comments alone do
  not prove preservation. Follow the registry's owner-decision requirements
  before proposing that a divergence be retired.
- Verify rebranding in code and paths: `coddy` -> `foxxycode`, `Coddy` ->
  `FoxxyCode`, `CODDY` -> `FOXXYCODE`. Check callers, imports, package/module
  owners, routes, environment names, fixtures and docs together. Keep explicit
  compatibility contracts and provenance. Mechanical patches can rename
  semantic values such as `engine: coddy` incorrectly; review those hunks.
- Check binary and non-UTF-8 files against the original patch; preparation keeps
  their payloads unchanged. Adapted text patches lack original blob IDs and are
  not suitable for `git apply --3way`. Never infer correctness from apply success.
- Follow the workflow obligations for changed behavior: tests, HTTP/OpenAPI,
  schema publication, CLI packaging, text encoding, trust gates, build tags,
  localization, rebuilt UI assets, screenshots and documentation as applicable.

Run the relevant behavioral guards with their real packages and build tags,
and verify that tests actually execute. Distinguish failures introduced by the
port from reproduced baseline failures and checks that could not run. Do not
claim the whole suite is green from registry validation or a targeted test.

## Report findings

Lead with actionable findings: severity, current file/line, concrete regression
or omission, and supporting upstream/fork evidence. Separate questions requiring
an owner decision from confirmed defects. If no actionable findings remain,
say so and name material validation limits.

Review ledger evidence: source SHA, fork commits, reason and remaining work.
`record` verifies commit existence and fork ancestry, not semantic equivalence;
do not accept its success as proof of a completed port. Review is read-only
unless the operator also requested fixes or journal updates. Do not close
partial work or rewrite divergence decisions merely to make the report clean.
