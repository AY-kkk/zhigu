#!/bin/sh
# Copy the owner-authorized pinned snapshot into app/gosaas.
# Origin is read-only; this script never pushes.
set -eu
ROOT="$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)"
SRC="$ROOT/references/gosaas"
DST="$ROOT/app/gosaas"
if [ ! -d "$SRC/server" ]; then
  echo "missing $SRC (pinned snapshot)" >&2
  exit 1
fi
rsync -a --delete \
  --exclude '.git' --exclude 'node_modules' --exclude 'dist' --exclude '.env' --exclude '.env.*' \
  "$SRC/" "$DST/"
printf '%s\n' 'local-authorized-snapshot' > "$DST/ORIGIN.txt"
echo "vendored GoSaaS snapshot to $DST"
