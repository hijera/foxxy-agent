#!/usr/bin/env bash
# Full ACP stdio e2e. Expects ./build/foxxycode (examples/build_foxxycode.sh links scheduler with HTTP for one binary).

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

ACP_DIR="$ROOT/examples/acp"

export FOXXYCODE_BIN="${FOXXYCODE_BIN:-$ROOT/build/foxxycode}"
export FOXXYCODE_CONFIG="${FOXXYCODE_CONFIG:-$ROOT/examples/config.demo.yaml}"
export SESSION_ROOT="${SESSION_ROOT:-/tmp/foxxycode-examples-acp}"
export SESSION_ID="${SESSION_ID:-example-acp}"

if [[ ! -x "$FOXXYCODE_BIN" ]]; then
  echo "binary not found, run: ./examples/build_foxxycode.sh" >&2
  exit 1
fi

python3 "$ACP_DIR/acp_smoke_gateway.py"
python3 "$ACP_DIR/acp_e2e_models.py"
python3 "$ACP_DIR/acp_e2e_web.py"
python3 "$ACP_DIR/acp_e2e_todo.py"
python3 "$ACP_DIR/acp_e2e_skills_slash.py"
python3 "$ACP_DIR/acp_e2e_rules.py"
python3 "$ACP_DIR/acp_e2e_memory.py"
python3 "$ACP_DIR/acp_e2e_toolcalls_persist.py"
python3 "$ACP_DIR/acp_e2e_scheduler_agent.py"
python3 "$ACP_DIR/acp_e2e_plan_files.py"

echo "ok acp tests"
