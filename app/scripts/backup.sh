#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPOSE_FILE="${COMPOSE_FILE:-$ROOT/app/deploy/compose.prod.yaml}"
BACKUP_ROOT="${BACKUP_ROOT:-$ROOT/artifacts/backup}"
OFFHOST_DIR="${BACKUP_OFFHOST_DIR:-}"
KEY_FILE="${BACKUP_KEY_FILE:-}"
TOKEN="$( [[ -n "${FUTURES_SERVICE_TOKEN:-}" ]] && printf %s "$FUTURES_SERVICE_TOKEN" || cat "${FUTURES_SERVICE_TOKEN_FILE:-/dev/null}" )"
RESEARCH_TOKEN="$( [[ -n "${ZHIGU_INTERNAL_TOKEN:-}" ]] && printf %s "$ZHIGU_INTERNAL_TOKEN" || cat "${ZHIGU_INTERNAL_TOKEN_FILE:-/dev/null}" )"
[[ -n "$TOKEN" && -n "$RESEARCH_TOKEN" ]] || { echo "worker tokens are required" >&2; exit 2; }
[[ -n "$KEY_FILE" && -s "$KEY_FILE" ]] || { echo "BACKUP_KEY_FILE is required" >&2; exit 2; }
mkdir -p "$BACKUP_ROOT"
ID="$(date -u +%Y%m%dT%H%M%SZ)-$$"
DIR="$BACKUP_ROOT/$ID"
mkdir -p "$DIR"
EPOCH="backup-$ID"
BARRIER_CLOSED=0
ack() {
  curl -fsS -X POST -H "Content-Type: application/json" -H "Authorization: Bearer $RESEARCH_TOKEN" -d "{\"epoch\":\"$EPOCH\"}" "http://127.0.0.1:8080/internal/ops/v1/backup/begin" >/dev/null
  curl -fsS -X POST -H "Content-Type: application/json" -H "X-Futures-Service-Token: $TOKEN" -d "{\"epoch\":\"$EPOCH\"}" "http://127.0.0.1:8092/internal/ops/quiesce/begin" >/dev/null
  curl -fsS -X POST -H "Content-Type: application/json" -H "Authorization: Bearer $RESEARCH_TOKEN" -d "{\"epoch\":\"$EPOCH\"}" "http://127.0.0.1:8091/internal/ops/quiesce/begin" >/dev/null
}
release() {
  if [[ "$BARRIER_CLOSED" == 0 ]]; then
    curl -fsS -X POST -H "Content-Type: application/json" -H "Authorization: Bearer $RESEARCH_TOKEN" -d "{\"epoch\":\"$EPOCH\",\"status\":\"failed\"}" "http://127.0.0.1:8080/internal/ops/v1/backup/end" >/dev/null || true
  fi
  curl -fsS -X POST -H "Content-Type: application/json" -H "X-Futures-Service-Token: $TOKEN" -d "{\"epoch\":\"$EPOCH\"}" "http://127.0.0.1:8092/internal/ops/quiesce/end" >/dev/null || true
  curl -fsS -X POST -H "Content-Type: application/json" -H "Authorization: Bearer $RESEARCH_TOKEN" -d "{\"epoch\":\"$EPOCH\"}" "http://127.0.0.1:8091/internal/ops/quiesce/end" >/dev/null || true
}
trap release EXIT
ack
for i in $(seq 1 60); do
  f="$(curl -fsS -H "X-Futures-Service-Token: $TOKEN" "http://127.0.0.1:8092/internal/ops/quiesce/status?epoch=$EPOCH")"
  r="$(curl -fsS -H "Authorization: Bearer $RESEARCH_TOKEN" "http://127.0.0.1:8091/internal/ops/quiesce/status?epoch=$EPOCH")"
  g="$(curl -fsS -H "Authorization: Bearer $RESEARCH_TOKEN" "http://127.0.0.1:8080/internal/ops/v1/backup/status?epoch=$EPOCH")"
  [[ "$f" == *'"acknowledged":true'* && "$r" == *'"acknowledged":true'* && "$g" == *'"acknowledged":true'* ]] && break
  [[ "$i" == 60 ]] && { echo "worker quiesce timeout" >&2; exit 1; }
  sleep 1
done
PGPASSWORD="${POSTGRES_PASSWORD:-}" pg_dump --format=custom --file "$DIR/postgresql.dump" "${DATABASE_URL:-${ZHIGU_POSTGRES_DSN:-}}"
python3 - "${ZHIGU_RESEARCH_SQLITE:-$ROOT/app/research-service/data/jobs.sqlite}" "$DIR/research.sqlite" <<'PY'
import sqlite3, sys
src, dst = sys.argv[1:]
con = sqlite3.connect(src)
con.backup(sqlite3.connect(dst))
con.close()
PY
tar -C "$ROOT/app/futures-service" -cf "$DIR/futures-documents.tar" data 2>/dev/null || tar -cf "$DIR/futures-documents.tar" --files-from /dev/null
sha256sum "$DIR/postgresql.dump" "$DIR/research.sqlite" "$DIR/futures-documents.tar" > "$DIR/SHA256SUMS"
cat > "$DIR/manifest.json" <<JSON
{"backup_id":"$ID","epoch":"$EPOCH","created_at":"$(date -u +%Y-%m-%dT%H:%M:%SZ)","schema":"zhigu.backup.v1","rpo_hours":24,"files":["postgresql.dump","research.sqlite","futures-documents.tar"]}
JSON
tar -C "$DIR" -cf "$DIR/backup.tar" postgresql.dump research.sqlite futures-documents.tar SHA256SUMS manifest.json
if command -v age >/dev/null 2>&1; then
  age -r "$(cat "${BACKUP_RECIPIENT_FILE:-$KEY_FILE}")" -o "$DIR/backup.age" "$DIR/backup.tar"
elif command -v openssl >/dev/null 2>&1; then
  openssl enc -aes-256-cbc -pbkdf2 -pass "file:$KEY_FILE" -in "$DIR/backup.tar" -out "$DIR/backup.age"
else
  echo "age or openssl is required for encrypted backup" >&2; exit 2
fi
rm -f "$DIR/backup.tar"
if [[ -n "$OFFHOST_DIR" ]]; then
  mkdir -p "$OFFHOST_DIR"
  cp "$DIR/backup.age" "$OFFHOST_DIR/"
fi
curl -fsS -X POST -H "Content-Type: application/json" -H "Authorization: Bearer $RESEARCH_TOKEN" -d "{\"epoch\":\"$EPOCH\",\"status\":\"succeeded\"}" "http://127.0.0.1:8080/internal/ops/v1/backup/end" >/dev/null
BARRIER_CLOSED=1
retained=()
while IFS= read -r directory; do
  [[ -n "$directory" ]] && retained+=("$directory")
done < <(ls -1dt "$BACKUP_ROOT"/*/ 2>/dev/null || true)
for ((index=7; index<${#retained[@]}; index++)); do
  rm -rf "${retained[$index]}"
done
echo "$DIR/backup.age"
