#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MODE="${1:---static}"
run_static() {
  app/research-service/.venv/bin/python -m unittest discover -s app/tests/contract -p 'test_*.py' -v
  app/research-service/.venv/bin/python app/scripts/verify-futures-contracts.py
  bash -n app/scripts/production-preflight.sh app/scripts/backup.sh app/scripts/restore-verify.sh app/scripts/maintenance-run.sh app/scripts/maintenance-watchdog.sh app/scripts/upgrade-verify.sh app/scripts/rollback.sh app/scripts/run-performance.sh
}
run_full() {
  run_static
  app/scripts/production-preflight.sh
  (cd app/server && GOCACHE="${GOCACHE:-/tmp/zhigu-go-cache}" go test ./... -count=1)
  (cd app/futures-service && PYTHONPATH=src .venv/bin/python -m unittest discover -s tests -p 'test_*.py')
  (cd app/research-service && .venv/bin/python -m pytest tests -q -k 'not actual_harness_import')
  (cd app/web && npm run build)
}
case "$MODE" in
  --static) run_static ;;
  --full) run_full ;;
  *) echo "usage: $0 [--static|--full]" >&2; exit 64 ;;
esac
echo "PASS: production verifier $MODE"
