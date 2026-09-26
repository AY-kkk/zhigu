#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT/app/web"
npx playwright test e2e/intel-video.spec.js --project=chromium
VIDEO="$(find test-results -name 'video.webm' -print | tail -n 1)"
test -n "$VIDEO"
cp "$VIDEO" "$ROOT/artifacts/intel/投资事件情报演示_60s.webm"
printf '%s\n' "$ROOT/artifacts/intel/投资事件情报演示_60s.webm"
