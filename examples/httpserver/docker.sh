#!/usr/bin/env bash
# Smoke foxxycode http inside docker compose (see repo docker-compose.dev.yml). Needs docker and docker compose.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
HTTP_DIR="$ROOT/examples/httpserver"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker not found" >&2
  exit 1
fi
if ! docker compose version >/dev/null 2>&1; then
  echo "docker compose not found" >&2
  exit 1
fi

PORT="${PORT:-12345}"
export PORT

TMP_DIR="$(mktemp -d -t foxxycode-docker-http-XXXXXX)"
cleanup() {
  docker compose -f docker-compose.dev.yml down -v --remove-orphans >/dev/null 2>&1 || true
  if [[ "${KEEP_TMP:-0}" != "1" ]]; then
    rm -rf "$TMP_DIR" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

FOXXYCODE_CWD="${FOXXYCODE_CWD:-$TMP_DIR/workspace}"
FOXXYCODE_HOME="${FOXXYCODE_HOME:-$TMP_DIR/foxxycode_home}"
FOXXYCODE_CONFIG="${FOXXYCODE_CONFIG:-$TMP_DIR/config.yaml}"

mkdir -p "$FOXXYCODE_CWD" "$FOXXYCODE_HOME"

python3 - "$ROOT/examples/config.demo.yaml" "$FOXXYCODE_CONFIG" "$PORT" <<'PY'
from __future__ import annotations

import sys

src_path = sys.argv[1]
dst_path = sys.argv[2]
port = sys.argv[3]

raw = open(src_path, "r", encoding="utf-8").read()
raw = raw.replace('host: "127.0.0.1"', 'host: "0.0.0.0"')
raw = raw.replace("port: 19876", f"port: {port}", 1)

# Make container logs visible in `docker compose logs`.
raw = raw.replace('outputs: ["file"]', 'outputs: ["stderr"]')
raw = raw.replace('file: "__E2E_LOG_PATH__"', 'file: ""')

open(dst_path, "w", encoding="utf-8").write(raw)
PY

if [[ ! -f "$FOXXYCODE_CONFIG" ]]; then
  echo "config path is not a file: $FOXXYCODE_CONFIG" >&2
  ls -la "$(dirname "$FOXXYCODE_CONFIG")" >&2 || true
  exit 1
fi

if [[ ! -s "$FOXXYCODE_CONFIG" ]]; then
  echo "config file is empty: $FOXXYCODE_CONFIG" >&2
  exit 1
fi

export FOXXYCODE_CWD FOXXYCODE_HOME FOXXYCODE_CONFIG

docker compose -f docker-compose.dev.yml up -d --build foxxycode

ready=0
for _ in $(seq 1 120); do
  if curl -sf -o /dev/null "http://127.0.0.1:${PORT}/v1/models"; then
    ready=1
    break
  fi
  sleep 0.25
done
if [[ "$ready" != 1 ]]; then
  echo "http server did not become ready on port ${PORT}" >&2
  docker compose -f docker-compose.dev.yml ps -a || true
  docker compose -f docker-compose.dev.yml logs foxxycode || true
  exit 1
fi

export BASE_URL="http://127.0.0.1:${PORT}/v1"
python3 "$HTTP_DIR/http_smoke_gateway.py"

echo "ok docker httpserver tests"
