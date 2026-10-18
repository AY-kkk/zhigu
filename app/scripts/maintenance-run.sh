#!/usr/bin/env bash
set -euo pipefail
ROOT="${ZHIGU_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
LOCK_FILE="${MAINTENANCE_LOCK:-/run/zhigu-maintenance.lock}"
ALERT_COMMAND="${ALERT_COMMAND:-}"
METRICS_URL="${ZHIGU_METRICS_URL:-http://127.0.0.1:8080/metrics}"
BACKUP_LAST_SUCCESS_FILE="${ZHIGU_BACKUP_LAST_SUCCESS_FILE:-/var/lib/zhigu/backup-last-success}"
TLS_CERT="${ZHIGU_TLS_CERT:-/etc/nginx/certs/fullchain.pem}"
DB_URL="${ZHIGU_POSTGRES_DSN:-${DATABASE_URL:-}}"

log() {
  local level="$1" code="$2" message="$3"
  printf '{"timestamp":"%s","level":"%s","service":"zhigu-maintenance","code":"%s","message":"%s"}\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$level" "$code" "$message"
}

alert() {
  local payload="$1"
  log error ALERT "$payload"
  if [[ -n "$ALERT_COMMAND" ]]; then
    printf '%s' "$payload" | if command -v timeout >/dev/null 2>&1; then timeout 30s sh -c "$ALERT_COMMAND"; else sh -c "$ALERT_COMMAND"; fi
  fi
}

exec 9>"$LOCK_FILE"
if ! flock -n 9; then
  log warn LOCK_BUSY "another maintenance run is active"
  exit 0
fi

failed=0
if ! timeout 300s "$ROOT/app/scripts/backup.sh" >/tmp/zhigu-maintenance-backup.out 2>&1; then
  alert '{"kind":"backup","severity":"critical","message":"encrypted backup failed"}'
  failed=1
fi
printf '%s' "$(date -u +%s)" > "$BACKUP_LAST_SUCCESS_FILE"

disk_used_percent="$(df -P "${ZHIGU_DATA_ROOT:-/var/lib/zhigu}" | awk 'NR==2 {gsub("%","",$5); print $5}')"
disk_free_percent=$((100 - disk_used_percent))
if [[ "$disk_free_percent" -lt 15 ]]; then
  alert '{"kind":"disk_free","severity":"critical","message":"free disk is below 15 percent"}'
  failed=1
fi

backup_age=999999
if [[ -s "$BACKUP_LAST_SUCCESS_FILE" ]]; then
  backup_age=$(( $(date -u +%s) - $(cat "$BACKUP_LAST_SUCCESS_FILE") ))
fi
if [[ "$backup_age" -gt 93600 ]]; then
  alert '{"kind":"backup_age","severity":"critical","message":"backup is older than 26 hours"}'
  failed=1
fi

if [[ -f "$TLS_CERT" ]] && ! openssl x509 -in "$TLS_CERT" -checkend 1209600 -noout >/dev/null 2>&1; then
  alert '{"kind":"certificate","severity":"warning","message":"TLS certificate expires within 14 days"}'
  failed=1
fi

if [[ -n "$DB_URL" ]] && command -v psql >/dev/null 2>&1; then
  oldest="$(psql "$DB_URL" -Atqc "SELECT coalesce(extract(epoch FROM now()-min(created_at))::bigint,0) FROM finance_research_tasks WHERE status='queued'")"
  cleanup_backlog="$(psql "$DB_URL" -Atqc "SELECT count(*) FROM futures_tombstones WHERE purge_due_at < now()")"
  [[ "$oldest" -gt 3600 ]] && { alert '{"kind":"queue_age","severity":"warning","message":"oldest queued task exceeds one hour"}'; failed=1; }
  [[ "$cleanup_backlog" -gt 1000 ]] && { alert '{"kind":"cleanup_backlog","severity":"warning","message":"cleanup backlog exceeds 1000"}'; failed=1; }
fi

if curl -fsS --max-time 5 "$METRICS_URL" | grep -Eq 'zhigu_budget_[^ ]+ (nan|-)'; then
  alert '{"kind":"budget","severity":"critical","message":"budget metrics report NaN or negative values"}'
  failed=1
fi

if [[ -f "$TLS_CERT" && "$TLS_CERT" -nt "${TLS_RELOAD_MARKER:-/var/lib/zhigu/tls-reload.marker}" ]]; then
  docker compose -f "$ROOT/app/deploy/compose.prod.yaml" exec -T web nginx -s reload
  touch "${TLS_RELOAD_MARKER:-/var/lib/zhigu/tls-reload.marker}"
fi

if [[ "$failed" -ne 0 ]]; then
  log error MAINTENANCE_FAILED "one or more maintenance checks failed"
  exit 1
fi
log info MAINTENANCE_OK "backup cleanup certificate disk queue and budget checks passed"
