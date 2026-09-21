#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
MODE="${1:-offline}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-local}"
export PATH="${HOME}/sdk/go1.24.2/bin:${PATH}"

run_offline() {
  (cd "$ROOT/app/server" && go test ./service/indicators ./service/strategy ./service/backtest ./service/market ./service/workbench -count=1 -timeout 180s)
  (cd "$ROOT/app/web" && npm run build)
  (cd "$ROOT/app/web" && CI=1 npx playwright test e2e/strategy.spec.js --reporter=line)
}

run_integration() {
  run_offline
  (cd "$ROOT/app/server" && go test ./service/workbench -count=1 -timeout 180s)
}

run_live_data() {
  if [[ "${ZHIGU_STRATEGY_LIVE_DATA:-}" != "1" ]]; then
    echo "live-data blocked: set ZHIGU_STRATEGY_LIVE_DATA=1 to fetch East Money + Cninfo" >&2
    exit 2
  fi
  mkdir -p "$ROOT/artifacts/strategy"
  (cd "$ROOT/app/server" && ZHIGU_STRATEGY_LIVE_DATA=1 ZHIGU_STRATEGY_COVERAGE_OUT="$ROOT/artifacts/strategy" go test ./service/market -run TestLiveCoverageAudit -count=1 -timeout 12m -v)
}

run_live_model() {
  if [[ -z "${ZHIGU_MODEL_API_KEY:-}" && -z "${OPENAI_API_KEY:-}" ]]; then
    echo "live-model blocked: no model credentials in env; product live uses admin-activated model config" >&2
    exit 2
  fi
  echo "live-model env key is not a signed acceptance. Product live uses admin-activated model config." >&2
  exit 2
}

case "$MODE" in
  offline) run_offline ;;
  integration) run_integration ;;
  live-data) run_live_data ;;
  live-model) run_live_model ;;
  all)
    run_integration
    run_live_data
    ;;
  *)
    echo "usage: $0 offline|integration|live-data|live-model|all" >&2
    exit 3
    ;;
esac
