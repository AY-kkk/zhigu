#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
MODE="${1:-offline}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-local}"

have_model_key() {
  [[ -n "${ZHIGU_MODEL_API_KEY:-}" || -n "${OPENAI_API_KEY:-}" || -n "${DEEPSEEK_API_KEY:-}" ]]
}

run_offline() {
  (cd "$ROOT/app/server" && go test ./... -count=1 -timeout 180s)
  (cd "$ROOT/app/research-service" && PYTHONPATH=. .venv/bin/python -m pytest tests -q -p no:cacheprovider)
  (cd "$ROOT/app/web" && npm run build)
}

run_integration() {
  run_offline
  (cd "$ROOT/app/server" && go test -race ./service/finance -count=1 -timeout 180s)
}

run_live() {
  if ! have_model_key && [[ "${ZHIGU_STAGE_B_LIVE_DATA:-}" != "1" ]]; then
    echo "live blocked: no model key and ZHIGU_STAGE_B_LIVE_DATA!=1" >&2
    exit 2
  fi
  if [[ "${ZHIGU_STAGE_B_LIVE_DATA:-}" == "1" ]]; then
    (cd "$ROOT/app/server" && go test ./service/finance -run 'TestLiveDataSampleGated|TestLiveThreeStatementsAndHK' -count=1 -timeout 180s)
  fi
  if ! have_model_key; then
    echo "model live skipped: credentials not configured (D-02/D-05)" >&2
    exit 2
  fi
  echo "model live is not enabled in this script until D-05 fee cap is written" >&2
  exit 2
}

case "$MODE" in
  offline) run_offline ;;
  integration) run_integration ;;
  live) run_live ;;
  all)
    run_integration
    run_live
    ;;
  *)
    echo "usage: $0 offline|integration|live|all" >&2
    exit 3
    ;;
esac
