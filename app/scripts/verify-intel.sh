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
  live-data|live-model)
    echo "blocked: $MODE requires explicit provider/model authorization and credentials" >&2
    python3 app/scripts/intel_manifest.py --mode "$MODE" --status blocked || true
    exit 2
    ;;
  regression)
    (cd app/server && go test ./service/finance ./service/workbench ./service/strategy ./service/backtest ./service/market -count=1)
    (cd app/web && VITE_INTEL_ENABLED=true npm run build)
    python3 app/scripts/intel_manifest.py --mode regression --status pass
    ;;
  video)
    bash app/scripts/create-intel-video.sh
    ;;
  *)
    echo "usage: bash app/scripts/verify-intel.sh {offline|integration|live-data|live-model|regression|video}" >&2
    exit 2
    ;;
esac
