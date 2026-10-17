#!/usr/bin/env bash
set -euo pipefail
ROOT="${ZHIGU_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
COMPOSE_FILE="${COMPOSE_FILE:-$ROOT/app/deploy/compose.prod.yaml}"
ONCE="${1:-}"
BACKOFF=1
alert() {
  printf '{"timestamp":"%s","level":"error","service":"zhigu-watchdog","code":"HEALTH_RESTART","message":"%s"}\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$1" >&2
  if [[ -n "${ALERT_COMMAND:-}" ]]; then printf '%s' "$1" | if command -v timeout >/dev/null 2>&1; then timeout 30s sh -c "$ALERT_COMMAND"; else sh -c "$ALERT_COMMAND"; fi; fi
}
while :; do
  failed=0
  for item in "http://127.0.0.1:8080/livez" "http://127.0.0.1:8091/livez" "http://127.0.0.1:8092/livez"; do
    if ! curl -fsS --max-time 3 "$item" >/dev/null; then
      alert "health failed: $item"
      failed=1
    fi
  done
  if [[ "$failed" == 1 ]]; then
    docker compose -f "$COMPOSE_FILE" restart server research futures || true
    BACKOFF=$((BACKOFF * 2))
    [[ "$BACKOFF" -gt 60 ]] && BACKOFF=60
  else
    BACKOFF=1
  fi
  [[ "$ONCE" == "--once" ]] && break
  sleep "$BACKOFF"
done
