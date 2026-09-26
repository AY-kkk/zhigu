#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MODE="${1:-offline}"
if [ -x /Users/chenyufan/sdk/go1.24.2/bin/go ]; then
  export PATH="/Users/chenyufan/sdk/go1.24.2/bin:${PATH:-}"
fi
export GOTOOLCHAIN=local
cd "$ROOT"
case "$MODE" in
  offline)
    python3 - <<'PY'
from pathlib import Path
import json
root=Path('.')
fixture=root/'contracts/intel/fixtures/events-v1'
for name in ('manifest.json','sources.json','extractions.json','expected.json'):
    json.loads((fixture/name).read_text())
text=(root/'contracts/intel/openapi.yaml').read_text()
for path in ('/session','/demo-sessions','/watchlist','/events','/notifications','/replay/actions'):
    assert path in text, path
print('intel contracts: ok')
PY
    (cd app/server && go test ./service/intel ./api/v1/intel ./initialize -count=1)
    (cd app/web && VITE_INTEL_ENABLED=true npm run build)
    (cd app/web && npx playwright test e2e/intel.spec.js --project=chromium)
    python3 app/scripts/intel_manifest.py --mode offline --status pass
    ;;
  integration)
    bash "$0" offline
    (cd app/web && E2E_INTEL_CHAIN=1 npx playwright test e2e/intel-chain.spec.js --config playwright.intel.config.js --project=chromium)
    (cd app/server && go test -race ./service/intel ./api/v1/intel -count=1)
    python3 app/scripts/intel_manifest.py --mode integration --status partial
    ;;
  live-data)
    if [ "${ZHIGU_INTEL_LIVE_TEST:-0}" != "1" ] || [ -z "${ZHIGU_INTEL_PROVIDERS:-}" ]; then
      echo "blocked: live-data requires ZHIGU_INTEL_LIVE_TEST=1 and ZHIGU_INTEL_PROVIDERS" >&2
      python3 app/scripts/intel_manifest.py --mode live-data --status blocked
      exit 2
    fi
    case ",${ZHIGU_INTEL_PROVIDERS}," in
      *,fuyao,*) [ -n "${FUYAO_API_KEY:-}" ] || { echo "blocked: FUYAO_API_KEY missing" >&2; python3 app/scripts/intel_manifest.py --mode live-data --status blocked; exit 2; } ;;
    esac
    case ",${ZHIGU_INTEL_PROVIDERS}," in
      *,cninfo,*) [ "${ZHIGU_CNINFO_LIVE_VERIFIED:-0}" = "1" ] && [ -n "${ZHIGU_LIVE_CNINFO_SYMBOL:-}" ] && [ -n "${ZHIGU_LIVE_CNINFO_ORG_ID:-}" ] || { echo "blocked: CNINFO verification/query mapping missing" >&2; python3 app/scripts/intel_manifest.py --mode live-data --status blocked; exit 2; } ;;
    esac
    case ",${ZHIGU_INTEL_PROVIDERS}," in
      *,ifind,*) [ -n "${IFIND_MCP_URL:-}" ] && [ -n "${IFIND_AUTHORIZATION:-}" ] || { echo "blocked: IFIND credentials missing" >&2; python3 app/scripts/intel_manifest.py --mode live-data --status blocked; exit 2; } ;;
    esac
    (cd app/server && go test ./service/intel -run TestLiveProviderEvidence -count=1)
    python3 app/scripts/intel_manifest.py --mode live-data --status pass
    ;;
  live-model)
    if [ "${ZHIGU_INTEL_LIVE_TEST:-0}" != "1" ] || [ -z "${ZHIGU_INTEL_MODEL_CONFIG_ID:-}" ] || [ -z "${ZHIGU_INTEL_MODEL_CONFIG_DIGEST:-}" ] || [ -z "${ZHIGU_INTEL_MODEL_PROTOCOL:-}" ] || [ -z "${ZHIGU_INTEL_MODEL:-}" ] || [ -z "${ZHIGU_INTEL_MODEL_BASE_URL:-}" ] || [ -z "${ZHIGU_INTEL_MODEL_API_KEY:-}" ]; then
      echo "blocked: live-model requires frozen model env and explicit ZHIGU_INTEL_LIVE_TEST=1" >&2
      python3 app/scripts/intel_manifest.py --mode live-model --status blocked
      exit 2
    fi
    (cd app/server && go test ./service/intel -run TestLiveModelExtraction -count=1)
    python3 app/scripts/intel_manifest.py --mode live-model --status pass
    ;;
  regression)
    (cd app/server && go test ./service/finance ./service/workbench ./service/strategy ./service/backtest ./service/market -count=1)
    (cd app/web && VITE_INTEL_ENABLED=true npm run build)
    python3 app/scripts/intel_manifest.py --mode regression --status pass
    ;;
  browser-matrix)
    (cd app/web && npx playwright test e2e/intel.spec.js --config playwright.intel.config.js --project=firefox --project=webkit)
    ;;
  video)
    bash app/scripts/create-intel-video.sh
    ;;
  *)
    echo "usage: bash app/scripts/verify-intel.sh {offline|integration|browser-matrix|live-data|live-model|regression|video}" >&2
    exit 2
    ;;
esac
