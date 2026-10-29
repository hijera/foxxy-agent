#!/usr/bin/env bash
# security-scan.sh — the project's AppSec gate: trivy (dependencies, secrets,
# misconfig) and semgrep (SAST) over the checkout. This script is the *only*
# place scanners are invoked: CI calls it through `make security`, developers
# call it or the make targets directly, so a finding that fails the pipeline
# is reproducible on a dev machine without pushing.
#
# Runner per scanner: a binary on PATH first, then a docker image pinned by
# tag and digest (ubuntu-latest runners ship docker, so the same path works
# in CI). Bump the *_IMAGE constants together with the guide's procedure in
# docs/contributing/security-scanning.md.
#
# Knobs (env):
#   SEC_SCANNERS       comma list: trivy,semgrep   (default: trivy,semgrep)
#   SEC_FAIL_TRIVY     UNKNOWN|LOW|MEDIUM|HIGH|CRITICAL|off
#                      (default: CRITICAL; gate covers vulnerability and
#                      secret findings — misconfig is report-only)
#   SEC_FAIL_SEMGREP   ERROR|WARNING|INFO|off      (default: off — report only)
#   SEC_DOCKER         0|1  allow the docker fallback (default: 1)
#   SEMGREP_CONFIGS    space-separated semgrep --config values
#                      (default: "p/golang p/typescript")
#   SEMGREP_APP_TOKEN  optional; passed through to semgrep for registry rate
#                      limits / Pro rules (never required)
#
# Reports land in dist/security/: trivy.json, trivy.sarif, semgrep.json,
# semgrep.sarif, summary.md. The SARIF files exclude secret findings
# (code scanning persists matched text on GitHub).
#
# Exit code: 0 when every requested scan ran and the gate is clean; 1 on a
# gate failure or an operational error (missing tool, dead docker, bad env
# value, crashed scan — report-only mode does not swallow errors).
set -uo pipefail

log() { printf 'security-scan: %s\n' "$*" >&2; }

# git exports repo-location vars into hooks it runs; strip them so nothing
# inside the scanners sees a foreign repo.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_PREFIX GIT_COMMON_DIR \
      GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_INDEX_VERSION \
      GIT_NAMESPACE GIT_REFLOG_ACTION

# Pinned images: tag@sha256. Update procedure: docs/contributing/security-scanning.md.
TRIVY_IMAGE="aquasec/trivy:0.74.0@sha256:62b1e65e8869bc4b4c6aa4fa2b21595256c7c2f6018a9d9ad61caf87187c1969"
SEMGREP_IMAGE="semgrep/semgrep:1.177.0@sha256:acaac22ffc7b7cc5926de0751b223bce0b2491c33d18422fa72f632c78d81198"

SEC_SCANNERS="${SEC_SCANNERS:-trivy,semgrep}"
SEC_FAIL_TRIVY="${SEC_FAIL_TRIVY:-CRITICAL}"
SEC_FAIL_SEMGREP="${SEC_FAIL_SEMGREP:-off}"
SEC_DOCKER="${SEC_DOCKER:-1}"
SEMGREP_CONFIGS="${SEMGREP_CONFIGS:-p/golang p/typescript}"

if root=$(git rev-parse --show-toplevel 2>/dev/null); then
  :
else
  root=$(cd "$(dirname "$0")/.." && pwd)
fi
cd "$root" || { log "cannot cd to repo root '$root'"; exit 1; }

OUT="dist/security"
mkdir -p "$OUT/.cache/trivy" "$OUT/.cache/semgrep-home" || {
  log "cannot create $OUT"; exit 1; }

status=0

want_scanner() {
  case ",$SEC_SCANNERS," in
    *",$1,"*) return 0 ;;
    *) return 1 ;;
  esac
}

# runner <scanner> prints the command prefix to run it:
#   "trivy" | "docker run ... aquasec/trivy:..." — returned via stdout array line.
# Returns 1 when neither a binary nor docker is available.
resolve_trivy() {
  if command -v trivy >/dev/null 2>&1; then
    TRIVY_RUN=(trivy)
    return 0
  fi
  if [ "$SEC_DOCKER" = "1" ] && command -v docker >/dev/null 2>&1; then
    TRIVY_RUN=(docker run --rm
      --user "$(id -u):$(id -g)"
      -e "TRIVY_CACHE_DIR=/cache/trivy"
      -v "$root:/src" -w /src
      -v "$root/$OUT/.cache:/cache"
      "$TRIVY_IMAGE")
    return 0
  fi
  return 1
}

resolve_semgrep() {
  if command -v semgrep >/dev/null 2>&1; then
    SEMGREP_RUN=(semgrep)
    return 0
  fi
  if [ "$SEC_DOCKER" = "1" ] && command -v docker >/dev/null 2>&1; then
    SEMGREP_RUN=(docker run --rm
      --user "$(id -u):$(id -g)"
      -e "HOME=/cache/semgrep-home"
      -e "SEMGREP_APP_TOKEN"
      -v "$root:/src" -w /src
      -v "$root/$OUT/.cache:/cache"
      "$SEMGREP_IMAGE" semgrep)
    return 0
  fi
  return 1
}

# Pull pinned images up front so a stale local image cannot diverge from CI.
pull_images() {
  local img
  for img in "$@"; do
    if ! docker pull "$img" >/dev/null 2>&1; then
      log "docker pull failed for $img"
      return 1
    fi
  done
  return 0
}

# --- validate gate knobs up front: a typo must not look like a clean gate ---
case "$SEC_FAIL_TRIVY" in
  UNKNOWN|LOW|MEDIUM|HIGH|CRITICAL|off) : ;;
  *) log "unknown SEC_FAIL_TRIVY='$SEC_FAIL_TRIVY' (want UNKNOWN|LOW|MEDIUM|HIGH|CRITICAL|off)"; exit 2 ;;
esac
case "$SEC_FAIL_SEMGREP" in
  ERROR|WARNING|INFO|off) : ;;
  *) log "unknown SEC_FAIL_SEMGREP='$SEC_FAIL_SEMGREP' (want ERROR|WARNING|INFO|off)"; exit 2 ;;
esac
case "$SEC_DOCKER" in
  0|1) : ;;
  *) log "unknown SEC_DOCKER='$SEC_DOCKER' (want 0|1)"; exit 2 ;;
esac
known=0
old_ifs=$IFS; IFS=','
for s in $SEC_SCANNERS; do
  case "$s" in
    trivy|semgrep) known=1 ;;
    *) log "unknown scanner '$s' in SEC_SCANNERS (want trivy,semgrep)"; IFS=$old_ifs; exit 2 ;;
  esac
done
IFS=$old_ifs
if [ "$known" -eq 0 ]; then
  log "SEC_SCANNERS='$SEC_SCANNERS' selects no known scanner (want trivy,semgrep)"
  exit 2
fi

if ! command -v python3 >/dev/null 2>&1; then
  log "python3 not found — the gate counter needs it"; exit 127
fi

# --- pre-pull pinned images for whichever scanners fall back to docker ---
TRIVY_VIA_DOCKER=0
SEMGREP_VIA_DOCKER=0
if want_scanner trivy; then
  if resolve_trivy; then
    [ "${TRIVY_RUN[0]}" = "docker" ] && TRIVY_VIA_DOCKER=1
  else
    log "trivy: no binary on PATH and no docker fallback (SEC_DOCKER=$SEC_DOCKER)"
    status=1
  fi
fi
if want_scanner semgrep; then
  if resolve_semgrep; then
    [ "${SEMGREP_RUN[0]}" = "docker" ] && SEMGREP_VIA_DOCKER=1
  else
    log "semgrep: no binary on PATH and no docker fallback (SEC_DOCKER=$SEC_DOCKER)"
    status=1
  fi
fi
if [ "$status" -ne 0 ]; then
  exit "$status"
fi
need_pull=()
[ "$TRIVY_VIA_DOCKER" = "1" ] && need_pull+=("$TRIVY_IMAGE")
[ "$SEMGREP_VIA_DOCKER" = "1" ] && need_pull+=("$SEMGREP_IMAGE")
if [ "${#need_pull[@]}" -gt 0 ]; then
  pull_images "${need_pull[@]}" || exit 1
fi

# --- trivy: full report (all severities, vuln+secret+misconfig), then SARIF
#         without secrets for code scanning ---
if want_scanner trivy; then
  log "trivy: filesystem scan (vuln,secret,misconfig)"
  if ! "${TRIVY_RUN[@]}" fs --ignorefile .trivyignore.yaml \
        --scanners vuln,secret,misconfig --format json --output "$OUT/trivy.json" .; then
    log "trivy: scan failed"
    status=1
  elif [ ! -s "$OUT/trivy.json" ]; then
    log "trivy: empty report"
    status=1
  else
    if ! "${TRIVY_RUN[@]}" fs --ignorefile .trivyignore.yaml \
          --scanners vuln,misconfig --format sarif --output "$OUT/trivy.sarif" .; then
      log "trivy: SARIF pass failed"
      status=1
    fi
  fi
fi

# --- semgrep: JSON report + SARIF ---
if want_scanner semgrep; then
  cfgs=()
  for c in $SEMGREP_CONFIGS; do cfgs+=(--config "$c"); done
  log "semgrep: scan ($SEMGREP_CONFIGS)"
  if ! "${SEMGREP_RUN[@]}" scan "${cfgs[@]}" --metrics=off \
        --json --output "$OUT/semgrep.json" .; then
    log "semgrep: scan failed"
    status=1
  elif [ ! -s "$OUT/semgrep.json" ]; then
    log "semgrep: empty report"
    status=1
  else
    if ! "${SEMGREP_RUN[@]}" scan "${cfgs[@]}" --metrics=off \
          --sarif --output "$OUT/semgrep.sarif" .; then
      log "semgrep: SARIF pass failed"
      status=1
    fi
  fi
fi

# --- gate: count severities from the JSON the scans just wrote ---
# Runs even when a scan failed above, so the summary still prints what landed.
gate_status=0
gate_output=$(SEC_FAIL_TRIVY="$SEC_FAIL_TRIVY" SEC_FAIL_SEMGREP="$SEC_FAIL_SEMGREP" \
  SEC_SCANNERS="$SEC_SCANNERS" OUT="$OUT" python3 - <<'PY'
import json, os, sys

out = os.environ["OUT"]
scanners = os.environ["SEC_SCANNERS"].split(",")
summary = []
failures = []

def sev_at_or_above(sev, floor, order):
    return sev in order and order.index(sev) >= order.index(floor)

if "trivy" in scanners:
    try:
        doc = json.load(open(os.path.join(out, "trivy.json")))
    except Exception:
        doc = None
    if doc is not None:
        order = ["UNKNOWN", "LOW", "MEDIUM", "HIGH", "CRITICAL"]
        counts = {s: 0 for s in order}
        misconfig = {s: 0 for s in order}
        for res in doc.get("Results", []):
            for v in res.get("Vulnerabilities") or []:
                counts[v.get("Severity", "UNKNOWN")] = \
                    counts.get(v.get("Severity", "UNKNOWN"), 0) + 1
            for s in res.get("Secrets") or []:
                counts[s.get("Severity", "UNKNOWN")] = \
                    counts.get(s.get("Severity", "UNKNOWN"), 0) + 1
            for m in res.get("Misconfigurations") or []:
                misconfig[m.get("Severity", "UNKNOWN")] = \
                    misconfig.get(m.get("Severity", "UNKNOWN"), 0) + 1
        summary.append("trivy (vuln+secret): " +
            " ".join(f"{s}={counts.get(s, 0)}" for s in reversed(order)) +
            " | misconfig (report-only): " +
            " ".join(f"{s}={misconfig.get(s, 0)}" for s in reversed(order)))
        floor = os.environ["SEC_FAIL_TRIVY"]
        if floor != "off":
            hits = sum(n for s, n in counts.items()
                       if sev_at_or_above(s, floor, order))
            if hits:
                failures.append(
                    f"GATE-FAIL trivy: {hits} finding(s) at or above {floor}")
if "semgrep" in scanners:
    try:
        doc = json.load(open(os.path.join(out, "semgrep.json")))
    except Exception:
        doc = None
    if doc is not None:
        order = ["INFO", "WARNING", "ERROR"]
        counts = {}
        for r in doc.get("results", []):
            sev = r.get("extra", {}).get("severity", "INFO")
            counts[sev] = counts.get(sev, 0) + 1
        summary.append("semgrep: " +
            " ".join(f"{s}={counts.get(s, 0)}" for s in reversed(order)))
        floor = os.environ["SEC_FAIL_SEMGREP"]
        if floor != "off":
            hits = sum(n for s, n in counts.items()
                       if sev_at_or_above(s, floor, order))
            if hits:
                failures.append(
                    f"GATE-FAIL semgrep: {hits} finding(s) at or above {floor}")

with open(os.path.join(out, "summary.md"), "w") as f:
    f.write("## Security scan\n\n")
    for line in summary:
        f.write(f"- {line}\n")
    for line in failures:
        f.write(f"- :x: {line}\n")
for line in summary:
    print(line)
for line in failures:
    print(line)
sys.exit(1 if failures else 0)
PY
)
gate_rc=$?
[ $gate_rc -ne 0 ] && gate_status=1
printf '%s\n' "$gate_output"

if [ "$gate_status" -ne 0 ] || [ "$status" -ne 0 ]; then
  log "FAIL (scans=$([ "$status" -eq 0 ] && echo ok || echo failed), gate=$([ "$gate_status" -eq 0 ] && echo clean || echo tripped))"
  exit 1
fi
log "PASS (reports in $OUT/)"
