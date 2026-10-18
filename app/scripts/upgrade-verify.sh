#!/usr/bin/env bash
# upgrade-verify.sh proves previous-database -> current-app and current-database -> previous-app compatibility.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MODE="${1:---static}"
COMPOSE=(-f "$ROOT/app/deploy/compose.prod.yaml")
ROLLBACK=(-f "$ROOT/app/deploy/compose.rollback.yaml")
PREVIOUS_DUMP="${PREVIOUS_DB_DUMP:-}"
PREVIOUS_IMAGE="${ZHIGU_PREVIOUS_SERVER_IMAGE:-}"

static() {
  python3 - "$ROOT/app/server/migrations" <<'PY'
from pathlib import Path
import re, sys
by_module = {}
for path in Path(sys.argv[1]).glob("*/*.sql"):
    match = re.match(r"^(\d+)_", path.name)
    assert match, path
    by_module.setdefault(path.parent.name, []).append((int(match.group(1)), path))
assert by_module, "no migrations"
for module, versions in by_module.items():
    versions.sort()
    assert len({version for version, _ in versions}) == len(versions), f"duplicate migration version in {module}"
    assert all(versions[i][0] < versions[i + 1][0] for i in range(len(versions) - 1)), f"migration order is not monotonic in {module}"
print("PASS: migration versions are unique and monotonic per module")
PY
  grep -q 'expand' "$ROOT/spec/后端生产化_开发SPEC.md"
  grep -q 'ZHIGU_PREVIOUS_SERVER_IMAGE' "$ROOT/app/deploy/compose.rollback.yaml"
}

full() {
  command -v docker >/dev/null || { echo "NOT RUN: Docker unavailable" >&2; exit 2; }
  [[ -s "$PREVIOUS_DUMP" ]] || { echo "NOT RUN: PREVIOUS_DB_DUMP is missing" >&2; exit 2; }
  [[ -n "$PREVIOUS_IMAGE" ]] || { echo "NOT RUN: ZHIGU_PREVIOUS_SERVER_IMAGE is missing" >&2; exit 2; }
  pg_restore --clean --if-exists --dbname="${PGDATABASE:?PGDATABASE required}" "$PREVIOUS_DUMP"
  docker compose "${COMPOSE[@]}" up -d --build postgres server
  curl -fsS --retry 60 --retry-delay 2 http://127.0.0.1:8080/readyz >/tmp/zhigu-upgrade-ready.json
  current_count="$(psql "${ZHIGU_POSTGRES_DSN:?ZHIGU_POSTGRES_DSN required}" -Atqc 'SELECT count(*) FROM finance_users')"
  docker compose "${COMPOSE[@]}" "${ROLLBACK[@]}" up -d --no-build server
  curl -fsS --retry 60 --retry-delay 2 http://127.0.0.1:8080/readyz >/tmp/zhigu-rollback-ready.json
  rollback_count="$(psql "$ZHIGU_POSTGRES_DSN" -Atqc 'SELECT count(*) FROM finance_users')"
  [[ "$current_count" == "$rollback_count" ]] || { echo "N/N-1 core count changed" >&2; exit 1; }
  echo "PASS: current app migrated previous DB and previous app accepted current compatible DB"
}

case "$MODE" in
  --static) static ;;
  --run) static; full ;;
  *) echo "usage: $0 [--static|--run]" >&2; exit 64 ;;
esac
