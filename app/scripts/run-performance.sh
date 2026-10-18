#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT="${1:-$ROOT/artifacts/performance}"
BASE_URL="${ZHIGU_PUBLIC_ORIGIN:-http://127.0.0.1}"
mkdir -p "$OUT"
if ! curl -fsS --connect-timeout 2 --max-time 5 "$BASE_URL/api/v1/futures/capabilities" >/dev/null; then
  echo "NOT RUN: performance target unavailable at $BASE_URL" >&2
  exit 2
fi
run() { python3 "$ROOT/app/tests/performance/http_load.py" --output "$OUT/$1.json" --url "$BASE_URL$2" --rps "$3" --duration "$4" --max-p95-ms "$5"; }
run health-capabilities /api/v1/futures/capabilities 20 900 300
run readonly-api /api/finance/intel/v1/events 10 900 800
echo "NOT RUN: mutation, queue, upload and restart scenarios require their dedicated acceptance harnesses" >&2
exit 2
