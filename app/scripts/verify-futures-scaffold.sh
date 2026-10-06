#!/usr/bin/env bash
set -euo pipefail
task_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
task_python="${FUTURES_PYTHON:-python3}"
export GOCACHE="${GOCACHE:-${TMPDIR:-/tmp}/zhigu-futures-go-cache}"
command -v go >/dev/null || { echo 'Go 1.24.2 is required in PATH' >&2; exit 1; }
command -v node >/dev/null
"$task_python" -c 'import sys; assert sys.version_info[:2] == (3, 12), "Python 3.12 required"; import jsonschema'
(
  cd "$task_root/app/server"
  go test ./service/futures/... ./api/v1/futures/...
)
(
  cd "$task_root/app/futures-service"
  PYTHONPATH=src "$task_python" -m unittest discover -s tests -v
)
"$task_python" "$task_root/app/scripts/verify-futures-contracts.py"
(
  cd "$task_root/app/web"
  node --test src/modules/futures/tests/*.test.mjs
)
echo 'PASS: futures scaffold only; host integration and live gates are not verified'
