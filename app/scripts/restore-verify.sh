#!/usr/bin/env bash
# restore-verify.sh verifies encrypted backup recovery into a fresh database.
set -euo pipefail
ARCHIVE="${1:?backup archive required}"
WORK="${2:-$(mktemp -d)}"
KEY_FILE="${BACKUP_KEY_FILE:?BACKUP_KEY_FILE required}"
mkdir -p "$WORK"
if tar -tf "$ARCHIVE" >/dev/null 2>&1; then
  cp "$ARCHIVE" "$WORK/backup.tar"
elif [[ "$ARCHIVE" == *.age ]] && command -v age >/dev/null 2>&1; then
  age -d -i "$KEY_FILE" -o "$WORK/backup.tar" "$ARCHIVE"
else
  openssl enc -d -aes-256-cbc -pbkdf2 -pass "file:$KEY_FILE" -in "$ARCHIVE" -out "$WORK/backup.tar"
fi
tar -xf "$WORK/backup.tar" -C "$WORK"
(cd "$WORK" && sha256sum -c SHA256SUMS)
export PGDATABASE="${PGDATABASE:-zhigu_restore_verify}"
pg_restore --clean --if-exists --dbname="$PGDATABASE" "$WORK/postgresql.dump"
python3 - "$WORK/research.sqlite" <<'PY'
import sqlite3, sys
con = sqlite3.connect(sys.argv[1])
count = con.execute("SELECT count(*) FROM sqlite_master WHERE type='table'").fetchone()[0]
assert count > 0, count
print(f"research tables={count}")
PY
echo "PASS: restore verification completed; record actual RTO in release manifest"
