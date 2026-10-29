# AppSec scanning

*How the open-source scanners (trivy, semgrep) run locally and in CI, where the
reports land, how to read them and how the gate is tightened. For the agent's
own security model — what it may execute, permission modes, project trust —
see [Security and trust](../operate/security.md).*

Issue [#374](https://github.com/hijera/foxxy-agent/issues/374) asked
for open-source AppSec scanners on the repository. One script is the only
place they are invoked, so a finding that fails the pipeline reproduces on a
developer machine with one command:

```bash
make security      # both scanners, the CI gate behaviour
make sec-trivy     # trivy only
make sec-semgrep   # semgrep only
make sec-report    # both, gate off — findings never fail, errors still do
```

Each target is a thin `SEC_*` wrapper around `scripts/security-scan.sh`; call
the script directly when you need a combination the targets do not name.

## What runs

- **trivy** — filesystem scan of the checkout: dependency vulnerabilities
  (`go.mod`, `package-lock.json`, …), secrets, and IaC misconfigurations
  (the Dockerfile). Misconfig findings are collected but sit **outside the
  gate** — they are policy questions, not vulnerabilities.
- **semgrep** — SAST with the registry packs in `SEMGREP_CONFIGS`
  (default `p/golang p/typescript`). Report-only for now: the backlog is
  large enough that gating on day one would block every pull request.

## Runner and versions

A scanner binary on `PATH` is used when present; otherwise the pinned docker
image runs (`aquasec/trivy`, `semgrep/semgrep`, `tag@sha256:` constants at the
top of `scripts/security-scan.sh`). `SEC_DOCKER=0` forbids the fallback, for
machines where docker is off-limits.

To bump a scanner: pull the new image, take its digest
(`docker images --digests`), update the constant, run `make security` and
compare the summary with the previous one — a rule or DB update can change
findings without a commit, so bumps are their own small pull requests.

## Reports

Everything lands in `dist/security/` (gitignored):

- `trivy.json`, `semgrep.json` — full findings, all severities
- `trivy.sarif`, `semgrep.sarif` — for GitHub code scanning; the trivy SARIF
  excludes secrets because code scanning persists the matched text; the semgrep SARIF
  carries matched code and is uploaded from trusted events only
- `summary.md` — severity counts, what the CI job summary shows
- `.cache/` — scanner caches so repeat runs stay fast

## The gate

Report and gate are separate: the scan always writes the full report, then
the script counts severities from the JSON and decides the exit code.

| Knob | Values | Default | Meaning |
|---|---|---|---|
| `SEC_FAIL_TRIVY` | `UNKNOWN\|LOW\|MEDIUM\|HIGH\|CRITICAL\|off` | `CRITICAL` | Fail on vulnerability/secret findings at or above the level |
| `SEC_FAIL_SEMGREP` | `ERROR\|WARNING\|INFO\|off` | `off` | Same, in semgrep's own severity vocabulary |
| `SEC_SCANNERS` | `trivy`, `semgrep`, `trivy,semgrep` | both | Which scanners run |
| `SEMGREP_CONFIGS` | space-separated `--config` values | `p/golang p/typescript` | Registry packs (floating aliases — findings may shift on rule updates) |
| `SEMGREP_APP_TOKEN` | token | unset | Optional; lifts registry rate limits and enables Pro rules. Fork pull requests never see it — the job stays report-only there by design |
| `SEC_DOCKER` | `0\|1` | `1` | Allow the docker fallback |

An unknown knob value, a missing tool, dead docker or a crashed scan is an
operational error and fails the run — including under `sec-report`, where
only *findings* are forgiven.

The vuln database itself is not pinned: local and CI agree exactly only when
they resolve the same DB state, so re-run locally before blaming a CI
failure on the environment.

## Triage and suppression

- **`.trivyignore.yaml`** — accepted CVE/misconfig findings, each entry with
  a `statement` and an `expired_at` date so the list rots instead of
  pardoning new issues forever.
- **`trivy.yaml`** — `scan.skip-dirs` for paths a fresh checkout does not
  hold (`node_modules`, `dist`, …). Trivy does not honor `.gitignore`;
  without this a local scan walks trees CI never sees. Secret-rule suppressions
  are supported but use the same expiry and written justification as an
  accepted CVE; prefer a `skip-dirs`/`skip-files` rule or a fix when possible.
- **`.semgrepignore`** — kept minimal (`.venv`); semgrep honors `.gitignore`
  on its own. Inline suppression is `// nosemgrep` (Go/TS) or
  `# nosemgrep` (YAML/shell), always with a justification comment.
- Test fixtures carry deliberately fake credentials (`sk-test`, PEM blocks
  in `pictures_test.go`, stub keys in `examples/`). They are expected
  findings; accept them via the ignore files above rather than "fixing" the
  fixture.

## CI: `.github/workflows/security.yaml`

Pull requests, pushes to `main`, a weekly Monday cron, and manual dispatch.
The job calls `make security`, uploads both SARIF files to code scanning
(distinct `category` values), attaches `dist/security/` as an artifact, and
prints the severity summary into the job summary even when the gate fails.

Fork pull requests are scanned and gated normally, but their SARIF upload is
skipped — a fork token cannot write `security-events`, and untrusted code
must never push findings into this repository's code-scanning history. The
artifact still carries the findings for maintainers. The workflow's job
(`AppSec scanners`) should be marked required in branch protection for the
gate to block merges.

## Ratcheting

The gate is meant to tighten as the backlog burns down, each step its own
pull request with its own triage:

1. `SEC_FAIL_TRIVY=CRITICAL` ← now
2. `SEC_FAIL_TRIVY=HIGH`
3. `SEC_FAIL_SEMGREP=ERROR`, then `WARNING`
4. bring `misconfig` and wider semgrep packs (`p/bash`, `p/yaml`,
   `p/dockerfile`, `p/react`) under the gate once their findings are triaged
