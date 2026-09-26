#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MODE="${1:-offline}"
case "$MODE" in
  offline|integration|live|eval) ;;
  *) echo "usage: bash app/scripts/verify-research-viewpoint.sh offline|integration|live|eval" >&2; exit 3 ;;
esac

GO_BIN="${GO_BIN:-$(command -v go || true)}"
if [[ -z "$GO_BIN" ]]; then
  echo "go binary not found" >&2
  exit 2
fi
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
ART="$ROOT/artifacts/research-viewpoint/$STAMP-${MODE}"
mkdir -p "$ART"
RESULTS="$ART/results.tsv"
: > "$RESULTS"

record() {
  printf '%s\t%s\t%s\t%s\n' "$1" "$2" "$3" "$4" >> "$RESULTS"
}

run_go() {
  local label="$1"; shift
  local log="$ART/${label}.log"
  if (cd "$ROOT/app/server" && GOTOOLCHAIN=local "$GO_BIN" test "$@" -v -count=1 -timeout 300s) >"$log" 2>&1; then
    python3 - "$log" "$RESULTS" <<'PY'
import re, sys
log, results = sys.argv[1:]
names = []
for line in open(log, encoding="utf-8", errors="replace"):
    m = re.match(r"=== RUN\s+([^/\s]+)", line)
    if m:
        names.append(m.group(1))
with open(results, "a", encoding="utf-8") as out:
    for name in sorted(set(names)):
        out.write(f"research\tapp/server\t{name}\tpass\n")
    if not names:
        out.write("research\tapp/server\tgo-finance-tests\tpass\n")
PY
  else
    record research app/server "$label" fail
    cat "$log" >&2
    return 1
  fi
}
run_py() {
  local log="$ART/python-tests.log"
  if (cd "$ROOT/app/research-service" && PYTHONPATH=. ZHIGU_RESEARCH_SQLITE="$ART/jobs.sqlite" .venv/bin/python -m pytest tests -v -k 'not actual_harness_import') >"$log" 2>&1; then
    python3 - "$log" "$RESULTS" <<'PY'
import re, sys
log, results = sys.argv[1:]
names = []
for line in open(log, encoding="utf-8", errors="replace"):
    m = re.search(r"::(test_[A-Za-z0-9_]+)\s+PASSED", line)
    if m:
        names.append(m.group(1))
with open(results, "a", encoding="utf-8") as out:
    for name in sorted(set(names)):
        out.write(f"research\tapp/research-service\t{name}\tpass\n")
    if not names:
        out.write("research\tapp/research-service\tpython-tests\tpass\n")
PY
  else
    record research app/research-service "python-tests" fail
    cat "$log" >&2
    return 1
  fi
}
run_web() {
  local log="$ART/web-build.log"
  if (cd "$ROOT/app/web" && npm run build) >"$log" 2>&1; then
    record research app/web "web-build" pass
  else
    record research app/web "web-build" fail
    cat "$log" >&2
    return 1
  fi
  local e2e_log="$ART/web-research-e2e.log"
  if (cd "$ROOT/app/web" && npx playwright test e2e/editorial.spec.js e2e/stage-b.spec.js e2e/research-viewpoint-input.spec.js e2e/research-viewpoint-report.spec.js) >"$e2e_log" 2>&1; then
    record research app/web "research-viewpoint-input.spec.js" pass
    record research app/web "research-viewpoint-report.spec.js" pass
  else
    record research app/web "research-viewpoint-input.spec.js" fail
    record research app/web "research-viewpoint-report.spec.js" fail
    cat "$e2e_log" >&2
    return 1
  fi
}

case "$MODE" in
  live)
    if [[ "${ZHIGU_STAGE_B_LIVE_DATA:-}" != "1" || -z "${ZHIGU_RESEARCH_MODEL_KEY:-}" ]]; then
      record research live "live-data-and-model" blocked
      python3 "$ROOT/app/scripts/research_viewpoint_manifest.py" --mode live --results "$RESULTS" --out "$ART" || true
      echo "live blocked: ZHIGU_STAGE_B_LIVE_DATA=1 and ZHIGU_RESEARCH_MODEL_KEY are required" >&2
      exit 2
    fi
    ;;
  eval)
    if [[ ! -f "$ROOT/tests/quality/research-viewpoint-set50.json" ]]; then
      record research eval "evaluate-research-viewpoint" blocked
      python3 "$ROOT/app/scripts/research_viewpoint_manifest.py" --mode eval --results "$RESULTS" --out "$ART" || true
      echo "eval blocked: tests/quality/research-viewpoint-set50.json missing" >&2
      exit 2
    fi
    if [[ -z "${ZHIGU_RESEARCH_EVAL_PREDICTIONS:-}" ]]; then
      record research eval "evaluate-research-viewpoint" blocked
      python3 "$ROOT/app/scripts/research_viewpoint_manifest.py" --mode eval --results "$RESULTS" --out "$ART" || true
      echo "eval blocked: ZHIGU_RESEARCH_EVAL_PREDICTIONS is required" >&2
      exit 2
    fi
    ;;
esac

run_go "go-finance-tests" ./service/finance
run_py
run_web
if [[ "$MODE" == "integration" ]]; then
  run_go "research-viewpoint-chain" ./service/finance -run TestResearchViewpointCrossProcessFixture
fi

if [[ "$MODE" == "eval" ]]; then
  if python3 "$ROOT/app/scripts/evaluate-research-viewpoint.py" \
    --input "$ROOT/tests/quality/research-viewpoint-set50.json" \
    --predictions "$ZHIGU_RESEARCH_EVAL_PREDICTIONS" \
    --out "$ART/eval.json"; then
    record research eval "evaluate-research-viewpoint" pass
  else
    record research eval "evaluate-research-viewpoint" fail
  fi
fi

python3 "$ROOT/app/scripts/research_viewpoint_manifest.py" --mode "$MODE" --results "$RESULTS" --out "$ART"
echo "artifacts: $ART"
echo "results: $RESULTS"
echo "manifest: $ART/manifest.json"
