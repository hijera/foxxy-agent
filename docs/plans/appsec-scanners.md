# Plan: open-source AppSec scanners for FoxxyCode (issue #374)

*Revised after cross-review by four independent reviewers
(qwen3.8-27b-noreason, devin/swe-2, gpt-5.6-luna, cursor/auto). All four
returned "revise" on the same points; the revision below is the consensus.*

## Goal

Give the project a repeatable, gate-able AppSec check: **the same command runs
locally and in CI**, so a finding that breaks the pipeline can be reproduced
and fixed on a developer machine without pushing. Scanners the issue names:
**trivy** (dependency vulnerabilities, secrets; misconfig report-only) and
**semgrep** (SAST). Starting point per the issue: `trivy: HIGH 8, CRITICAL 1`,
`semgrep: HIGH 476` — the gate cannot fail on HIGH from day one without
blocking every PR.

## Core design decision (from review)

**One implementation.** `scripts/security-scan.sh` is the *only* place that
invokes the scanners — versions, flags, skip-lists, gate thresholds. CI calls
it through `make security`; it is *not* re-implemented with
`aquasecurity/trivy-action` (the action's trivy version is tied to the action
release, and every flag written twice drifts — the failure mode the issue asks
to remove). `ubuntu-latest` runners ship docker, so the same docker fallback
path works in CI. After the script runs, CI only uploads the `*.sarif` files
it produced — same convention as `tests-on-pr.yaml` reading `TEST_TAG_SETS`
from the Makefile "so this file cannot drift".

**Report and gate are separate passes over one scan.** `--severity` in trivy
both filters the report *and* feeds `--exit-code`; wiring `SEC_FAIL_TRIVY` to
it would silently drop HIGH findings the day the gate ratchets down. So:
scan the full severity set with exit-code 0, write complete JSON + SARIF,
then a `gate()` step counts severities from the JSON and decides the exit
code. Scanner operational errors (missing tool, dead docker, bad env value,
crashed scan) always fail, even in `sec-report` mode — report-only means
"findings don't fail", never "errors don't fail".

## Files

### `scripts/security-scan.sh` (new, bash, `set -uo pipefail` — no `-e`, `status=` accumulation like `checks.sh`)

- Resolves the repo root from git or its own path; cd's there.
- Runner resolution per scanner: local binary on PATH → docker image pinned
  `tag@sha256:` (`TRIVY_IMAGE`, `SEMGREP_IMAGE` constants at the top, bump
  procedure documented in the guide). `SEC_DOCKER=0` forbids the fallback
  (pure-local runs). Docker runs use `--rm`, `--pull` semantics that match CI
  (`docker pull` once, so a stale local image cannot diverge), `--user
  $(id -u):$(id -g)`, and a writable per-scanner cache dir under
  `dist/security/` so reports don't come out root-owned.
- **trivy**: `trivy fs --scanners vuln,secret,misconfig --format json` →
  `dist/security/trivy.json` (+ `--format sarif --scanners vuln,misconfig`
  second pass → `dist/security/trivy.sarif`; secrets are excluded from the
  SARIF because code scanning persists the matched text on GitHub). Repo-root
  `trivy.yaml` carries `scan.skip-dirs` (`node_modules`, `dist`, `build`,
  `docs/assets`, `.venv`, `.git`) — trivy does *not* respect `.gitignore`, so
  a dev machine after `npm install` would otherwise scan ~200k files CI never
  sees. `misconfig` is scanned for visibility but is *outside* the gate
  (report-only: its findings are policy questions, e.g. `AVD-DS-0002` on the
  `FROM scratch` Dockerfile).
- **semgrep**: `semgrep scan --config <SEMGREP_CONFIGS> --metrics=off --json`
  → `dist/security/semgrep.json` and `--sarif` → `dist/security/semgrep.sarif`.
  Default `SEMGREP_CONFIGS="p/golang p/typescript"` (registry packs are
  floating aliases, not pins — documented; `SEMGREP_APP_TOKEN` optional env,
  passed through when set, documented for rate limits / Pro rules; fork PRs
  never see it and the job is report-only anyway). Semgrep already respects
  `.gitignore`; `.semgrepignore` stays minimal (`.venv` — the one path
  `.gitignore` doesn't cover but a local tree can hold).
- **Gate** (python3 json stdlib — already a repo/CI dependency; jq fallback
  not needed): counts from `trivy.json` (`Vulnerabilities` + `Secrets` by
  severity) and `semgrep.json` (`results[].extra.severity` —
  `ERROR/WARNING/INFO`, never trivy vocabulary). `SEC_FAIL_TRIVY` default
  `CRITICAL` (fail on count>0 at or above it, vuln+secret only);
  `SEC_FAIL_SEMGREP` default `off` (report-only until the backlog shrinks).
  Both validated against their vocabularies; an unknown value is an
  operational error, not a pass.
- Prints a compact severity-count summary per scanner on stdout and writes
  `dist/security/summary.md` for the CI job summary.
- `SEC_SCANNERS="trivy,semgrep"` default; `sec-trivy`/`sec-semgrep` narrow it.

### `Makefile` (+ `.PHONY`)

- `make security` — both scanners, gate on.
- `make sec-trivy` / `make sec-semgrep` — one scanner via `SEC_SCANNERS`.
- `make sec-report` — both, `SEC_FAIL_*=off` forced (never fails on findings;
  still fails on operational errors).

### `.github/workflows/security.yaml` (new)

- Triggers: `pull_request`, `push: branches: [main]` (keeps a fresh SARIF
  baseline on the default branch so code scanning shows PR *deltas* instead
  of "everything is new"; also covers direct pushes), `schedule` weekly cron
  (a clean dependency gains a CVE without any commit), `workflow_dispatch`.
- One job `security-scan`: `permissions: contents: read`,
  `security-events: write`, `actions: read` (job-scoped, not workflow-wide).
  Never `pull_request_target` — untrusted checkout must not run with a
  write token.
- Steps: checkout → `make security` (script's docker fallback runs on the
  runner) → `upload-sarif` for `trivy.sarif` (`category: trivy`) and
  `semgrep.sarif` (`category: semgrep`), each `if: always() &&
  !(github.event_name == 'pull_request' &&
  github.event.pull_request.head.repo.full_name != github.repository)` —
  fork PRs are scanned and gated but never write to this repo's code
  scanning; a *trusted* upload failure fails visibly (no blanket
  `continue-on-error`, which would also hide a broken gate). Reports also go
  to `actions/upload-artifact` so fork-PR findings are still downloadable.
- Job summary: append `dist/security/summary.md` to `$GITHUB_STEP_SUMMARY`
  with `if: always()` so counts are visible even when the gate fails.
- Note in the guide + PR body: the workflow's job name should be added to
  branch-protection required checks (repo file alone can't do that).

### Suppressions

- `.trivyignore.yaml` (not `.trivyignore`): structured entries with
  `expired_at` and `statement` — used for the one CRITICAL the issue reports
  *only* if it can't be fixed in this change, and for `AVD-DS-0002`-class
  misconfig that has no fix in a scratch image. Path exclusions live in
  `trivy.yaml` `skip-dirs`, not here; secret findings are not suppressible
  via trivyignore at all — real false positives get `--skip-files` or stay
  visible.
- `.semgrepignore`: `.venv` only (semgrep honors `.gitignore` for the rest).
  Inline suppression is `// nosemgrep` (Go/TS) or `# nosemgrep`
  (YAML/shell), always with a justification comment.

### Docs (documentation contract)

- `docs/contributing/security-scanning.md` (new) — *not* `security.md`:
  `docs/operate/security.md` already exists ("Security and trust"); the new
  page cross-links it. Covers: local runs (binary or docker), reports,
  `.trivyignore.yaml` / `nosemgrep` workflow with justification, the gate and
  how to ratchet it (trivy HIGH next, then semgrep ERROR), the docker digest
  bump procedure, the `SEMGREP_APP_TOKEN` note, and the parity caveat (the
  vuln DB itself isn't pinned — local matches CI only on the same DB state).
- `docs/nav.yaml` entry; `docs/contributing/build.md` target list;
  `docs/reference/environment-variables.md` rows for `SEC_*` and
  `SEMGREP_APP_TOKEN`; `AGENTS.md` + `CLAUDE.md` one line each.
- `make docs` regenerates the hub; `make site-docs-check` verifies the site
  layer (nav changed).

### Not in this change

- Gate ratcheting beyond `CRITICAL`, vendored/licensed semgrep rules,
  `.semgrepignore` tuning for fixtures, fixes for the HIGH backlog (separate
  PRs, each with its own triage).
- Pre-commit hook: a full trivy+semgrep run is too slow for a commit gate;
  `make security` is the documented local path (same opt-in pattern as
  `FOXXYCODE_HOOK_TESTS`).
- `dist/` is already gitignored — no `.gitignore` change needed.

## Verification

- `make sec-trivy`, `make sec-semgrep`, `make sec-report`, `make security`
  locally via the docker fallback (no local binaries on this machine):
  reports land in `dist/security/`, counts print, `sec-report` exits 0 with
  findings, `SEC_FAIL_TRIVY=LOW` flips the gate non-zero, a bogus
  `SEC_FAIL_TRIVY=banana` fails as an operational error.
- Triage the day-one findings: fix or `.trivyignore.yaml` the 1 CRITICAL;
  list the HIGH count in the PR.
- `shellcheck` the script if available; `actionlint` the workflow if
  available; `make docs-check` (or `docsgen -skip-cli` + the docs CI job) for
  the nav/links contract.
- CI proves itself when the PR opens (`pull_request` trigger).
