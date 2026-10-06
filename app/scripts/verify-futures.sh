#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MODE="${1:---offline}"
GO_BIN="${GO_BIN:-$HOME/sdk/go1.24.2/bin/go}"
PYTHON_BIN="${FUTURES_PYTHON:-$ROOT/app/futures-service/.venv/bin/python}"

if [[ ! -x "$PYTHON_BIN" ]] || ! "$PYTHON_BIN" -c 'import fastapi, httpx, pydantic' >/dev/null 2>&1; then
  PYTHON_BIN="$ROOT/app/research-service/.venv/bin/python"
  echo "WARN: app/futures-service/.venv is missing; using the existing Python 3.12 runtime for offline tests only." >&2
fi

run_offline() {
  echo "== futures offline: contracts =="
  "$PYTHON_BIN" "$ROOT/app/scripts/verify-futures-contracts.py"
  echo "== futures offline: Python worker =="
  (cd "$ROOT/app/futures-service" && PYTHONPATH=src "$PYTHON_BIN" -m unittest discover -s tests -v)
  echo "== futures offline: Go =="
  (cd "$ROOT/app/server" && GOCACHE="${GOCACHE:-/private/tmp/zhigu-go-cache}" "$GO_BIN" test ./service/futures/... ./api/v1/futures/... ./model/futures/...)
  echo "== futures offline: Web unit/compile =="
  (cd "$ROOT/app/web" && node --test src/modules/futures/tests/*.test.mjs)
  echo "PASS: offline contract, worker, Go and Web tests. This is not live-source/model/UI/performance acceptance."
}

run_ui() {
  echo "== futures UI: build and browser contract =="
  (cd "$ROOT/app/web" && npm run build && npx playwright test e2e/futures.spec.js --reporter=line)
}

blocked_gate() {
  local name="$1"
  echo "NOT RUN: $name requires explicit source/model/load authorization and captured evidence."
  echo "BLOCKED: do not interpret this command as a passed production gate."
  return 2
}

case "$MODE" in
  --offline) run_offline ;;
  --ui) run_ui ;;
  --real-sources) blocked_gate "real source probe and five-trading-day stability" ;;
  --real-model) blocked_gate "30-call real-model quality evaluation" ;;
  --performance) blocked_gate "20-session/10RPS/15-minute load and old-module baseline" ;;
  --rollback) blocked_gate "read_only/off and application rollback drill" ;;
  --all)
    run_offline
    run_ui
    blocked_gate "real-source gate"
    blocked_gate "real-model gate"
    blocked_gate "performance gate"
    blocked_gate "rollback gate"
    ;;
  *) echo "usage: $0 [--offline|--ui|--real-sources|--real-model|--performance|--rollback|--all]" >&2; exit 64 ;;
esac
