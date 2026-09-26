#!/usr/bin/env bash
# 阶段 B 统一证据入口（研究 + 策略分报 manifest）。
# 退出码：0 通过；1 断言失败；2 环境/凭据/授权阻塞；3 必需 skip/空集/证据不全。
# 退出 0 不等于用户已签收。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
MODE="${1:-offline}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-local}"

RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)-$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || echo noSHA)"
ART="$ROOT/artifacts/stage-b/$RUN_ID"
STRAT_DIR="$ART/strategy"
RESULTS="$ART/results.tsv"
mkdir -p "$STRAT_DIR"
: > "$RESULTS"
STARTED="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
FAILED=0
EMPTY_GUARD=0

have_model_key() {
  [[ -n "${ZHIGU_MODEL_API_KEY:-}" || -n "${OPENAI_API_KEY:-}" || -n "${DEEPSEEK_API_KEY:-}" ]]
}

log_path() {
  local domain="$1" pkg="$2"
  local name
  name="$(echo "${domain}-${pkg}" | tr '/.' '__').log"
  if [[ "$domain" == "strategy" ]]; then
    echo "$STRAT_DIR/$name"
  else
    echo "$ART/$name"
  fi
}

record() {
  printf '%s\t%s\t%s\t%s\n' "$1" "$2" "$3" "$4" >> "$RESULTS"
}

record_go_log() {
  local domain="$1" pkg="$2" log="$3"
  awk -v pkg="$pkg" -v dom="$domain" '/^ *--- (PASS|FAIL|SKIP): / {
    st = tolower($2); sub(/:$/, "", st)
    print dom "\t" pkg "\t" $3 "\t" st
  }' "$log" >> "$RESULTS"
}

run_go() {
  local domain="$1" pkg="$2"
  shift 2
  local label="$pkg" arg code log
  for arg in "$@"; do
    if [[ "$arg" == "-race" ]]; then
      label="${pkg}-race"
    fi
  done
  log="$(log_path "$domain" "$label")"
  set +e
  (cd "$ROOT/app/server" && go test "$pkg" -v -count=1 -timeout 300s "$@") > "$log" 2>&1
  code=$?
  set -e
  record_go_log "$domain" "$label" "$log"
  if [[ $code -ne 0 ]]; then
    FAILED=1
    echo "FAILED: go test $pkg (see $log)" >&2
  fi
}

run_pytest() {
  local log="$ART/research-service-pytest.log" code
  set +e
  (cd "$ROOT/app/research-service" && PYTHONPATH=. .venv/bin/python -m pytest tests -v --tb=short -p no:cacheprovider) > "$log" 2>&1
  code=$?
  set -e
  awk '/::/ && ($2 == "PASSED" || $2 == "FAILED" || $2 == "SKIPPED") {
    st = tolower($2)
    if (st == "passed") st = "pass"; else if (st == "failed") st = "fail"; else if (st == "skipped") st = "skip"
    print "research\tresearch-service\t" $1 "\t" st
  }' "$log" >> "$RESULTS"
  if [[ $code -ne 0 ]]; then
    FAILED=1
    echo "FAILED: pytest (see $log)" >&2
  fi
}

run_web_build() {
  local log="$ART/web-build.log" code
  set +e
  (cd "$ROOT/app/web" && npm run build) > "$log" 2>&1
  code=$?
  set -e
  if [[ $code -eq 0 ]]; then
    record strategy "app/web" "npm_run_build" pass
  else
    record strategy "app/web" "npm_run_build" fail
    FAILED=1
    echo "FAILED: npm run build (see $log)" >&2
  fi
}

run_e2e() {
  local log="$ART/e2e-strategy.json" code
  set +e
  (cd "$ROOT/app/web" && npx playwright test e2e/strategy-market.spec.js e2e/strategy.spec.js e2e/stage-b.spec.js --reporter=json) > "$log" 2>&1
  code=$?
  set -e
  record_playwright_json "$log"
  if [[ $code -ne 0 ]]; then
    FAILED=1
    echo "FAILED: playwright e2e (see $log)" >&2
  fi
}

record_playwright_json() {
  python3 - "$1" >> "$RESULTS" <<'PYEOF'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8", errors="replace"))


def walk(suite, file_hint):
    spec_file = suite.get("file") or file_hint
    dom = "research" if "stage-b" in spec_file and "strategy" not in spec_file else "strategy"
    for spec in suite.get("specs", []):
        st = "skip"
        for t in spec.get("tests", []):
            for res in t.get("results", []):
                status = res.get("status")
                if status == "passed":
                    st = "pass"
                elif status in ("failed", "timedOut"):
                    st = "fail"
                elif status == "skipped" and st != "fail" and st != "pass":
                    st = "skip"
        print(f"{dom}\tapp/web/e2e/{spec_file}\t{spec['title']}\t{st}")
    for sub in suite.get("suites", []):
        walk(sub, spec_file)


for s in data.get("suites", []):
    walk(s, "e2e")
PYEOF
}

run_chain() {
  # R7：真实 Vue→Go→PostgreSQL/worker 链路（复制→编辑→保存→回测→结果）。
  # 行情为服务端明确标识的 fixture；实际请求不替换为静态成功体。
  local log="$ART/e2e-chain.json" code i
  local dsn_file="$ART/chain.dsn" devpg_log="$ART/chain-devpg.log" go_log="$ART/chain-go.log"
  rm -f "$dsn_file"
  (cd "$ROOT/app/server" && ZHIGU_DEVPG_DIR="$ART/devpg" ZHIGU_DEVPG_DSN_FILE="$dsn_file" go run ./cmd/devpg) > "$devpg_log" 2>&1 &
  local devpg_pid=$!
  for i in $(seq 1 90); do
    [[ -s "$dsn_file" ]] && break
    sleep 1
  done
  if [[ ! -s "$dsn_file" ]]; then
    echo "chain blocked: isolated postgres did not start (see $devpg_log)" >&2
    kill "$devpg_pid" 2>/dev/null || true
    FAILED=1
    return
  fi
  (cd "$ROOT/app/server" && ZHIGU_RESEARCH_MODE=unit-test ZHIGU_POSTGRES_DSN="$(cat "$dsn_file")" ZHIGU_MARKET_MODE=fixture ZHIGU_HTTP_ADDR=127.0.0.1:8080 go run .) > "$go_log" 2>&1 &
  local go_pid=$!
  for i in $(seq 1 90); do
    curl -fsS http://127.0.0.1:8080/healthz > /dev/null 2>&1 && break
    kill -0 "$go_pid" 2>/dev/null || break
    sleep 1
  done
  set +e
  (cd "$ROOT/app/web" && npx playwright test e2e/strategy-market-chain.spec.js --reporter=json) > "$log" 2>&1
  code=$?
  set -e
  kill "$go_pid" "$devpg_pid" 2>/dev/null || true
  wait "$go_pid" "$devpg_pid" 2>/dev/null || true
  if [[ -s "$log" ]]; then
    record_playwright_json "$log"
  else
    record strategy "app/web/e2e/strategy-market-chain.spec.js" "copy edit save backtest use same version" fail
  fi
  if [[ $code -ne 0 ]]; then
    FAILED=1
    echo "FAILED: real chain (see $log / $go_log)" >&2
  fi
}

require_tests() {
  local label="$1" pattern="$2"
  if ! grep -q "$pattern" "$RESULTS"; then
    echo "EMPTY TEST SET: $label ($pattern)" >&2
    EMPTY_GUARD=1
  fi
}

collect_server_offline() {
  # §12.10 策略线 offline：Schema 与规则测试不可空集（下方 require_tests 强制）。
  run_go research ./service/finance
  run_go strategy ./initialize
  run_go strategy ./service/strategy
  run_go strategy ./service/strategy_market
  run_go strategy ./service/workbench
  run_go strategy ./service/backtest
  run_go strategy ./service/market
  run_go strategy ./service/indicators
  run_go strategy ./api/v1/finance
  run_pytest
  run_web_build
  require_tests "契约一致性" "TestContractsMatchRuntime"
  require_tests "规则编辑器" "TestEditorRepeatExpansion"
  require_tests "迁移执行器" "TestMigrateFreshDatabaseAndRepeat"
  require_tests "市场复制并发" "TestCopyConcurrencyOneDraft"
  require_tests "研究线回归" "TestStageB"
}

write_manifest() {
  local mode="$1" finished wt_hash runtime locks prereq limits
  finished="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  wt_hash="$(git -C "$ROOT" status --porcelain | shasum | cut -d' ' -f1)"
  runtime="go=$(go version 2>/dev/null | awk '{print $3}'); node=$(node -v 2>/dev/null); python=$(python3 --version 2>/dev/null | awk '{print $2}')"
  locks="go.sum=$(shasum "$ROOT/app/server/go.sum" 2>/dev/null | cut -d' ' -f1); package-lock.json=$(shasum "$ROOT/app/web/package-lock.json" 2>/dev/null | cut -d' ' -f1); uv.lock=$(shasum "$ROOT/app/research-service/uv.lock" 2>/dev/null | cut -d' ' -f1)"
  prereq="D-01 独立宿主:verified_local|D-08 隔离PostgreSQL(embedded):verified_by_migration_tests|D-02 模型Key:$(have_model_key && echo present || echo blocked)|D-05 费用上限:$( [[ -n "${ZHIGU_D05_FEE_CAP:-}" ]] && echo written || echo blocked)|D-09 正式市场内容:$( [[ -n "${ZHIGU_MARKET_CONTENT_READY:-}" ]] && echo provided || echo none)"
  limits="e2e 为受控 UI（API 替身），真链路由 Go 集成测试分层覆盖|live 未执行时相关需求标 not_run_in_mode/blocked|B-S-10 正式内容闭环在无已审核上架内容时保持缺口|Chat/Responses live 生成与解释在无凭据时记「实现已测、live 未验证」"
  python3 "$ROOT/app/scripts/stage_b_manifest.py" \
    --mode "$mode" \
    --results "$RESULTS" \
    --out "$ART/manifest.json" \
    --git-sha "$(git -C "$ROOT" rev-parse HEAD)" \
    --working-tree-hash "$wt_hash" \
    --started "$STARTED" \
    --finished "$finished" \
    --runtime "$runtime" \
    --locks "$locks" \
    --prereq "$prereq" \
    --limitations "$limits"
  {
    echo "# stage-B verify summary ($mode)"
    echo
    echo "- run: $RUN_ID"
    echo "- results: results.tsv（精确测试节点与状态）"
    echo "- failed=$FAILED empty_guard=$EMPTY_GUARD"
    echo "- manifest.json：research 与 strategy 分报，requirements 精确到 test 节点"
  } > "$ART/summary.md"
}

finish() {
  local mrc=0
  write_manifest "$1" || mrc=$?
  if [[ $FAILED -ne 0 ]]; then
    exit 1
  fi
  # R7-E2：manifest 缺证据（missing/failed/必需空跑）→ 不完整退出码 3。
  if [[ $EMPTY_GUARD -ne 0 || $mrc -ne 0 ]]; then
    exit 3
  fi
}

run_offline() {
  collect_server_offline
  finish offline
}

run_integration() {
  collect_server_offline
  run_go research ./service/finance -race
  run_go strategy ./service/strategy_market -race
  run_go strategy ./service/workbench -race
  run_e2e
  run_chain
  finish integration
}

run_live() {
  local live_note=""
  if [[ "${ZHIGU_STAGE_B_LIVE_DATA:-}" == "1" ]]; then
    (cd "$ROOT/app/server" && go test ./service/finance -run 'TestLiveDataSampleGated|TestLiveThreeStatementsAndHK' -count=1 -timeout 180s)
    live_note="live-data 已跑"
  fi
  # 策略线 live 分项：来源审核、真实行情/费用/公司行动、实际模型生成与样本回测；
  # 两协议可用性分开记录，无 Key 不伪通过（§12.10）。
  echo "strategy live 分项（市场来源审核、真实行情/费用/公司行动、真实生成与样本回测）需 D-02/D-05 与正式内容；未满足即 blocked" >&2
  write_manifest live || true
  if ! have_model_key && [[ "${ZHIGU_STAGE_B_LIVE_DATA:-}" != "1" ]]; then
    echo "live blocked: no model key and ZHIGU_STAGE_B_LIVE_DATA!=1${live_note:+ ($live_note)}" >&2
    exit 2
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
    collect_server_offline
    run_go research ./service/finance -race
    run_go strategy ./service/strategy_market -race
    run_go strategy ./service/workbench -race
    run_e2e
    run_chain
    finish all
    run_live
    ;;
  *)
    echo "usage: $0 offline|integration|live|all" >&2
    exit 3
    ;;
esac
