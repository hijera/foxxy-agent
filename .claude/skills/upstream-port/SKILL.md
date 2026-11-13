---
name: upstream-port
description: Plan and port coddy-agent changes into the FoxxyCode fork in reviewable waves, preserving intentional divergences and tracking partial ports. Use for upstream release syncs and selected upstream commit ports.
---

# Port coddy-agent into FoxxyCode

Run commands from the repository root. This skill supports the direction
`coddy-project/coddy-agent` -> `hijera/foxxy-agent`; resolve a different requested
direction before applying patches.

## Establish the range

Read `AGENTS.md`, `ports.yaml`, `UPSTREAM_SYNC.md` when present, and
`.cursor/rules/upstream-divergences.mdc`. The journal can be local and ignored by
Git. If it is absent, use the operator's explicit range and known port history;
do not invent a synchronization checkpoint.

Inspect `git status --short` and `git remote -v`. For an authorized sync, fetch
the configured upstream explicitly. If adding a missing `upstream` remote, use
`https://github.com/coddy-project/coddy-agent.git`; never replace an existing
remote with a different URL silently. Resolve the requested release/ref to a
full SHA so the wave does not change while work is in progress.

```bash
python scripts/upstream.py scan --from <upstream-base-sha> --to <target-sha>
python scripts/upstream.py wave --from <upstream-base-sha> --to <target-sha>
python scripts/upstream.py overlap --from <upstream-base-sha> --to <target-sha> --fork-base HEAD
```

`scan` inventories candidates, including merge commits, and annotates them from
`ports.yaml`. Missing records are `unreviewed`, not proof that a change is
unported. Cross-check the journal and fork history, especially partial and out-of-order
ports such as `deferred-351`. A merge's first-parent diff can overlap the branch
commits listed with it. Choose coherent changes, not every patch in the report.

For a range of many releases, plan with `wave` before selecting anything: it
cuts the range into waves ending at release tag boundaries, with commit, file,
divergence and unreviewed counts per wave and PR merges grouped with their
commits. `overlap` then drops the files the fork already matches, so the review
list is only what actually differs.

The script maps product branding in paths and checks both upstream and fork
paths for markers and registry sites. Inspect other renames and mapping
collisions manually. An empty divergence list is not a safety verdict.

## Port a wave

Prepare a new worktree and review bundle when the wave is ready:

```bash
python scripts/upstream.py prepare --wave <name> --from <upstream-base> --to <target> --fork-base HEAD --worktree worktrees/<name> --dry-run
```

Remove `--dry-run` to create `codex/upstream-<name>`. The command preserves source
checkout edits and refuses existing branches/paths. Only the committed fork
base is included; commit required tooling separately or invoke it from the
original checkout with `--repo <worktree>`.

The printed bundle path contains the original and mechanically rebranded net
patches, scan reports and pinned SHAs. Patches are not applied. Inspect the
ledger before selecting hunks: the net patch includes already recorded ports.
Review binary/non-UTF-8 payloads separately; their contents are not rebranded.
After manual review, use `git apply --check <bundle>/adapted.patch` in the new
worktree before applying selected changes. Adapted text patches cannot use
`--3way` because their upstream blob IDs no longer describe the rebranded text.

Prefer `preview` and `apply` over hand-applying hunks. `preview` runs the
normalized three-way merge and reports conflicts as divergence candidates;
`apply` writes only its clean dispositions into the working tree, never staging
or committing, and skips paths with uncommitted edits. Structural and conflicted
rows stay manual. After the mechanical step, run `rebrand-check` and the
divergence guard tests before recording.

Follow `.cursor/rules/workflow.mdc` and the scoped rules for touched files.
Keep substantive project rules there; this skill does not supersede them.

- Group related changes by dependency. Establish the fork base SHA before edits
  and use an isolated branch/worktree when needed to preserve existing work.
- Compare upstream intent, tests and implementation with the fork. Preserve
  deliberate fork behavior even when a patch applies without a conflict.
  Follow each registry entry's next-change instruction, including owner
  decisions where required. Do not retire an entry merely to make tests pass.
- Rebrand imported product names in **code and paths**, not only visible copy:
  `coddy` -> `foxxycode`, `Coddy` -> `FoxxyCode`, `CODDY` -> `FOXXYCODE`.
  This includes identifiers, package/import paths, string literals, environment
  variable prefixes, CLI names, routes, filenames and directories. For example,
  port `cmd/coddy/main.go` into `cmd/foxxycode/main.go`, and update its callers,
  tests, fixtures and documentation consistently. Use the existing fork's
  module/repository addresses; token replacement alone does not change owners
  such as `coddy-project` into `hijera`.
  Keep upstream provenance URLs/SHAs and documented compatibility aliases or
  semantic values intact where the repository contract requires them (for
  example, an existing `engine: coddy` value). These are explicit exceptions,
  not a reason to leave upstream branding in new fork code. Record exceptions
  in the wave report. The script reports mappings; it does not rewrite files.
- Carry tests with the behavior; add a failing reproduction before implementing
  changed behavior as required by the repository workflow. Review dependencies
  on earlier upstream changes instead of copying whole divergent files.

## Verify and report

```bash
python scripts/upstream.py check --base <fork-base-sha>
python scripts/upstream.py rebrand-check --base <fork-base-sha>
python scripts/upstream.py verify --base <fork-base-sha> --run
python scripts/upstream.py ledger-check
go test ./internal/forkguard -count=1
```

`check` produces a review checklist, not a test verdict. It includes local
untracked paths and recovers registry sites/markers from the base revision.
Run the identified behavioral guards in their owning packages with appropriate
build tags; a successful Go invocation that runs zero matching tests is not
validation. Registry integrity alone does not prove behavior survived.
`verify` builds exactly those guard commands and, with `--run`, executes them
and reports `passed`/`failed`/`no_tests`/`unavailable` per command.
`rebrand-check` is the mechanical gate for leftover upstream branding; it
exits 1 on any finding outside the allowlisted compatibility values.

Complete the applicable workflow checks: HTTP/OpenAPI, configuration schema,
CLI packaging, UI rebuild/i18n/screenshots, documentation, Windows and build-tag
boundaries. Follow the existing schema publication rule for renamed/removed
keys. Finish with the required regression and lint gates and state unavailable
checks precisely.

Record each reviewed upstream commit in `ports.yaml` using a full upstream SHA,
status (`ported`, `adapted`, `partial`, `deferred` or `skipped`), `fork_commits`,
`reason` and `remaining`. Full fork commit SHAs are required for ported, adapted
and partial entries; keep uncommitted work in the wave report until those commits
exist. For example, from the checkout containing the helper:

```bash
python scripts/upstream.py --repo <fork-worktree> record --upstream-commit <source> --status adapted --fork-commit <result> --reason "Adapted to fork behavior" --dry-run
python scripts/upstream.py --repo <fork-worktree> record --batch reviewed.json --dry-run
```

`--batch` takes a JSON array of the same entries and records them in one atomic
write; a large range should be recorded that way rather than one invocation per
commit. The batch is all-or-nothing against the existing ledger.

Remove `--dry-run` to write the existing ledger. Repeat `--fork-commit` for
multiple commits, and `--remaining` for unfinished items. Updating a different
existing decision requires `--update`; supply the entire new decision. Recording
resolves refs and checks that fork commits belong to the worktree's HEAD history.
It does not commit, push or prove behavior. The write uses a lock and atomic
replacement; YAML formatting/comments are normalized, but notes and other
records are retained.

Partial/deferred entries require concrete remaining work. Completed and
skipped entries have an empty remaining list. Update an existing record rather
than adding a second entry for the same SHA. Validate with `ledger-check`.

The ledger begins without migrated historical entries. Resolve and verify the
journal's abbreviated SHAs before recording history. Schema validation alone
does not verify that commits exist or that behavior was ported. Inspect the
scan report's open backlog even outside the selected range; these entries are
not automatically filtered by ancestry. Never silently close a deferred item
when moving the range forward. Update the wave journal when appropriate and do
not mark a whole release synchronized while unresolved parts remain. Pushing,
publishing and merging follow the operator's requested scope.

For a requested port audit, use the repository's
[upstream-review](../upstream-review/SKILL.md) skill to compare original upstream
intent with the final fork behavior and ledger evidence.
